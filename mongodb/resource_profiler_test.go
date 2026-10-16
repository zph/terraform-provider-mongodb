package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// PROF-T01, PROF-T06: PROF-001 — field types and defaults (slowms=100,
// ratelimit=1). PROF-T04: DANGER-015 — database Required and immutable via
// CustomizeDiff, not ForceNew. (PROF-T05 ID format: see TestFormatResourceId.)
func TestProfilerSchema(t *testing.T) {
	checkSchemaFields(t, resourceProfiler(), []schemaField{
		{name: "database", typ: schema.TypeString, required: true},
		{name: "level", typ: schema.TypeInt, required: true},
		{name: "slowms", typ: schema.TypeInt, optional: true, def: 100},
		{name: "ratelimit", typ: schema.TypeInt, optional: true, def: 1},
	})
}

// PROF-T02: PROF-002 — level rejects outside [0,2]
// PROF-T03: PROF-003 — slowms rejects negative
func TestProfilerSchema_Validation(t *testing.T) {
	res := resourceProfiler()
	t.Run("level", func(t *testing.T) {
		checkValidateFunc(t, res.Schema["level"].ValidateFunc, "level",
			[]interface{}{0, 1, 2}, []interface{}{-1, 3, 100})
	})
	t.Run("slowms", func(t *testing.T) {
		checkValidateFunc(t, res.Schema["slowms"].ValidateFunc, "slowms",
			[]interface{}{0, 100}, []interface{}{-1})
	})
}
