// Zone sharding: map shards to zones and assign key ranges.
//
// Features demonstrated:
//   - ShardZones: assign shards to named zones (mongodb_shard_zone)
//   - ZoneKeyRanges: route key ranges to zones (mongodb_zone_key_range)
//   - Geographic data routing with hashed shard key ($minKey/$maxKey)
//   - Compound shard key range assignment
//   - Multi-zone shard (one shard in multiple zones)
//   - Automatic dependency ordering (zones created before key ranges)
//
// Topology:
//   - 3 shard RSs: shard01 (US-East), shard02 (US-West), shard03 (US-East + Backup)
//   - 1 config server RS
//   - 1 mongos router
//
// Zone layout:
//   - US-East: shard01, shard03
//   - US-West: shard02
//   - Backup:  shard03 (multi-zone: same shard in two zones)
//
// Key ranges:
//   - orders collection: hashed _id split between US-East and US-West
//   - logs collection: compound key (tenant + timestamp) routed to Backup zone
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

	cluster, err := cdktn.NewMongoShardedCluster("zones", &cdktn.MongoShardedClusterProps{
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
			{
				ReplicaSetName: "shard03",
				Members: []cdktn.MemberConfig{
					{Host: "shard03a.example.com", Port: 27018},
					{Host: "shard03b.example.com", Port: 27018},
					{Host: "shard03c.example.com", Port: 27018},
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

		// Register shards so zone/range resources depend on them
		RegisterShards: true,

		// Map shards to zones.
		// shard03 participates in two zones (US-East + Backup).
		ShardZones: []cdktn.ShardZoneConfig{
			{ShardName: "shard01", Zone: "US-East"},
			{ShardName: "shard02", Zone: "US-West"},
			{ShardName: "shard03", Zone: "US-East"},
			{ShardName: "shard03", Zone: "Backup"},
		},

		// Assign key ranges to zones.
		// The library automatically adds depends_on from key ranges to zone
		// assignments, ensuring zones exist before ranges are created.
		ZoneKeyRanges: []cdktn.ZoneKeyRangeConfig{
			// Hashed _id: lower half -> US-East, upper half -> US-West
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
			// Compound key: route all logs to the Backup zone
			{
				Namespace: "analytics.logs",
				Zone:      "Backup",
				Min:       `{"tenant":{"$minKey":1},"timestamp":{"$minKey":1}}`,
				Max:       `{"tenant":{"$maxKey":1},"timestamp":{"$maxKey":1}}`,
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
