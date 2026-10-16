package mongodb

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestBuildShardConnectionString(t *testing.T) {
	cases := []struct {
		name  string
		shard string
		hosts []string
		want  string
	}{
		// CLUS-T01
		{"three hosts", "shard01", []string{"mongo1:27017", "mongo2:27017", "mongo3:27017"}, "shard01/mongo1:27017,mongo2:27017,mongo3:27017"},
		// CLUS-T02
		{"single host", "rs0", []string{"localhost:27018"}, "rs0/localhost:27018"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildShardConnectionString(tc.shard, tc.hosts); got != tc.want {
				t.Errorf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestShardSchema(t *testing.T) {
	cases := []struct {
		field string
		check func(t *testing.T, field *schema.Schema)
	}{
		{
			// CLUS-T03: shard_name Required, ForceNew
			field: "shard_name",
			check: func(t *testing.T, field *schema.Schema) {
				if !field.Required {
					t.Error("shard_name should be Required")
				}
				if !field.ForceNew {
					t.Error("shard_name should be ForceNew")
				}
			},
		},
		{
			// CLUS-T04: DANGER-016 — hosts Required, TypeList, immutable via CustomizeDiff (not ForceNew)
			// CLUS-016: hosts carries a diff suppressor for reorderings
			field: "hosts",
			check: func(t *testing.T, field *schema.Schema) {
				if !field.Required {
					t.Error("hosts should be Required")
				}
				if field.Type != schema.TypeList {
					t.Errorf("hosts type: want TypeList, got %v", field.Type)
				}
				if field.ForceNew {
					t.Error("hosts should not be ForceNew (DANGER-010); use CustomizeDiff instead")
				}
				if field.DiffSuppressFunc == nil {
					t.Fatal("hosts should have a DiffSuppressFunc")
				}
			},
		},
		{
			// CLUS-T05: state Computed
			field: "state",
			check: func(t *testing.T, field *schema.Schema) {
				if !field.Computed {
					t.Error("state should be Computed")
				}
			},
		},
		{
			// CLUS-T06: remove_timeout_secs Optional, Default 300
			field: "remove_timeout_secs",
			check: func(t *testing.T, field *schema.Schema) {
				if field.Required {
					t.Error("remove_timeout_secs should be Optional, not Required")
				}
				if field.Default != DefaultRemoveTimeoutSecs {
					t.Errorf("remove_timeout_secs default: want %d, got %v", DefaultRemoveTimeoutSecs, field.Default)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			field, ok := resourceShard().Schema[tc.field]
			if !ok {
				t.Fatalf("schema missing %q field", tc.field)
			}
			tc.check(t, field)
		})
	}
}

func TestIsReadPreferenceError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		// CLUS-T06a: FailedToSatisfyReadPreference (code 133)
		{"code 133", mongo.CommandError{Code: MongoErrFailedReadPreference, Message: "Could not find host matching read preference"}, true},
		// CLUS-T06b
		{"unrelated code", mongo.CommandError{Code: 99, Message: "other error"}, false},
		// CLUS-T06c
		{"wrapped", errors.Wrap(mongo.CommandError{Code: MongoErrFailedReadPreference, Message: "Could not find host"}, "addShard"), true},
		// CLUS-T06d: string match (connection error)
		{"string match", fmt.Errorf("addShard failed: (FailedToSatisfyReadPreference) Could not find host matching read preference"), true},
		// CLUS-T06e
		{"plain error", fmt.Errorf("network timeout"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsReadPreferenceError(tc.err); got != tc.want {
				t.Errorf("IsReadPreferenceError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// CLUS-016: the same hosts in another order are not a change
func TestSameHostSet(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{"same order", []string{"a:1", "b:1"}, []string{"a:1", "b:1"}, true},
		{"reordered", []string{"a:1", "b:1", "c:1"}, []string{"c:1", "a:1", "b:1"}, true},
		{"both empty", nil, []string{}, true},
		{"different host", []string{"a:1", "b:1"}, []string{"a:1", "c:1"}, false},
		{"one missing", []string{"a:1", "b:1"}, []string{"a:1"}, false},
		{"state empty on import", nil, []string{"a:1"}, false},
		{"duplicate differs", []string{"a:1", "a:1"}, []string{"a:1", "b:1"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameHostSet(tc.a, tc.b); got != tc.want {
				t.Errorf("sameHostSet(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// CLUS-016: suppressHostsReorder reads the whole list, not the one element
func TestSuppressHostsReorder(t *testing.T) {
	res := resourceShard()
	prior := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"shard_name": "rs0",
		"hosts":      []interface{}{"a:27017", "b:27017"},
	})
	prior.SetId("rs0")

	cases := []struct {
		name    string
		hosts   []interface{}
		blocked bool // CustomizeDiff rejects the change at plan time
	}{
		{"reordered", []interface{}{"b:27017", "a:27017"}, false},
		{"changed", []interface{}{"a:27017", "c:27017"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := map[string]interface{}{"shard_name": "rs0", "hosts": tc.hosts}
			diff, err := res.Diff(context.Background(), prior.State(), terraform.NewResourceConfigRaw(config), nil)
			if tc.blocked {
				if err == nil {
					t.Fatal("expected the hosts change to be blocked at plan time")
				}
				return
			}
			if err != nil {
				t.Fatalf("Diff returned error: %v", err)
			}
			if diff != nil && len(diff.Attributes) > 0 {
				t.Errorf("expected no diff for reordered hosts, got %v", diff.Attributes)
			}
		})
	}
}

// CLUS-015: stringList tolerates a nil list and non-string elements
func TestStringList(t *testing.T) {
	if got := stringList(nil); len(got) != 0 {
		t.Errorf("expected empty for nil, got %v", got)
	}
	got := stringList([]interface{}{"a:1", nil, "b:1"})
	if len(got) != 2 || got[0] != "a:1" || got[1] != "b:1" {
		t.Errorf("unexpected result %v", got)
	}
}

// CLUS-017: Import seeds the client-side timeouts with their defaults so the
// first plan after an import does not record them as a change.
func TestShardImport_SeedsClientSideDefaults(t *testing.T) {
	data := resourceShard().Data(&terraform.InstanceState{ID: "rs0"})
	imported, err := RShard.Import(context.Background(), data, nil)
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(imported) != 1 {
		t.Fatalf("Import returned %d states, want 1", len(imported))
	}
	got := imported[0]
	if got.Id() != "rs0" {
		t.Errorf("id = %q, want rs0", got.Id())
	}
	if v := got.Get("add_timeout_secs").(int); v != DefaultAddTimeoutSecs {
		t.Errorf("add_timeout_secs = %d, want %d", v, DefaultAddTimeoutSecs)
	}
	if v := got.Get("remove_timeout_secs").(int); v != DefaultRemoveTimeoutSecs {
		t.Errorf("remove_timeout_secs = %d, want %d", v, DefaultRemoveTimeoutSecs)
	}
	if _, set := got.GetOk("hosts"); set {
		t.Errorf("hosts must be left for Read to fill, got %v", got.Get("hosts"))
	}
}
