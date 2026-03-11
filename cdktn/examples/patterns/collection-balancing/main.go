// Per-collection balancing configuration using the L3 cluster construct.
//
// Features demonstrated:
//   - CollectionBalancing: enable/disable balancing per collection
//   - Per-collection chunk size override (MongoDB 6.0+)
//   - Global balancer settings alongside per-collection overrides
//   - Dependency ordering: collection balancing depends on shard registration
//
// Use cases:
//   - Disable balancing on a collection during bulk imports to avoid
//     chunk migrations while data is loading
//   - Set a larger chunk size on high-throughput collections to reduce
//     migration frequency
//   - Set a smaller chunk size on collections that need even distribution
//
// Topology:
//   - 2 shard RSs (shard01, shard02)
//   - 1 config server RS
//   - 1 mongos router
//
// Prerequisites:
//   - Shards already registered with mongos
//   - Collections already sharded (sh.shardCollection)
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/zph/terraform-provider-mongodb/cdktn"
)

func main() {
	password := os.Getenv("MONGO_PASSWORD")
	if password == "" {
		log.Fatal("MONGO_PASSWORD environment variable is required")
	}

	cluster, err := cdktn.NewMongoShardedCluster("colbal", &cdktn.MongoShardedClusterProps{
		Credentials: &cdktn.DirectCredentials{
			Username: "admin",
			Password: password,
		},

		ConfigServers: cdktn.ConfigServerConfig{
			ReplicaSetName: "configRS",
			Members: []cdktn.MemberConfig{
				{Host: "cfg0.example.com", Port: 27019},
				{Host: "cfg1.example.com", Port: 27019},
				{Host: "cfg2.example.com", Port: 27019},
			},
		},

		Shards: []cdktn.ShardConfig{
			{
				ReplicaSetName: "shard01",
				Members: []cdktn.MemberConfig{
					{Host: "shard01a.example.com", Port: 27018},
					{Host: "shard01b.example.com", Port: 27018},
					{Host: "shard01c.example.com", Port: 27018},
				},
			},
			{
				ReplicaSetName: "shard02",
				Members: []cdktn.MemberConfig{
					{Host: "shard02a.example.com", Port: 27018},
					{Host: "shard02b.example.com", Port: 27018},
					{Host: "shard02c.example.com", Port: 27018},
				},
			},
		},

		Mongos: []cdktn.MongosConfig{
			{
				Members: []cdktn.MemberConfig{
					{Host: "mongos0.example.com", Port: 27017},
				},
			},
		},

		// Register shards so collection balancing resources depend on them
		RegisterShards: true,

		// Global balancer: enabled with a maintenance window
		Balancer: &cdktn.BalancerConfig{
			Enabled:           true,
			ActiveWindowStart: "02:00",
			ActiveWindowStop:  "06:00",
			ChunkSizeMB:       128,
		},

		// Per-collection overrides
		CollectionBalancing: []cdktn.CollectionBalancingConfig{
			// Disable balancing during bulk import window.
			// Re-enable by changing Enabled to true after import completes.
			{
				Namespace: "app_db.staging_imports",
				Enabled:   false,
			},
			// High-throughput collection: larger chunks reduce migration frequency
			{
				Namespace:   "app_db.events",
				Enabled:     true,
				ChunkSizeMB: 256,
			},
			// Small reference collection: smaller chunks for even distribution
			{
				Namespace:   "app_db.config_data",
				Enabled:     true,
				ChunkSizeMB: 32,
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
