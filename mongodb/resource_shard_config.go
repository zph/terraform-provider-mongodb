package mongodb

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"go.mongodb.org/mongo-driver/mongo"
)

type ResourceShardConfig struct {
}

// Create detects whether the target replica set is initialized.
// INIT-001: If replSetGetConfig returns code 94, enter init flow.
// INIT-002: If replSetGetConfig returns a valid config, delegate to Update.
// INIT-015: If replSetGetConfig returns code 23, delegate to Update.
func (r *ResourceShardConfig) Create(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	client, cleanup, errD := r.getShardClient(ctx, data, i)
	if errD != nil {
		return errD
	}
	defer cleanup()

	_, err := GetReplSetConfig(ctx, client)
	switch {
	case err == nil:
		// INIT-002: Already configured, delegate to Update
		return r.Update(ctx, data, i)
	case IsAlreadyInitialized(err):
		// INIT-015: Already initialized, delegate to Update
		return r.Update(ctx, data, i)
	case IsNotYetInitialized(err):
		// INIT-001: Enter initialization flow
		return r.initializeReplicaSet(ctx, data, i, client)
	default:
		return diag.FromErr(err)
	}
}

// initializeReplicaSet performs a two-phase RS initialization:
// Phase 1: replSetInitiate with a single member (INIT-007)
// Phase 2: replSetReconfig to add remaining members (INIT-010)
func (r *ResourceShardConfig) initializeReplicaSet(ctx context.Context, data *schema.ResourceData, i interface{}, _ *mongo.Client) diag.Diagnostics {
	providerConf := i.(*MongoDatabaseConfiguration)

	overrides, ok := extractMemberOverrides(data)
	if !ok || len(overrides) == 0 {
		// INIT-003: member blocks required for initialization
		return diag.Errorf("member blocks are required for replica set initialization")
	}
	if errD := validateMemberOverrides(overrides); errD != nil {
		return errD
	}

	shardName := data.Get("shard_name").(string)
	timeout := time.Duration(data.Get("init_timeout_secs").(int)) * time.Second
	firstHost := overrides[0].Host

	host, port, err := SplitHostPort(firstHost)
	if err != nil {
		return diag.FromErr(fmt.Errorf("invalid first member host %q: %w", firstHost, err))
	}

	// INIT-006/017/018/022: Direct connect with auth fallback
	initClient, initCleanup, err := ConnectForInit(ctx, providerConf.Config, host, port, providerConf.MaxConnLifetime)
	if err != nil {
		return diag.FromErr(err)
	}
	defer initCleanup()

	// INIT-007: replSetInitiate with single member
	err = InitiateReplicaSet(ctx, initClient, shardName, firstHost)
	if err != nil {
		if IsAlreadyInitialized(err) {
			// INIT-015: Idempotent — fall through to Update
			return r.Update(ctx, data, i)
		}
		return diag.FromErr(err)
	}

	// INIT-008/009: Wait for PRIMARY
	if err := WaitForPrimary(ctx, initClient, timeout); err != nil {
		return diag.FromErr(err)
	}

	// INIT-010/011/012: Add remaining members and apply settings
	if len(overrides) > 1 {
		config, err := GetReplSetConfig(ctx, initClient)
		if err != nil {
			return diag.FromErr(err)
		}

		config.Members = BuildInitialMembers(overrides)
		config.Version++

		// INIT-012: Apply RS settings from HCL
		config.Settings.ChainingAllowed = data.Get("chaining_allowed").(bool)
		config.Settings.HeartbeatIntervalMillis = int64(data.Get("heartbeat_interval_millis").(int))
		config.Settings.HeartbeatTimeoutSecs = data.Get("heartbeat_timeout_secs").(int)
		config.Settings.ElectionTimeoutMillis = int64(data.Get("election_timeout_millis").(int))

		if err := SetReplSetConfig(ctx, initClient, config); err != nil {
			return diag.FromErr(err)
		}

		// INIT-013/014: Wait for majority healthy
		if err := WaitForMajorityHealthy(ctx, initClient, len(overrides), timeout); err != nil {
			return diag.FromErr(err)
		}
	}

	// Read back final config and set Terraform state
	finalConfig, err := GetReplSetConfig(ctx, initClient)
	if err != nil {
		return diag.FromErr(err)
	}

	data.SetId(finalConfig.ID)
	if err := data.Set("shard_name", finalConfig.ID); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("chaining_allowed", finalConfig.Settings.ChainingAllowed); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("heartbeat_interval_millis", finalConfig.Settings.HeartbeatIntervalMillis); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("heartbeat_timeout_secs", finalConfig.Settings.HeartbeatTimeoutSecs); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("election_timeout_millis", finalConfig.Settings.ElectionTimeoutMillis); err != nil {
		return diag.FromErr(err)
	}

	managedHosts := managedHostsFromState(data)
	memberState := RSConfigMembersToState(finalConfig.Members, managedHosts)
	if err := data.Set("member", memberState); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

type SettingsModel struct {
	ChainingAllowed         bool  `tfsdk:"chaining_allowed,omitempty"`
	HeartbeatIntervalMillis int64 `tfsdk:"heartbeat_interval_millis,omitempty"`
	HeartbeatTimeoutSecs    int   `tfsdk:"heartbeat_timeout_secs,omitempty"`
	ElectionTimeoutMillis   int64 `tfsdk:"election_timeout_millis,omitempty"`
}

type ShardModel struct {
	ID       string        `tfsdk:"id"`
	Settings SettingsModel `tfsdk:"settings"`
}

// MemberOverride represents Terraform-declared member configuration.
// SHARD-003: Members are identified by host (case-sensitive exact match).
type MemberOverride struct {
	Host         string
	Priority     float64
	Votes        int
	Hidden       bool
	ArbiterOnly  bool
	BuildIndexes bool
	Tags         map[string]string
}

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

// toFloat64 converts Terraform schema priority (TypeFloat → float64, or int from state) to float64.
func toFloat64(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch p := v.(type) {
	case float64:
		return p
	case int:
		return float64(p)
	case int64:
		return float64(p)
	default:
		return 0
	}
}
func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// MergeMembers applies Terraform member overrides onto RSConfig members,
// matching by host. Returns error if any override host is not found.
// SHARD-004: Error when host not found.
// SHARD-005: All fields from the override are applied.
// SHARD-006: Unlisted members are left unchanged.
func MergeMembers(rsMembers ConfigMembers, overrides []MemberOverride) (ConfigMembers, error) {
	if len(overrides) == 0 {
		return rsMembers, nil
	}

	hostIndex := make(map[string]int, len(rsMembers))
	for i, m := range rsMembers {
		hostIndex[m.Host] = i
	}

	for _, o := range overrides {
		idx, found := hostIndex[o.Host]
		if !found {
			return nil, fmt.Errorf("member host %q not found in replica set members: %v",
				o.Host, memberHosts(rsMembers))
		}

		rsMembers[idx].Priority = o.Priority
		rsMembers[idx].Votes = intPtr(o.Votes)
		rsMembers[idx].Hidden = boolPtr(o.Hidden)
		rsMembers[idx].ArbiterOnly = boolPtr(o.ArbiterOnly)
		rsMembers[idx].BuildIndexes = boolPtr(o.BuildIndexes)
		if o.Tags != nil {
			rsMembers[idx].Tags = ReplsetTags(o.Tags)
		} else {
			rsMembers[idx].Tags = nil
		}
	}

	return rsMembers, nil
}

func memberHosts(members ConfigMembers) []string {
	hosts := make([]string, len(members))
	for i, m := range members {
		hosts[i] = m.Host
	}
	return hosts
}

// RSConfigMembersToState converts ConfigMembers to the []interface{} format
// for Terraform state. If managedHosts is nil, returns nil (no member block
// declared). Only members whose host is in managedHosts are included.
// SHARD-007: Read-back for drift detection.
// SHARD-008: Only managed hosts returned.
func RSConfigMembersToState(members ConfigMembers, managedHosts map[string]bool) []interface{} {
	if managedHosts == nil {
		return nil
	}

	result := make([]interface{}, 0, len(managedHosts))
	for _, m := range members {
		if !managedHosts[m.Host] {
			continue
		}

		memberMap := map[string]interface{}{
			"host":          m.Host,
			"priority":      m.Priority,
			"arbiter_only":  derefBool(m.ArbiterOnly),
			"build_indexes": derefBool(m.BuildIndexes),
			"hidden":        derefBool(m.Hidden),
			"votes":         derefInt(m.Votes),
		}

		if m.Tags != nil {
			tags := make(map[string]interface{}, len(m.Tags))
			for k, v := range m.Tags {
				tags[k] = v
			}
			memberMap["tags"] = tags
		} else {
			memberMap["tags"] = map[string]interface{}{}
		}

		result = append(result, memberMap)
	}

	return result
}

// validateMemberOverrides returns diagnostics if any member has an empty or missing host.
// Call this before using overrides so MergeMembers does not receive invalid hosts.
func validateMemberOverrides(overrides []MemberOverride) diag.Diagnostics {
	for i, o := range overrides {
		if strings.TrimSpace(o.Host) == "" {
			return diag.Errorf("member at index %d: host is required and must be non-empty (host:port)", i)
		}
	}
	return nil
}

// extractMemberOverrides parses the Terraform "member" block into MemberOverride structs.
// Host is read safely; use validateMemberOverrides after extraction to reject empty or missing host.
func extractMemberOverrides(data *schema.ResourceData) ([]MemberOverride, bool) {
	v, ok := data.GetOk("member")
	if !ok {
		return nil, false
	}
	tfMembers := v.([]interface{})
	if len(tfMembers) == 0 {
		return nil, false
	}
	overrides := make([]MemberOverride, 0, len(tfMembers))
	for _, raw := range tfMembers {
		m := raw.(map[string]interface{})
		var host string
		if v, ok := m["host"]; ok && v != nil {
			if s, ok := v.(string); ok {
				host = s
			}
		}
		override := MemberOverride{
			Host:         host,
			Priority:     toFloat64(m["priority"]),
			Votes:        m["votes"].(int),
			Hidden:       m["hidden"].(bool),
			ArbiterOnly:  m["arbiter_only"].(bool),
			BuildIndexes: m["build_indexes"].(bool),
		}
		if tags, ok := m["tags"].(map[string]interface{}); ok && len(tags) > 0 {
			override.Tags = make(map[string]string, len(tags))
			for k, v := range tags {
				override.Tags[k] = v.(string)
			}
		}
		overrides = append(overrides, override)
	}
	return overrides, true
}

// managedHostsFromState extracts the set of managed hosts from the current TF state.
// Entries with missing or empty host are skipped so we do not panic on invalid state.
func managedHostsFromState(data *schema.ResourceData) map[string]bool {
	v, ok := data.GetOk("member")
	if !ok {
		return nil
	}
	tfMembers := v.([]interface{})
	if len(tfMembers) == 0 {
		return nil
	}
	managed := make(map[string]bool, len(tfMembers))
	for _, raw := range tfMembers {
		m := raw.(map[string]interface{})
		var host string
		if v, ok := m["host"]; ok && v != nil {
			if s, ok := v.(string); ok {
				host = s
			}
		}
		if strings.TrimSpace(host) != "" {
			managed[host] = true
		}
	}
	return managed
}

func (r *ResourceShardConfig) Update(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	var m ShardModel

	m.Settings.ChainingAllowed = data.Get("chaining_allowed").(bool)
	m.Settings.HeartbeatIntervalMillis = int64(data.Get("heartbeat_interval_millis").(int))
	m.Settings.HeartbeatTimeoutSecs = data.Get("heartbeat_timeout_secs").(int)
	m.Settings.ElectionTimeoutMillis = int64(data.Get("election_timeout_millis").(int))
	client, cleanup, errD := r.getShardClient(ctx, data, i)
	if errD != nil {
		return errD
	}
	defer cleanup()

	config, errD := r.getReplSetConfig(ctx, client)
	if errD != nil {
		return errD
	}

	ctx = tflog.SetField(ctx, `replSetConfig`, config)
	tflog.Debug(ctx, `fetched ReplSetConfig`)

	version := config.Version
	version += 1
	config.Version = version

	config.Settings.ChainingAllowed = m.Settings.ChainingAllowed
	config.Settings.HeartbeatIntervalMillis = m.Settings.HeartbeatIntervalMillis
	config.Settings.HeartbeatTimeoutSecs = m.Settings.HeartbeatTimeoutSecs
	config.Settings.ElectionTimeoutMillis = m.Settings.ElectionTimeoutMillis

	// SHARD-003/005/006: Apply member overrides if present
	if overrides, ok := extractMemberOverrides(data); ok {
		if errD := validateMemberOverrides(overrides); errD != nil {
			return errD
		}
		// Reject any override whose host is not in the current replica set (e.g. user changed host).
		// Changing host is not allowed; remove the member and add a new one.
		rsHosts := make(map[string]bool, len(config.Members))
		for _, m := range config.Members {
			rsHosts[m.Host] = true
		}
		for _, o := range overrides {
			if !rsHosts[o.Host] {
				return diag.Errorf("changing member host is not allowed: %q is not in the replica set (current members: %v). Remove the member and add a new one with the desired host",
					o.Host, memberHosts(config.Members))
			}
		}
		merged, mergeErr := MergeMembers(config.Members, overrides)
		if mergeErr != nil {
			return diag.FromErr(mergeErr)
		}
		config.Members = merged
	}

	ctx = tflog.SetField(ctx, `updated replSetConfig`, config)
	tflog.Debug(ctx, `replacement ReplSetConfig`)

	err := SetReplSetConfig(ctx, client, config)
	if err != nil {
		return diag.FromErr(err)
	}

	data.SetId(config.ID)
	if err := data.Set("shard_name", config.ID); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("chaining_allowed", config.Settings.ChainingAllowed); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("heartbeat_interval_millis", config.Settings.HeartbeatIntervalMillis); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("heartbeat_timeout_secs", config.Settings.HeartbeatTimeoutSecs); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("election_timeout_millis", config.Settings.ElectionTimeoutMillis); err != nil {
		return diag.FromErr(err)
	}

	// SHARD-007/008: Read back member state for drift detection
	managedHosts := managedHostsFromState(data)
	memberState := RSConfigMembersToState(config.Members, managedHosts)
	if err := data.Set("member", memberState); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func (r *ResourceShardConfig) getReplSetConfig(ctx context.Context, client *mongo.Client) (*RSConfig, diag.Diagnostics) {
	rs, err := GetReplSetConfig(ctx, client)
	return rs, diag.FromErr(err)
}

func (r *ResourceShardConfig) Read(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	client, cleanup, errD := r.getShardClient(ctx, data, i)
	if errD != nil {
		return errD
	}
	defer cleanup()

	config, errD := r.getReplSetConfig(ctx, client)
	if errD != nil {
		return errD
	}

	data.SetId(config.ID)
	if err := data.Set("shard_name", config.ID); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("chaining_allowed", config.Settings.ChainingAllowed); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("heartbeat_interval_millis", config.Settings.HeartbeatIntervalMillis); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("heartbeat_timeout_secs", config.Settings.HeartbeatTimeoutSecs); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("election_timeout_millis", config.Settings.ElectionTimeoutMillis); err != nil {
		return diag.FromErr(err)
	}

	// SHARD-007/008/010: Read back managed members for drift detection
	managedHosts := managedHostsFromState(data)
	memberState := RSConfigMembersToState(config.Members, managedHosts)
	if err := data.Set("member", memberState); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func (r *ResourceShardConfig) Delete(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	client, cleanup, errD := r.getShardClient(ctx, data, i)
	if errD != nil {
		return errD
	}
	defer cleanup()

	_ = client
	//	var stateId = data.State().ID
	//	roleName, database, err := r.ParseId(stateId)
	//
	//	if err != nil {
	//		return diag.Errorf("%s", err)
	//	}
	//
	//	db := client.Database(database)
	//	result := db.RunCommand(context.Background(), bson.D{{Key: "dropRole", Value: roleName}})
	//
	//	if result.Err() != nil {
	//		return diag.Errorf("%s", result.Err())
	//	}
	//
	return nil
}

// getShardClient returns a MongoDB client connected to the appropriate shard.
// If the provider is connected to a mongos, it auto-discovers the shard via
// listShards and creates a temporary direct connection. The returned cleanup
// function MUST be called via defer to disconnect temporary clients.
// DISC-001 through DISC-010
func (r *ResourceShardConfig) getShardClient(ctx context.Context, data *schema.ResourceData, i interface{}) (*mongo.Client, func(), diag.Diagnostics) {
	providerConf := i.(*MongoDatabaseConfiguration)
	providerClient, err := MongoClientInit(ctx, providerConf)
	if err != nil {
		return nil, func() {}, diag.Errorf("Error connecting to database: %s", err)
	}

	shardName := data.Get("shard_name").(string)
	hostOverride := ""
	if v, ok := data.GetOk("host_override"); ok {
		hostOverride = v.(string)
	}

	shardClient, shardCleanup, err := ResolveShardClient(
		ctx, providerClient, providerConf.Config,
		shardName, hostOverride, providerConf.MaxConnLifetime,
	)
	if err != nil {
		_ = providerClient.Disconnect(ctx)
		return nil, func() {}, diag.Errorf("Error resolving shard client: %s", err)
	}

	// Build a combined cleanup that disconnects both clients when the shard
	// client is a separate temporary connection, or just the provider client.
	cleanup := func() {
		shardCleanup()
		// If shard client is the same as provider client, shardCleanup is a
		// noop, so we still need to disconnect the provider client.
		if shardClient != providerClient {
			_ = providerClient.Disconnect(ctx)
		} else {
			_ = providerClient.Disconnect(ctx)
		}
	}

	return shardClient, cleanup, nil
}

func (r *ResourceShardConfig) ParseId(id string) (string, string, error) {
	result, errEncoding := base64.StdEncoding.DecodeString(id)

	if errEncoding != nil {
		return "", "", fmt.Errorf("unexpected format of ID Error : %s", errEncoding)
	}
	parts := strings.SplitN(string(result), ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("unexpected format of ID (%s), expected database.roleName", id)
	}

	database := parts[0]
	roleName := parts[1]

	return roleName, database, nil
}

var RShardConfig = ResourceShardConfig{}

func resourceShardConfig() *schema.Resource {
	return &schema.Resource{
		CreateContext: RShardConfig.Create,
		ReadContext:   RShardConfig.Read,
		UpdateContext: RShardConfig.Update,
		DeleteContext: RShardConfig.Delete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"shard_name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"chaining_allowed": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
			},
			"heartbeat_interval_millis": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1000,
			},
			"heartbeat_timeout_secs": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  10,
			},
			"election_timeout_millis": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  10000,
			},
			// SHARD-001: Optional member block for per-member configuration
			"member": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"host": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The host:port of the replica set member to configure",
						},
						"arbiter_only": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Whether this member is an arbiter",
						},
						"build_indexes": {
							Type:        schema.TypeBool,
							Optional:    true,
							Default:     true,
							Description: "Whether this member builds indexes",
						},
						"hidden": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Whether this member is hidden from client discovery",
						},
						"priority": {
							Type:        schema.TypeFloat,
							Optional:    true,
							Description: "Election priority for this member (0 = never primary). MongoDB accepts 0-1000 (integer or decimal).",
						},
						"tags": {
							Type:     schema.TypeMap,
							Optional: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
							Description: "Replica set tags for this member (zone, dc, rack, etc.)",
						},
						"votes": {
							Type:        schema.TypeInt,
							Optional:    true,
							Description: "Number of votes this member has in elections (0 or 1)",
						},
					},
				},
			},
			// INIT-020/021: Timeout for replica set initialization
			"init_timeout_secs": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     DefaultInitTimeoutSecs,
				Description: "Timeout in seconds for replica set initialization.",
			},
			// DISC-008: Override the shard host discovered via listShards
			"host_override": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "Override the shard host:port discovered via listShards. " +
					"Use when internal hostnames from listShards are unreachable from the Terraform runner.",
			},
		},
	}
}

