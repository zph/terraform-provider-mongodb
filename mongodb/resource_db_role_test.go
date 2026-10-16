package mongodb

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestResourceDatabaseRoleParseId(t *testing.T) {
	checkParseId(t, resourceDatabaseRoleParseId, []parseIdCase{
		{name: "TEST-005 valid", input: "admin.myRole", wantName: "myRole", wantDB: "admin"},
		{name: "TEST-006 no separator", input: "nodotshere", wantErr: true},
		{name: "TEST-006 empty database", input: ".roleName", wantErr: true},
		{name: "TEST-006 empty roleName", input: "database.", wantErr: true},
	})
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
