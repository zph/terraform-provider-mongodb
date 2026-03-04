package mongodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	// DefaultRemoveTimeoutSecs is the default timeout for shard removal. // CLUS-009
	DefaultRemoveTimeoutSecs = 300

	// shardRemovePollInterval is the polling interval for removeShard. // CLUS-008
	shardRemovePollInterval = 5 * time.Second
)

// BuildShardConnectionString builds the connection string for addShard:
// "rsName/host1:port,host2:port" // CLUS-002
func BuildShardConnectionString(shardName string, hosts []string) string {
	return shardName + "/" + strings.Join(hosts, ",")
}

type ResourceShard struct{}

var RShard = ResourceShard{}

func resourceShard() *schema.Resource {
	return &schema.Resource{
		CreateContext: RShard.Create,
		ReadContext:   RShard.Read,
		UpdateContext: RShard.Update,
		DeleteContext: RShard.Delete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			// CLUS-001: shard_name Required, ForceNew
			"shard_name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The replica set name of the shard to add.",
			},
			// CLUS-001: hosts Required, TypeList, ForceNew
			"hosts": {
				Type:     schema.TypeList,
				Required: true,
				ForceNew: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Description: "List of host:port addresses for the shard replica set members.",
			},
			// CLUS-001: state Computed
			"state": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "The state of the shard as reported by listShards.",
			},
			// CLUS-009/010: remove_timeout_secs Optional, Default 300
			"remove_timeout_secs": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     DefaultRemoveTimeoutSecs,
				Description: "Timeout in seconds for shard removal (draining).",
			},
		},
	}
}

// Create runs addShard to register the shard with the mongos router. // CLUS-002
func (r *ResourceShard) Create(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	providerConf := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, providerConf)
	if err != nil {
		return diag.Errorf("error connecting to MongoDB: %s", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	shardName := data.Get("shard_name").(string)
	hostsRaw := data.Get("hosts").([]interface{})
	hosts := make([]string, len(hostsRaw))
	for i, h := range hostsRaw {
		hosts[i] = h.(string)
	}

	connStr := BuildShardConnectionString(shardName, hosts)

	tflog.Info(ctx, "adding shard", map[string]interface{}{
		"shard_name":        shardName,
		"connection_string": connStr,
	})

	// CLUS-002: addShard command
	res := client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "addShard", Value: connStr},
	})
	if res.Err() != nil {
		return diag.FromErr(fmt.Errorf("addShard failed: %w", res.Err()))
	}

	var resp OKResponse
	if err := res.Decode(&resp); err != nil {
		return diag.FromErr(fmt.Errorf("addShard decode: %w", err))
	}
	if resp.OK != 1 {
		return diag.Errorf("addShard failed: %s", resp.Errmsg)
	}

	data.SetId(shardName)

	// CLUS-003: Read back state
	return r.Read(ctx, data, i)
}

// Read runs listShards and updates state for the target shard. // CLUS-004
func (r *ResourceShard) Read(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	providerConf := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, providerConf)
	if err != nil {
		return diag.Errorf("error connecting to MongoDB: %s", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	shardName := data.Id()
	if shardName == "" {
		shardName = data.Get("shard_name").(string)
	}

	shards, err := ListShards(ctx, client)
	if err != nil {
		return diag.FromErr(err)
	}

	// CLUS-004: Find shard and update state
	for _, s := range shards.Shards {
		if s.ID == shardName {
			if err := data.Set("state", s.State); err != nil {
				return diag.FromErr(err)
			}
			if err := data.Set("shard_name", s.ID); err != nil {
				return diag.FromErr(err)
			}
			return nil
		}
	}

	// CLUS-005: Shard not found — clear ID
	tflog.Warn(ctx, "shard not found in listShards, removing from state", map[string]interface{}{
		"shard_name": shardName,
	})
	data.SetId("")
	return nil
}

// Update returns an error since all schema fields are ForceNew. // CLUS-007
func (r *ResourceShard) Update(_ context.Context, _ *schema.ResourceData, _ interface{}) diag.Diagnostics {
	return diag.Errorf("mongodb_shard does not support in-place updates; all changes force replacement")
}

// Delete runs removeShard and polls until the shard is fully removed. // CLUS-006
func (r *ResourceShard) Delete(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	providerConf := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, providerConf)
	if err != nil {
		return diag.Errorf("error connecting to MongoDB: %s", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	shardName := data.Id()
	timeout := time.Duration(data.Get("remove_timeout_secs").(int)) * time.Second
	deadline := time.Now().Add(timeout)

	tflog.Info(ctx, "removing shard", map[string]interface{}{
		"shard_name": shardName,
		"timeout":    timeout.String(),
	})

	for {
		if time.Now().After(deadline) {
			return diag.Errorf("shard %q removal did not complete within %s", shardName, timeout)
		}

		// CLUS-006: removeShard command
		res := client.Database("admin").RunCommand(ctx, bson.D{
			{Key: "removeShard", Value: shardName},
		})
		if res.Err() != nil {
			return diag.FromErr(fmt.Errorf("removeShard failed: %w", res.Err()))
		}

		var resp ShardRemoveResp
		if err := res.Decode(&resp); err != nil {
			return diag.FromErr(fmt.Errorf("removeShard decode: %w", err))
		}
		if resp.OK != 1 {
			return diag.Errorf("removeShard failed: %s", resp.Msg)
		}

		// CLUS-006: Check if removal is completed
		if resp.State == ShardRemoveCompleted {
			tflog.Info(ctx, "shard removal completed", map[string]interface{}{
				"shard_name": shardName,
			})
			return nil
		}

		tflog.Debug(ctx, "shard removal in progress", map[string]interface{}{
			"shard_name":       shardName,
			"state":            resp.State,
			"remaining_chunks": resp.Remaining.Chunks,
		})

		// CLUS-008: Poll at 5s intervals
		select {
		case <-ctx.Done():
			return diag.FromErr(ctx.Err())
		case <-time.After(shardRemovePollInterval):
		}
	}
}
