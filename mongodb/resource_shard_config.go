package mongodb

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"go.mongodb.org/mongo-driver/mongo"
)

type ResourceShardConfig struct {
}

func (r *ResourceShardConfig) Create(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	return r.Update(ctx, data, i)
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
	Priority     int
	Votes        int
	Hidden       bool
	ArbiterOnly  bool
	BuildIndexes bool
	Tags         map[string]string
}

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }
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

// extractMemberOverrides parses the Terraform "member" block into MemberOverride structs.
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
		override := MemberOverride{
			Host:         m["host"].(string),
			Priority:     m["priority"].(int),
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
		managed[m["host"].(string)] = true
	}
	return managed
}

func (r *ResourceShardConfig) Update(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	var m ShardModel

	m.Settings.ChainingAllowed = data.Get("chaining_allowed").(bool)
	m.Settings.HeartbeatIntervalMillis = int64(data.Get("heartbeat_interval_millis").(int))
	m.Settings.HeartbeatTimeoutSecs = data.Get("heartbeat_timeout_secs").(int)
	m.Settings.ElectionTimeoutMillis = int64(data.Get("election_timeout_millis").(int))
	client, errD := r.getClient(ctx, i)
	if errD != nil {
		return errD
	}

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
	client, errD := r.getClient(ctx, i)
	if errD != nil {
		return errD
	}

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
	client, err := r.getClient(ctx, i)
	if err != nil {
		return err
	}

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

func (r *ResourceShardConfig) getClient(ctx context.Context, i interface{}) (*mongo.Client, diag.Diagnostics) {
	var config = i.(*MongoDatabaseConfiguration)
	client, connectionError := MongoClientInit(ctx, config)
	if connectionError != nil {
		return nil, diag.Errorf("Error connecting to database : %s ", connectionError)
	}
	return client, nil
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
							Type:        schema.TypeInt,
							Optional:    true,
							Description: "Election priority for this member (0 = never primary)",
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
