package mongodb

import (
	"encoding/base64"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// ZONE-T08..T10: ZONE-014 — identity fields Required (immutable via
// CustomizeDiff, not ForceNew), planned_commands Computed.
func TestZoneKeyRangeSchema(t *testing.T) {
	checkSchemaFields(t, resourceZoneKeyRange(), []schemaField{
		{name: "namespace", typ: schema.TypeString, required: true},
		{name: "zone", typ: schema.TypeString, required: true},
		{name: "min", typ: schema.TypeString, required: true},
		{name: "max", typ: schema.TypeString, required: true},
		{name: "planned_commands", typ: schema.TypeString, computed: true},
	})
}

// ZONE-T11, ZONE-T12: ZONE-015 — namespace must be db.collection
func TestZoneKeyRangeSchema_NamespaceValidation(t *testing.T) {
	checkValidateFunc(t, validateNamespaceDotFormat, "namespace",
		[]interface{}{"mydb.users", "app_db.orders", "a.b"},
		[]interface{}{"noDot", ".leading", "trailing.", ""})
}

// ZONE-T13, ZONE-T14: ZONE-016 — min/max must be JSON
func TestZoneKeyRangeSchema_JSONValidation(t *testing.T) {
	checkValidateFunc(t, validateJSON, "min",
		[]interface{}{`{"region": "east"}`, `{"_id": {"$minKey": 1}}`, `{"x": 0}`},
		[]interface{}{"not json", "{unclosed", ""})
}

// ZONE-T15, ZONE-T16: ZONE-024 — the ID is ns::b64(min)::b64(max) and
// parses back to its parts.
func TestZoneKeyRangeID_RoundTrip(t *testing.T) {
	ns := "app_db.orders"
	minJSON := `{"region": "east"}`
	maxJSON := `{"region": "west"}`

	id := formatZoneKeyRangeID(ns, minJSON, maxJSON)
	want := ns + "::" + base64.StdEncoding.EncodeToString([]byte(minJSON)) +
		"::" + base64.StdEncoding.EncodeToString([]byte(maxJSON))
	if id != want {
		t.Errorf("formatZoneKeyRangeID = %q, want %q", id, want)
	}

	gotNS, gotMin, gotMax, err := parseZoneKeyRangeID(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotNS != ns || gotMin != minJSON || gotMax != maxJSON {
		t.Errorf("parseZoneKeyRangeID = (%q, %q, %q), want (%q, %q, %q)", gotNS, gotMin, gotMax, ns, minJSON, maxJSON)
	}
}

// ZONE-T17: ZONE-024 — parseZoneKeyRangeID rejects invalid IDs
func TestParseZoneKeyRangeID_Invalid(t *testing.T) {
	for _, id := range []string{"", "only-one-part", "two::parts", "::empty-ns::bQ=="} {
		t.Run(id, func(t *testing.T) {
			if _, _, _, err := parseZoneKeyRangeID(id); err == nil {
				t.Errorf("parseZoneKeyRangeID(%q): expected error, got nil", id)
			}
		})
	}
}

// ZONE-T18, ZONE-T19: jsonToBsonD converts valid JSON and rejects the rest
func TestJsonToBsonD(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantLen int
		wantErr bool
	}{
		{"ZONE-T18 valid", `{"region": "east", "zone": 1}`, 2, false},
		{"ZONE-T19 invalid", "not json", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := jsonToBsonD(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error for invalid JSON")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(doc) != tc.wantLen {
				t.Errorf("expected %d elements, got %d", tc.wantLen, len(doc))
			}
		})
	}
}
