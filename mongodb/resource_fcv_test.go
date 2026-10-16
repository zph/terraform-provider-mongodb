package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// FCV-T01, FCV-T03: FCV-001 — version Required string; danger_mode Optional
// bool defaulting to false. (FCV-T04 CustomizeDiff: see
// TestDangerousOps_CustomizeDiffPresent.)
func TestFCVSchema(t *testing.T) {
	checkSchemaFields(t, resourceFCV(), []schemaField{
		{name: "version", typ: schema.TypeString, required: true},
		{name: "danger_mode", typ: schema.TypeBool, optional: true, def: false},
	})
}

// FCV-T02: FCV-002 — version ValidateFunc accepts/rejects correct patterns
func TestFCVSchema_VersionValidation(t *testing.T) {
	checkValidateFunc(t, resourceFCV().Schema["version"].ValidateFunc, "version",
		[]interface{}{"6.0", "7.0", "10.0"}, []interface{}{"", "abc", "6", "6.0.1"})
}

// FCV-012: compareFCV orders by major then minor and rejects bad input.
func TestCompareFCV(t *testing.T) {
	cases := []struct {
		name    string
		a, b    string
		want    int
		wantErr bool
	}{
		{"FCV-T05 greater major", "7.0", "6.0", 1, false},
		{"FCV-T06 lesser major", "6.0", "7.0", -1, false},
		{"FCV-T07 equal", "7.0", "7.0", 0, false},
		{"FCV-T08 greater minor", "7.1", "7.0", 1, false},
		{"FCV-T09 invalid left", "invalid", "7.0", 0, true},
		{"FCV-T09 invalid right", "7.0", "invalid", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := compareFCV(tc.a, tc.b)
			if tc.wantErr {
				if err == nil {
					t.Errorf("compareFCV(%s, %s) should return error", tc.a, tc.b)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("compareFCV(%s, %s) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// FCV-T10: FCV-003 — fcvResourceID constant equals "fcv"
func TestFCVResourceID(t *testing.T) {
	if fcvResourceID != "fcv" {
		t.Errorf("fcvResourceID = %q, want %q", fcvResourceID, "fcv")
	}
}
