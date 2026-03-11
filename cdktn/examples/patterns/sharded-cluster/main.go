// Sharded cluster with SSL/TLS and custom roles using the cdktn construct library.
//
// Topology:
//   - 2 shard RSs (shard01, shard02) — 3 members each
//   - 1 config server RS (configRS) — 3 members
//   - 1 mongos router
//
// Features demonstrated:
//   - SSL/TLS with PEM certificate (cluster-level propagation)
//   - Custom roles with cluster-level and database-level privileges
//   - Multiple roles per user (app + monitoring)
//   - Direct connections to shard primaries (automatic)
//
// This example assumes credentials already exist on all components
// (no bootstrap via OriginalUserConfig). Use the full-cluster-setup
// pattern if starting from scratch.
package main

import (
	"fmt"
	"log"
	"os"
)

import "github.com/zph/terraform-provider-mongodb/cdktn"

func main() {
	password := os.Getenv("MONGO_PASSWORD")
	appPassword := os.Getenv("APP_PASSWORD")
	opsPassword := os.Getenv("OPS_PASSWORD")
	caCert := os.Getenv("CA_CERT_PEM")

	if password == "" || appPassword == "" || opsPassword == "" || caCert == "" {
		log.Fatal("MONGO_PASSWORD, APP_PASSWORD, OPS_PASSWORD, and CA_CERT_PEM env vars are required")
	}

	cluster, err := cdktn.NewMongoShardedCluster("prod", &cdktn.MongoShardedClusterProps{
		Credentials: &cdktn.DirectCredentials{
			Username: "root",
			Password: password,
		},

		// Cluster-level SSL — propagates to all provider aliases
		SSL: &cdktn.SSLConfig{
			Enabled:     true,
			Certificate: caCert,
		},

		ConfigServers: cdktn.ConfigServerConfig{
			ReplicaSetName: "configRS",
			Members: []cdktn.MemberConfig{
				{Host: "configsvr0.example.com", Port: 27019},
				{Host: "configsvr1.example.com", Port: 27019},
				{Host: "configsvr2.example.com", Port: 27019},
			},
			ShardConfig: &cdktn.ShardConfigSettings{
				ChainingAllowed:         false,
				HeartbeatIntervalMillis: 1000,
				HeartbeatTimeoutSecs:    10,
				ElectionTimeoutMillis:   5000,
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
				ShardConfig: &cdktn.ShardConfigSettings{
					ChainingAllowed:         false,
					HeartbeatIntervalMillis: 1000,
					HeartbeatTimeoutSecs:    10,
					ElectionTimeoutMillis:   5000,
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
					ChainingAllowed:         false,
					HeartbeatIntervalMillis: 1000,
					HeartbeatTimeoutSecs:    10,
					ElectionTimeoutMillis:   5000,
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

		// Cluster-level roles — applied to all mongos + shard primaries
		Roles: []cdktn.RoleConfig{
			{
				Name:     "app_readwrite",
				Database: "admin",
				Privileges: []cdktn.Privilege{
					{DB: "app_db", Collection: "", Actions: []string{"find", "insert", "update", "remove"}},
				},
			},
			{
				Name:     "ops_monitoring",
				Database: "admin",
				Privileges: []cdktn.Privilege{
					{Cluster: true, Actions: []string{"replSetGetStatus", "serverStatus"}},
					{DB: "admin", Collection: "", Actions: []string{"collStats", "dbStats"}},
				},
			},
		},

		// Cluster-level users
		Users: []cdktn.UserConfig{
			{
				Username: "app_service",
				Password: appPassword,
				Database: "admin",
				Roles:    []cdktn.UserRoleRef{{Role: "app_readwrite", DB: "admin"}},
			},
			{
				Username: "ops_monitor",
				Password: opsPassword,
				Database: "admin",
				Roles: []cdktn.UserRoleRef{
					{Role: "ops_monitoring", DB: "admin"},
					{Role: "clusterMonitor", DB: "admin"},
				},
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
