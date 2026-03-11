package mongodb

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"go.mongodb.org/mongo-driver/bson"
)

const shardZoneIDSeparator = ":"

// ResourceShardZone manages shard-to-zone assignments via addShardToZone/removeShardFromZone.
type ResourceShardZone struct{}

var RShardZone = ResourceShardZone{}

// formatShardZoneID builds the resource ID from shard name and zone. // ZONE-009
func formatShardZoneID(shardName, zone string) string {
	return shardName + shardZoneIDSeparator + zone
}

// parseShardZoneID splits a resource ID into shard name and zone. // ZONE-009
func parseShardZoneID(id string) (shardName, zone string, err error) {
	parts := strings.SplitN(id, shardZoneIDSeparator, 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid shard_zone ID %q: expected shard_name:zone", id)
	}
	return parts[0], parts[1], nil
}

// ZONE-001
func resourceShardZone() *schema.Resource {
	return &schema.Resource{
		CreateContext: RShardZone.Create,
		ReadContext:   RShardZone.Read,
		UpdateContext: RShardZone.Update,
		DeleteContext: RShardZone.Delete,
		Importer: &schema.ResourceImporter{
			StateContext: shardZoneImportState,
		},
		// ZONE-010, ZONE-011, ZONE-012, ZONE-013
		CustomizeDiff: customdiff.All(
			requireFeature("mongodb_shard_zone"),
			blockFieldChange("shard_name"),
			blockFieldChange("zone"),
			previewCommands(shardZoneCommandPreview),
		),
		Schema: map[string]*schema.Schema{
			"planned_commands": commandPreviewSchema(), // PREVIEW-005
			"shard_name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The name of the shard to associate with the zone.",
			},
			"zone": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The zone name to assign to the shard.",
			},
		},
	}
}

// shardZoneImportState parses the import ID (shard_name:zone) into state.
func shardZoneImportState(_ context.Context, d *schema.ResourceData, _ interface{}) ([]*schema.ResourceData, error) {
	shardName, zone, err := parseShardZoneID(d.Id())
	if err != nil {
		return nil, err
	}
	if err := d.Set("shard_name", shardName); err != nil {
		return nil, err
	}
	if err := d.Set("zone", zone); err != nil {
		return nil, err
	}
	return []*schema.ResourceData{d}, nil
}

// Create runs addShardToZone to assign a shard to a zone. // ZONE-002, ZONE-003
func (r *ResourceShardZone) Create(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	providerConf := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, providerConf)
	if err != nil {
		return diag.Errorf("error connecting to MongoDB: %s", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	// ZONE-008
	if diags := requireMongos(ctx, client); diags.HasError() {
		return diags
	}

	shardName := data.Get("shard_name").(string)
	zone := data.Get("zone").(string)

	tflog.Info(ctx, "adding shard to zone", map[string]interface{}{
		"shard_name": shardName,
		"zone":       zone,
	})

	// ZONE-002
	res := client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "addShardToZone", Value: shardName},
		{Key: "zone", Value: zone},
	})
	if res.Err() != nil {
		return diag.FromErr(fmt.Errorf("addShardToZone failed: %w", res.Err()))
	}

	var resp OKResponse
	if err := res.Decode(&resp); err != nil {
		return diag.FromErr(fmt.Errorf("addShardToZone decode: %w", err))
	}
	if resp.OK != 1 {
		return diag.Errorf("addShardToZone failed: %s", resp.Errmsg)
	}

	// ZONE-009
	data.SetId(formatShardZoneID(shardName, zone))

	// ZONE-003
	return r.Read(ctx, data, i)
}

// Read queries config.shards to verify the shard has the zone in its tags. // ZONE-004, ZONE-005
func (r *ResourceShardZone) Read(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	providerConf := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, providerConf)
	if err != nil {
		return diag.Errorf("error connecting to MongoDB: %s", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	shardName := data.Get("shard_name").(string)
	zone := data.Get("zone").(string)

	if shardName == "" || zone == "" {
		shardName, zone, err = parseShardZoneID(data.Id())
		if err != nil {
			return diag.FromErr(err)
		}
	}

	// ZONE-004: query config.shards for the shard document
	var shardDoc bson.M
	err = client.Database("config").Collection("shards").FindOne(ctx, bson.M{"_id": shardName}).Decode(&shardDoc)
	if err != nil {
		// ZONE-005: shard not found
		tflog.Warn(ctx, "shard not found in config.shards, removing from state", map[string]interface{}{
			"shard_name": shardName,
		})
		data.SetId("")
		return nil
	}

	// Check the tags array contains the zone
	tags, _ := shardDoc["tags"].(bson.A)
	found := false
	for _, t := range tags {
		if tagStr, ok := t.(string); ok && tagStr == zone {
			found = true
			break
		}
	}

	if !found {
		// ZONE-005: zone not in shard's tags
		tflog.Warn(ctx, "zone not found in shard tags, removing from state", map[string]interface{}{
			"shard_name": shardName,
			"zone":       zone,
		})
		data.SetId("")
		return nil
	}

	if err := data.Set("shard_name", shardName); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("zone", zone); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

// Update is a no-op — all fields are identity fields blocked by CustomizeDiff. // ZONE-006
func (r *ResourceShardZone) Update(_ context.Context, _ *schema.ResourceData, _ interface{}) diag.Diagnostics {
	return nil
}

// Delete runs removeShardFromZone. // ZONE-007
func (r *ResourceShardZone) Delete(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	providerConf := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, providerConf)
	if err != nil {
		return diag.Errorf("error connecting to MongoDB: %s", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	shardName := data.Get("shard_name").(string)
	zone := data.Get("zone").(string)

	tflog.Info(ctx, "removing shard from zone", map[string]interface{}{
		"shard_name": shardName,
		"zone":       zone,
	})

	// ZONE-007
	res := client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "removeShardFromZone", Value: shardName},
		{Key: "zone", Value: zone},
	})
	if res.Err() != nil {
		return diag.FromErr(fmt.Errorf("removeShardFromZone failed: %w", res.Err()))
	}

	var resp OKResponse
	if err := res.Decode(&resp); err != nil {
		return diag.FromErr(fmt.Errorf("removeShardFromZone decode: %w", err))
	}
	if resp.OK != 1 {
		return diag.Errorf("removeShardFromZone failed: %s", resp.Errmsg)
	}

	return nil
}
