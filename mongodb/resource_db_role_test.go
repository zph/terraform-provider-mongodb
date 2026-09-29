package mongodb

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// TEST-005: Valid plain text ID returns (roleName, database, nil)
func TestResourceDatabaseRoleParseId_Valid(t *testing.T) {
	roleName, database, err := resourceDatabaseRoleParseId("admin.myRole")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if roleName != "myRole" {
		t.Errorf("expected roleName 'myRole', got '%s'", roleName)
	}
	if database != "admin" {
		t.Errorf("expected database 'admin', got '%s'", database)
	}
}

// TEST-006: Invalid inputs return errors
func TestResourceDatabaseRoleParseId_InvalidInputs(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"no separator", "nodotshere"},
		{"empty database", ".roleName"},
		{"empty roleName", "database."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := resourceDatabaseRoleParseId(tc.input)
			if err == nil {
				t.Fatalf("expected error for case %q, got nil", tc.name)
			}
		})
	}
}

// TEST-057: expandPrivileges copies each privilege and sorts its actions
func TestExpandPrivileges_SortsActions(t *testing.T) {
	res := resourceDatabaseRole()
	data := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"name": "analyst",
		"privilege": []interface{}{
			map[string]interface{}{
				"db":         "analytics",
				"collection": "",
				"actions":    []interface{}{"listIndexes", "find", "collStats"},
			},
			map[string]interface{}{
				"cluster": true,
				"actions": []interface{}{"serverStatus", "replSetGetStatus"},
			},
		},
	})

	got := expandPrivileges(data.Get("privilege").(*schema.Set).List())
	if len(got) != 2 {
		t.Fatalf("expected 2 privileges, got %d: %+v", len(got), got)
	}
	// Set elements have no defined order, so index them by the cluster flag.
	byCluster := map[bool]PrivilegeDto{}
	for _, p := range got {
		byCluster[p.Cluster] = p
	}
	want := PrivilegeDto{Db: "analytics", Actions: []string{"collStats", "find", "listIndexes"}}
	if !reflect.DeepEqual(byCluster[false], want) {
		t.Errorf("collection privilege: got %+v, want %+v", byCluster[false], want)
	}
	wantCluster := PrivilegeDto{Cluster: true, Actions: []string{"replSetGetStatus", "serverStatus"}}
	if !reflect.DeepEqual(byCluster[true], wantCluster) {
		t.Errorf("cluster privilege: got %+v, want %+v", byCluster[true], wantCluster)
	}
}

// TEST-058: DANGER-026 — a config that lists the same actions in a different
// order than state, with cluster unset, plans as a no-op.
func TestDatabaseRole_ActionOrderDoesNotDiff(t *testing.T) {
	res := resourceDatabaseRole()

	// State as Read writes it: MongoDB reports actions in its own order.
	state := res.Data(nil)
	state.SetId(formatResourceId("admin", "explain_role"))
	stateAttrs := map[string]interface{}{
		"database": "admin",
		"name":     "explain_role",
		"privilege": []interface{}{map[string]interface{}{
			"db":         "app",
			"collection": "",
			"cluster":    false,
			"actions":    []string{"collStats", "dbHash", "dbStats", "find", "listCollections", "listIndexes"},
		}},
	}
	for k, v := range stateAttrs {
		if err := state.Set(k, v); err != nil {
			t.Fatalf("Set(%s): %v", k, err)
		}
	}

	// Config as written by hand: cluster unset, actions in another order.
	config := terraform.NewResourceConfigRaw(map[string]interface{}{
		"database": "admin",
		"name":     "explain_role",
		"privilege": []interface{}{map[string]interface{}{
			"db":         "app",
			"collection": "",
			"actions":    []interface{}{"listIndexes", "listCollections", "dbStats", "dbHash", "collStats", "find"},
		}},
	})

	diff, err := res.Diff(context.Background(), state.State(), config, nil)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if diff != nil {
		t.Fatalf("expected no diff for reordered actions, got attributes: %v", diff.Attributes)
	}
}
