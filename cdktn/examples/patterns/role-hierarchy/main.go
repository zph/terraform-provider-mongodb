// Layered role hierarchy: viewer -> editor -> admin using L2 constructs.
//
// Features demonstrated:
//   - MongoMongos L2 construct for role and user management
//   - Role inheritance via InheritedRoles
//   - Three-tier privilege escalation pattern
//   - One user at each tier
//
// Each layer adds privileges on top of what it inherits:
//   - viewer: read-only access (find, listCollections, stats)
//   - editor: inherits viewer + write operations (insert, update, remove)
//   - admin:  inherits editor + management (createIndex, dropIndex, createCollection, dropCollection)
//
// Note: The cdktn library creates roles on every provider alias. For
// role inheritance, MongoDB handles the depends_on at the server level —
// the inherited role must exist when the inheriting role is created.
// The library emits user resources with depends_on all roles on the
// same alias, ensuring correct ordering.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/zph/terraform-provider-mongodb/cdktn"
)

func main() {
	mongoPassword := os.Getenv("MONGO_PASSWORD")
	viewerPassword := os.Getenv("VIEWER_PASSWORD")
	editorPassword := os.Getenv("EDITOR_PASSWORD")
	adminPassword := os.Getenv("ADMIN_PASSWORD")

	if mongoPassword == "" || viewerPassword == "" || editorPassword == "" || adminPassword == "" {
		log.Fatal("MONGO_PASSWORD, VIEWER_PASSWORD, EDITOR_PASSWORD, and ADMIN_PASSWORD env vars are required")
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

		// Three-tier role hierarchy
		Roles: []cdktn.RoleConfig{
			// Layer 1: base read-only access
			{
				Name:     "app_viewer",
				Database: "admin",
				Privileges: []cdktn.Privilege{
					{DB: "app_db", Collection: "", Actions: []string{"find", "listCollections"}},
					{DB: "app_db", Collection: "", Actions: []string{"collStats", "dbStats"}},
				},
			},
			// Layer 2: editor inherits viewer, adds write operations
			{
				Name:     "app_editor",
				Database: "admin",
				Privileges: []cdktn.Privilege{
					{DB: "app_db", Collection: "", Actions: []string{"insert", "update", "remove"}},
				},
				InheritedRoles: []cdktn.InheritedRole{
					{Role: "app_viewer", DB: "admin"},
				},
			},
			// Layer 3: admin inherits editor, adds index and collection management
			{
				Name:     "app_admin",
				Database: "admin",
				Privileges: []cdktn.Privilege{
					{DB: "app_db", Collection: "", Actions: []string{"createIndex", "dropIndex", "createCollection", "dropCollection"}},
				},
				InheritedRoles: []cdktn.InheritedRole{
					{Role: "app_editor", DB: "admin"},
				},
			},
		},

		// One user at each tier
		Users: []cdktn.UserConfig{
			{
				Username: "viewer_user",
				Password: viewerPassword,
				Database: "admin",
				Roles:    []cdktn.UserRoleRef{{Role: "app_viewer", DB: "admin"}},
			},
			{
				Username: "editor_user",
				Password: editorPassword,
				Database: "admin",
				Roles:    []cdktn.UserRoleRef{{Role: "app_editor", DB: "admin"}},
			},
			{
				Username: "admin_user",
				Password: adminPassword,
				Database: "admin",
				Roles:    []cdktn.UserRoleRef{{Role: "app_admin", DB: "admin"}},
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
