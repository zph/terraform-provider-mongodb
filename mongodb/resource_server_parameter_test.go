package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// PARAM-T01, PARAM-T03: PARAM-001 — field types and the ignore_read default.
// PARAM-T02: DANGER-013 — parameter Required and immutable via CustomizeDiff,
// not ForceNew. (PARAM-T10 ID format: see TestFormatResourceId.)
func TestServerParameterSchema(t *testing.T) {
	checkSchemaFields(t, resourceServerParameter(), []schemaField{
		{name: "parameter", typ: schema.TypeString, required: true},
		{name: "value", typ: schema.TypeString, required: true},
		{name: "ignore_read", typ: schema.TypeBool, optional: true, def: false},
	})
}

// PARAM-008: coerceParameterValue picks the narrowest of bool, int64,
// float64, string.
func TestCoerceParameterValue(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  interface{}
	}{
		{"PARAM-T04 bool true", "true", true},
		{"PARAM-T05 bool false", "false", false},
		{"PARAM-T06 int", "42", int64(42)},
		{"PARAM-T07 float", "3.14", 3.14},
		{"PARAM-T08 plain string", "cacheSizeGB=2", "cacheSizeGB=2"},
		{"PARAM-T09 one is int not float", "1", int64(1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Interface equality covers both dynamic type and value.
			if got := coerceParameterValue(tc.input); got != tc.want {
				t.Errorf("coerceParameterValue(%q) = %v (%T), want %v (%T)", tc.input, got, got, tc.want, tc.want)
			}
		})
	}
}
