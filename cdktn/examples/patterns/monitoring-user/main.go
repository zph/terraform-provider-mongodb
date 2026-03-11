// Least-privilege monitoring role and exporter user using L2 constructs.
//
// Features demonstrated:
//   - MongoMongos L2 construct (single component, not full cluster)
//   - Custom role with cluster-level and database-level privileges
//   - Multiple roles on a single user (custom + built-in clusterMonitor)
//   - Oplog access for replication lag monitoring
//
// Use case: create a Prometheus/Datadog exporter user with minimal
// permissions for metrics collection on an existing mongos.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/zph/terraform-provider-mongodb/cdktn"
)

func main() {
	mongoPassword := os.Getenv("MONGO_PASSWORD")
	exporterPassword := os.Getenv("EXPORTER_PASSWORD")

	if mongoPassword == "" || exporterPassword == "" {
		log.Fatal("MONGO_PASSWORD and EXPORTER_PASSWORD env vars are required")
	}

	stack := cdktn.NewTerraformStack(cdktn.DefaultTerraformVersion, "9.9.9")

	_, err := cdktn.NewMongoMongos(stack, "mongos", &cdktn.MongosProps{
		Members: []cdktn.MemberConfig{
			{Host: "127.0.0.1", Port: 27017},
		},
		Credentials: &cdktn.DirectCredentials{
			Username: "root",
			Password: mongoPassword,
		},

		Roles: []cdktn.RoleConfig{
			{
				Name:     "metrics_exporter",
				Database: "admin",
				Privileges: []cdktn.Privilege{
					// Cluster-level metrics (serverStatus, replSetGetStatus, etc.)
					{Cluster: true, Actions: []string{"serverStatus", "replSetGetStatus"}},
					// Database-level stats across all databases
					{DB: "", Collection: "", Actions: []string{"dbStats", "collStats", "indexStats"}},
					// Oplog access for replication lag monitoring
					{DB: "local", Collection: "oplog.rs", Actions: []string{"find"}},
				},
			},
		},

		Users: []cdktn.UserConfig{
			{
				Username: "mongodb_exporter",
				Password: exporterPassword,
				Database: "admin",
				Roles: []cdktn.UserRoleRef{
					{Role: "metrics_exporter", DB: "admin"},
					// Supplement custom role with built-in clusterMonitor
					{Role: "clusterMonitor", DB: "admin"},
				},
			},
		},
	})
	if err != nil {
		log.Fatalf("failed to build mongos construct: %v", err)
	}

	data, err := stack.Synth()
	if err != nil {
		log.Fatalf("failed to synthesize: %v", err)
	}

	fmt.Println(string(data))
}
