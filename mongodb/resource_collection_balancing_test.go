package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// CBAL-T01, CBAL-T03, CBAL-T06, CBAL-T07: CBAL-001 — field types, modes and
// the enabled default. CBAL-T02: DANGER-014 — namespace is Required and
// immutable via CustomizeDiff, not ForceNew.
func TestCollectionBalancingSchema(t *testing.T) {
	checkSchemaFields(t, resourceCollectionBalancing(), []schemaField{
		{name: "namespace", typ: schema.TypeString, required: true},
		{name: "enabled", typ: schema.TypeBool, optional: true, def: true},
		{name: "chunk_size_mb", typ: schema.TypeInt, optional: true},
	})
}

// CBAL-T04: CBAL-002 — namespace validation rejects strings without a dot
func TestCollectionBalancingSchema_NamespaceValidation(t *testing.T) {
	checkValidateFunc(t, resourceCollectionBalancing().Schema["namespace"].ValidateFunc, "namespace",
		[]interface{}{"mydb.mycoll", "test.users", "admin.system.roles"},
		[]interface{}{"nodot", "", "."})
}

// CBAL-T08: parseFCVMajor helper
func TestParseFCVMajor(t *testing.T) {
	cases := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"6.0", 6, false},
		{"5.0", 5, false},
		{"7.0", 7, false},
		{"4.4", 4, false},
		{"", 0, true},
		{"abc", 0, true},
		{"6", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseFCVMajor(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("parseFCVMajor(%q) should fail", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFCVMajor(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("parseFCVMajor(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}