/*
					db.adminCommand({replSetGetConfig: 1})
					config.{members|settings}
					{
	  //_id: <string>,
	  //version: <int>, (make this computed)
	  //term: <int>,
	  //protocolVersion: <number>,
	  //writeConcernMajorityJournalDefault: <boolean>,
	  //configsvr: <boolean>,
	  members: [
	    {
	      _id: <int>,
	      host: <string>,
	      arbiterOnly: <boolean>,
	      buildIndexes: <boolean>,
	      hidden: <boolean>,
	      priority: <number>,
	      tags: <document>,
	      votes: <number>
	    },
	    ...
	  ],
	  settings: {
	    chainingAllowed : <boolean>,
	    heartbeatIntervalMillis : <int>,
	    heartbeatTimeoutSecs: <int>,
	    electionTimeoutMillis : <int>,
	    catchUpTimeoutMillis : <int>,
	    getLastErrorModes : <document>,
	    getLastErrorDefaults : <document>,
	    replicaSetId: <ObjectId>
	  }
	}

					https://www.mongodb.com/docs/manual/reference/command/replSetReconfig/#mongodb-dbcommand-dbcmd.replSetReconfig
					Change with: {replSetReconfig: {document}, force: false|true})
					{
			        "config" : {
			                "_id" : "shard01",
			                "version" : 1,
			                "protocolVersion" : NumberLong(1),
			                "members" : [
			                        {
			                                "_id" : 0,
			                                "host" : "localhost:27018",
			                                "arbiterOnly" : false,
			                                "buildIndexes" : true,
			                                "hidden" : false,
			                                "priority" : 1,
			                                "tags" : {

			                                },
			                                "slaveDelay" : NumberLong(0),
			                                "votes" : 1
			                        },
			                        {
			                                "_id" : 1,
			                                "host" : "localhost:27019",
			                                "arbiterOnly" : false,
			                                "buildIndexes" : true,
			                                "hidden" : false,
			                                "priority" : 1,
			                                "tags" : {

			                                },
			                                "slaveDelay" : NumberLong(0),
			                                "votes" : 1
			                        },
			                        {
			                                "_id" : 2,
			                                "host" : "localhost:27020",
			                                "arbiterOnly" : false,
			                                "buildIndexes" : true,
			                                "hidden" : false,
			                                "priority" : 1,
			                                "tags" : {

			                                },
			                                "slaveDelay" : NumberLong(0),
			                                "votes" : 1
			                        }
			                ],
			                "settings" : {
			                        "chainingAllowed" : true,
			                        "heartbeatIntervalMillis" : 2000,
			                        "heartbeatTimeoutSecs" : 10,
			                        "electionTimeoutMillis" : 10000,
			                        "catchUpTimeoutMillis" : -1,
			                        "catchUpTakeoverDelayMillis" : 30000,
			                        "getLastErrorModes" : {

			                        },
			                        "getLastErrorDefaults" : {
			                                "w" : 1,
			                                "wtimeout" : 0
			                        },
			                        "replicaSetId" : ObjectId("668874d02b2227c1922e0a7e")
			                }
			        },
			        "ok" : 1,
			        "operationTime" : Timestamp(1720219056, 1),
			        "$gleStats" : {
			                "lastOpTime" : Timestamp(0, 0),
			                "electionId" : ObjectId("7fffffff0000000000000002")
			        },
			        "$configServerState" : {
			                "opTime" : {
			                        "ts" : Timestamp(1720219056, 1),
			                        "t" : NumberLong(2)
			                }
			        },
			        "$clusterTime" : {
			                "clusterTime" : Timestamp(1720219057, 1),
			                "signature" : {
			                        "hash" : BinData(0,"BNjajv5gy5tmFSBhkGccuL0QZwk="),
			                        "keyId" : NumberLong("7388283642583187458")
			                }
			        }
			}
*/

//data.SetId(stateID)
//diags = nil
//return diags
