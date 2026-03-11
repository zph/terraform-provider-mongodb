// Full sharded cluster setup from scratch using the cdktn construct library.
//
// Topology:
//   - 1 config server RS (configRS) — 3 members
//   - 2 shard RSs (shard01, shard02) — 3 members each
//   - 1 mongos router
//
// Features demonstrated:
//   - L3 MongoShardedCluster construct (composes all L2s)
//   - Cluster-level credentials cascade to all components
//   - Bootstrap admin users via OriginalUserConfig (localhost exception)
//   - Shard registration (RegisterShards: true)
//   - Balancer configuration with active window
//   - Zone sharding with key range assignments
//   - Cluster-level roles and users propagated to mongos + shard primaries
//   - Per-member replica set overrides (priority, votes, tags)
//
// Execution order is handled automatically by the construct library:
//  1. Provider aliases created for every member
//  2. Original users for bootstrap
//  3. Shard config on each RS primary
//  4. Shard registration via mongos
//  5. Balancer + zone configuration
//  6. Roles and users on all target aliases
//
// Prerequisites:
//   - All mongod/mongos processes started with --auth and --keyFile
//   - Config server RS and shard RSs already initialized (replSetInitiate)
//   - Mongos started with --configdb pointing to the config server RS
//   - No users exist yet (localhost exception allows bootstrap)
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/zph/terraform-provider-mongodb/cdktn"
)

