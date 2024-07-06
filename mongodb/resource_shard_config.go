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

func (r *ResourceShardConfig) Update(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	var m ShardModel
	m.Settings.ChainingAllowed = data.Get("chaining_allowed").(bool)
	m.Settings.HeartbeatIntervalMillis = int64(data.Get("heartbeat_interval_millis").(int))
	m.Settings.HeartbeatTimeoutSecs = data.Get("heartbeat_timeout_secs").(int)
	m.Settings.ElectionTimeoutMillis = int64(data.Get("election_timeout_millis").(int))
	client, errD := r.getClient(i)
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

	data.State().
		config.Settings.ChainingAllowed = m.Settings.ChainingAllowed
	config.Settings.HeartbeatIntervalMillis = m.Settings.HeartbeatIntervalMillis
	config.Settings.HeartbeatTimeoutSecs = m.Settings.HeartbeatTimeoutSecs
	config.Settings.ElectionTimeoutMillis = m.Settings.ElectionTimeoutMillis

	ctx = tflog.SetField(ctx, `updated replSetConfig`, config)
	tflog.Debug(ctx, `replacement ReplSetConfig`)

	err := SetReplSetConfig(ctx, client, config)
	if err != nil {
		return diag.FromErr(err)
	}

	data.SetId(config.ID)
	data.Set("shard_name", config.ID)
	data.Set("chaining_allowed", config.Settings.ChainingAllowed)
	data.Set("heartbeat_interval_millis", config.Settings.HeartbeatIntervalMillis)
	data.Set("heartbeat_timeout_secs", config.Settings.HeartbeatTimeoutSecs)
	data.Set("election_timeout_millis", config.Settings.ElectionTimeoutMillis)

	return nil
}

func (r *ResourceShardConfig) getReplSetConfig(ctx context.Context, client *mongo.Client) (*RSConfig, diag.Diagnostics) {
	rs, err := GetReplSetConfig(ctx, client)
	return rs, diag.FromErr(err)
}

func (r *ResourceShardConfig) Read(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	client, errD := r.getClient(i)
	if errD != nil {
		return errD
	}

	config, errD := r.getReplSetConfig(ctx, client)
	if errD != nil {
		return errD
	}

	data.SetId(config.ID)
	data.Set("shard_name", config.ID)
	return nil
}

func (r *ResourceShardConfig) Delete(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	client, err := r.getClient(i)
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

func (r *ResourceShardConfig) getClient(i interface{}) (*mongo.Client, diag.Diagnostics) {
	var config = i.(*MongoDatabaseConfiguration)
	client, connectionError := MongoClientInit(config)
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
	      secondaryDelaySecs: <int>,
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
