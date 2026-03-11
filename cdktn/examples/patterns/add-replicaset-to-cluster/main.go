// Add a new replica set (shard03) to an existing sharded cluster.
//
// Use case: day-2 horizontal scale-out — the cluster already has mongos,
// config servers, and one or more shards running with auth enabled.
// This generates Terraform JSON for bootstrapping a new RS from bare
// mongod processes and optionally registering it as a shard.
//
// Topology (new):
//   - 1 new shard RS (shard03) — 3 members, initialized from scratch
//
// Topology (existing):
//   - 1 mongos router (already running, admin user exists)
//   - 1+ existing shards (already registered)
//
// Features demonstrated:
//   - MongoShard L2 construct (standalone, not part of L3 cluster)
//   - Bootstrap admin via OriginalUserConfig (localhost exception)
//   - Per-member replica set overrides (priority, votes, tags)
//   - Standalone shard config targeting primary
//
// Prerequisites:
//   - New mongod processes started with --shardsvr, --auth, and --keyFile
//     (same keyFile as the rest of the cluster)
//   - No users exist on the new RS yet (localhost exception allows bootstrap)
//   - Mongos is reachable and an admin user already exists on it
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

	stack := cdktn.NewTerraformStack(cdktn.DefaultTerraformVersion, "9.9.9")

	// Phase 1 & 2: Bootstrap admin + configure new RS using MongoShard L2
	_, err := cdktn.NewMongoShard(stack, "shard03", &cdktn.MongoShardProps{
		ReplicaSetName: "shard03",
		Members: []cdktn.MemberConfig{
			{Host: "shard03a.example.com", Port: 27018},
			{Host: "shard03b.example.com", Port: 27018},
			{Host: "shard03c.example.com", Port: 27018},
		},
		Credentials: &cdktn.DirectCredentials{
			Username: "admin",
			Password: adminPassword,
		},

		// Bootstrap admin on the primary via localhost exception
		OriginalUsers: []cdktn.OriginalUserConfig{
			{
				Host:     "shard03a.example.com",
				Port:     27018,
				Username: "admin",
				Password: adminPassword,
				Roles:    []cdktn.UserRoleRef{{Role: "root", DB: "admin"}},
			},
		},

		// RS membership with priority/votes/tags
		ShardConfig: &cdktn.ShardConfigSettings{
			ChainingAllowed:         true,
			HeartbeatIntervalMillis: 1000,
			HeartbeatTimeoutSecs:    10,
			ElectionTimeoutMillis:   10000,
			Members: []cdktn.MemberOverrideConfig{
				{Host: "shard03a.example.com:27018", Priority: 10, Votes: 1, Tags: map[string]string{"role": "primary-preferred"}},
				{Host: "shard03b.example.com:27018", Priority: 5, Votes: 1, Tags: map[string]string{"role": "secondary"}},
				{Host: "shard03c.example.com:27018", Priority: 5, Votes: 1, Tags: map[string]string{"role": "secondary"}},
			},
		},
	})
	if err != nil {
		log.Fatalf("failed to build shard construct: %v", err)
	}

	// Phase 3: Register shard with mongos
	// To add the shard to the cluster, use a separate MongoMongos construct
	// targeting the existing mongos and call BuildShardRegistrations manually,
	// or use the full L3 MongoShardedCluster with RegisterShards: true.
	//
	// Example (manual registration):
	//
	//   mongosAliases := cdktn.BuildProviders(stack, cdktn.ComponentTypeMongos, "",
	//       []cdktn.MemberConfig{{Host: "mongos.example.com", Port: 27017}},
	//       &cdktn.DirectCredentials{Username: "admin", Password: adminPassword},
	//       nil, "")
	//
	//   cdktn.BuildShardRegistrations(stack, mongosAliases[0],
	//       []cdktn.ShardRegistrationEntry{{RSName: "shard03", Hosts: []string{
	//           "shard03a.example.com:27018",
	//           "shard03b.example.com:27018",
	//           "shard03c.example.com:27018",
	//       }}}, nil)

	data, err := stack.Synth()
	if err != nil {
		log.Fatalf("failed to synthesize: %v", err)
	}

	fmt.Println(string(data))
}