func main() {
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		log.Fatal("ADMIN_PASSWORD environment variable is required")
	}

	appPassword := os.Getenv("APP_PASSWORD")
	if appPassword == "" {
		log.Fatal("APP_PASSWORD environment variable is required")
	}

	cluster, err := cdktn.NewMongoShardedCluster("prod", &cdktn.MongoShardedClusterProps{
		Credentials: &cdktn.DirectCredentials{
			Username: "admin",
			Password: adminPassword,
		},

		// Config server replica set — 3 members
		ConfigServers: cdktn.ConfigServerConfig{
			ReplicaSetName: "configRS",
			Members: []cdktn.MemberConfig{
				{Host: "configsvr0.example.com", Port: 27019},
				{Host: "configsvr1.example.com", Port: 27019},
				{Host: "configsvr2.example.com", Port: 27019},
			},
			// Bootstrap admin on config server primary via localhost exception
			OriginalUsers: []cdktn.OriginalUserConfig{
				{
					Host:     "configsvr0.example.com",
					Port:     27019,
					Username: "admin",
					Password: adminPassword,
					Roles:    []cdktn.UserRoleRef{{Role: "root", DB: "admin"}},
				},
			},
		},

		// Two shard replica sets — 3 members each with per-member overrides
		Shards: []cdktn.ShardConfig{
			{
				ReplicaSetName: "shard01",
				Members: []cdktn.MemberConfig{
					{Host: "shard01a.example.com", Port: 27018},
					{Host: "shard01b.example.com", Port: 27018},
					{Host: "shard01c.example.com", Port: 27018},
				},
				ShardConfig: &cdktn.ShardConfigSettings{
					ChainingAllowed:         true,
					HeartbeatIntervalMillis: 1000,
					HeartbeatTimeoutSecs:    10,
					ElectionTimeoutMillis:   10000,
					Members: []cdktn.MemberOverrideConfig{
						{Host: "shard01a.example.com:27018", Priority: 10, Votes: 1, Tags: map[string]string{"role": "primary-preferred", "zone": "us-east-1a"}},
						{Host: "shard01b.example.com:27018", Priority: 5, Votes: 1, Tags: map[string]string{"role": "secondary", "zone": "us-east-1b"}},
						{Host: "shard01c.example.com:27018", Priority: 5, Votes: 1, Tags: map[string]string{"role": "secondary", "zone": "us-east-1c"}},
					},
				},
				OriginalUsers: []cdktn.OriginalUserConfig{
					{
						Host:     "shard01a.example.com",
						Port:     27018,
						Username: "admin",
						Password: adminPassword,
						Roles:    []cdktn.UserRoleRef{{Role: "root", DB: "admin"}},
					},
				},
			},
			{
				ReplicaSetName: "shard02",
				Members: []cdktn.MemberConfig{
					{Host: "shard02a.example.com", Port: 27018},
					{Host: "shard02b.example.com", Port: 27018},
					{Host: "shard02c.example.com", Port: 27018},
				},
				ShardConfig: &cdktn.ShardConfigSettings{
					ChainingAllowed:         true,
					HeartbeatIntervalMillis: 1000,
					HeartbeatTimeoutSecs:    10,
					ElectionTimeoutMillis:   10000,
					Members: []cdktn.MemberOverrideConfig{
						{Host: "shard02a.example.com:27018", Priority: 10, Votes: 1, Tags: map[string]string{"role": "primary-preferred", "zone": "us-west-2a"}},
						{Host: "shard02b.example.com:27018", Priority: 5, Votes: 1, Tags: map[string]string{"role": "secondary", "zone": "us-west-2b"}},
						{Host: "shard02c.example.com:27018", Priority: 5, Votes: 1, Tags: map[string]string{"role": "secondary", "zone": "us-west-2c"}},
					},
				},
				OriginalUsers: []cdktn.OriginalUserConfig{
					{
						Host:     "shard02a.example.com",
						Port:     27018,
						Username: "admin",
						Password: adminPassword,
						Roles:    []cdktn.UserRoleRef{{Role: "root", DB: "admin"}},
					},
				},
			},
		},

		// Mongos router
		Mongos: []cdktn.MongosConfig{
			{
				Members: []cdktn.MemberConfig{
					{Host: "mongos0.example.com", Port: 27017},
				},
				OriginalUsers: []cdktn.OriginalUserConfig{
					{
						Host:     "mongos0.example.com",
						Port:     27017,
						Username: "admin",
						Password: adminPassword,
						Roles:    []cdktn.UserRoleRef{{Role: "root", DB: "admin"}},
					},
				},
			},
		},

		// Register shards with mongos (generates mongodb_shard resources)
		RegisterShards: true,

		// Balancer with a maintenance window
		Balancer: &cdktn.BalancerConfig{
			Enabled:           true,
			ActiveWindowStart: "02:00",
			ActiveWindowStop:  "06:00",
		},

		// Zone sharding: map shards to geographic zones
		ShardZones: []cdktn.ShardZoneConfig{
			{ShardName: "shard01", Zone: "US-East"},
			{ShardName: "shard02", Zone: "US-West"},
		},

		// Route key ranges to zones (hashed _id split)
		ZoneKeyRanges: []cdktn.ZoneKeyRangeConfig{
			{
				Namespace: "app_db.orders",
				Zone:      "US-East",
				Min:       `{"_id":{"$minKey":1}}`,
				Max:       `{"_id":0}`,
			},
			{
				Namespace: "app_db.orders",
				Zone:      "US-West",
				Min:       `{"_id":0}`,
				Max:       `{"_id":{"$maxKey":1}}`,
			},
		},

		// Cluster-level roles — created on all mongos + shard primaries
		Roles: []cdktn.RoleConfig{
			{
				Name:     "app_readwrite",
				Database: "admin",
				Privileges: []cdktn.Privilege{
					{DB: "app_db", Collection: "", Actions: []string{"find", "insert", "update", "remove"}},
					{DB: "app_db", Collection: "", Actions: []string{"createIndex", "dropIndex"}},
				},
			},
		},

		// Cluster-level users — created on all mongos + shard primaries
		Users: []cdktn.UserConfig{
			{
				Username: "app_service",
				Password: appPassword,
				Database: "admin",
				Roles:    []cdktn.UserRoleRef{{Role: "app_readwrite", DB: "admin"}},
			},
		},
	})
	if err != nil {
		log.Fatalf("failed to build cluster: %v", err)
	}

	data, err := cluster.Stack.Synth()
	if err != nil {
		log.Fatalf("failed to synthesize: %v", err)
	}

	fmt.Println(string(data))
}
