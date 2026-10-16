package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var (
	validHHMM   = []interface{}{"00:00", "23:59", "12:30", "09:05"}
	invalidHHMM = []interface{}{"24:00", "12:60", "1:30", "12:3", "noon", "12:30:00", ""}
)

// BAL-T01, BAL-T02, BAL-T06, BAL-T07: BAL-001/002 — field types, modes and
// the enabled default. BAL-T08: every field is Optional (planned_commands is
// Computed).
func TestBalancerConfigSchema(t *testing.T) {
	res := resourceBalancerConfig()
	checkSchemaFields(t, res, []schemaField{
		{name: "enabled", typ: schema.TypeBool, optional: true, def: true},
		{name: "active_window_start", typ: schema.TypeString, optional: true},
		{name: "active_window_stop", typ: schema.TypeString, optional: true},
		{name: "chunk_size_mb", typ: schema.TypeInt, optional: true},
		{name: "secondary_throttle", typ: schema.TypeString, optional: true},
		{name: "wait_for_delete", typ: schema.TypeBool, optional: true},
		{name: "planned_commands", typ: schema.TypeString, computed: true},
	})
	for name, field := range res.Schema {
		if field.Required {
			t.Errorf("field %q should be Optional, not Required", name)
		}
	}
}

// BAL-T03: BAL-015 — chunk_size_mb rejects 0 and 1025
// BAL-T04: BAL-011 — active_window_start/stop format validation
func TestBalancerConfigSchema_Validation(t *testing.T) {
	res := resourceBalancerConfig()
	t.Run("chunk_size_mb", func(t *testing.T) {
		checkValidateFunc(t, res.Schema["chunk_size_mb"].ValidateFunc, "chunk_size_mb",
			[]interface{}{1, 64, 128, 1024}, []interface{}{0, -1, 1025, 2048})
	})
	for _, name := range []string{"active_window_start", "active_window_stop"} {
		t.Run(name, func(t *testing.T) {
			checkValidateFunc(t, res.Schema[name].ValidateFunc, name, validHHMM, invalidHHMM)
		})
	}
}

// BAL-T10: validateHHMM helper
func TestValidateHHMM(t *testing.T) {
	checkValidateFunc(t, validateHHMM, "test", validHHMM, invalidHHMM)
}

// BAL-T05: BAL-013 — ID is "balancer"
func TestBalancerConfigIdFixed(t *testing.T) {
	if balancerResourceID != "balancer" {
		t.Errorf("balancer config ID should be 'balancer', got %q", balancerResourceID)
	}
}
