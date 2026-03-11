package mongodb

import (
	"encoding/base64"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// ZONE-T08: ZONE-014 — Schema has all expected fields with correct types
func TestZoneKeyRangeSchema_AllFields(t *testing.T) {
	res := resourceZoneKeyRange()
	expected := map[string]schema.ValueType{
		"namespace":        schema.TypeString,
		"zone":             schema.TypeString,
		"min":              schema.TypeString,
		"max":              schema.TypeString,
		"planned_commands": schema.TypeString,
	}
	for name, typ := range expected {
		field, ok := res.Schema[name]
		if !ok {
			t.Errorf("schema missing field %q", name)
			continue
		}
		if field.Type != typ {
			t.Errorf("field %q: want type %v, got %v", name, typ, field.Type)
		}
	}
}

// ZONE-T09: ZONE-014 — All identity fields are Required and not ForceNew
func TestZoneKeyRangeSchema_IdentityFields(t *testing.T) {
	res := resourceZoneKeyRange()
	for _, name := range []string{"namespace", "zone", "min", "max"} {
		field := res.Schema[name]
		if !field.Required {
			t.Errorf("%s should be Required", name)
		}
		if field.ForceNew {
			t.Errorf("%s should not be ForceNew (DANGER-010); use CustomizeDiff instead", name)
		}
	}
}

// ZONE-T10: ZONE-014 — planned_commands is Computed
func TestZoneKeyRangeSchema_PlannedCommands(t *testing.T) {
	res := resourceZoneKeyRange()
	field := res.Schema["planned_commands"]
	if !field.Computed {
		t.Error("planned_commands should be Computed")
	}
}

// ZONE-T11: ZONE-015 — namespace validation accepts db.collection format
func TestZoneKeyRangeSchema_NamespaceValid(t *testing.T) {
	valid := []string{"mydb.users", "app_db.orders", "a.b"}
	for _, v := range valid {
		_, errs := validateNamespaceDotFormat(v, "namespace")
		if len(errs) > 0 {
			t.Errorf("namespace=%q should be valid, got errors: %v", v, errs)
		}
	}
}

// ZONE-T12: ZONE-015 — namespace validation rejects invalid formats
func TestZoneKeyRangeSchema_NamespaceInvalid(t *testing.T) {
	invalid := []string{"noDot", ".leading", "trailing.", ""}
	for _, v := range invalid {
		_, errs := validateNamespaceDotFormat(v, "namespace")
		if len(errs) == 0 {
			t.Errorf("namespace=%q should be invalid, got no errors", v)
		}
	}
}

// ZONE-T13: ZONE-016 — min/max validation accepts valid JSON
func TestZoneKeyRangeSchema_JSONValid(t *testing.T) {
	valid := []string{
		`{"region": "east"}`,
		`{"_id": {"$minKey": 1}}`,
		`{"x": 0}`,
	}
	for _, v := range valid {
		_, errs := validateJSON(v, "min")
		if len(errs) > 0 {
			t.Errorf("json=%q should be valid, got errors: %v", v, errs)
		}
	}
}

// ZONE-T14: ZONE-016 — min/max validation rejects invalid JSON
func TestZoneKeyRangeSchema_JSONInvalid(t *testing.T) {
	invalid := []string{"not json", "{unclosed", ""}
	for _, v := range invalid {
		_, errs := validateJSON(v, "min")
		if len(errs) == 0 {
			t.Errorf("json=%q should be invalid, got no errors", v)
		}
	}
}

// ZONE-T15: ZONE-024 — formatZoneKeyRangeID produces correct format
func TestFormatZoneKeyRangeID(t *testing.T) {
	ns := "app_db.orders"
	minJSON := `{"region": "east"}`
	maxJSON := `{"region": "west"}`

	id := formatZoneKeyRangeID(ns, minJSON, maxJSON)

	minB64 := base64.StdEncoding.EncodeToString([]byte(minJSON))
	maxB64 := base64.StdEncoding.EncodeToString([]byte(maxJSON))
	want := ns + "::" + minB64 + "::" + maxB64

	if id != want {
		t.Errorf("formatZoneKeyRangeID = %q, want %q", id, want)
	}
}

// ZONE-T16: ZONE-024 — parseZoneKeyRangeID round-trips with formatZoneKeyRangeID
func TestParseZoneKeyRangeID_RoundTrip(t *testing.T) {
	ns := "app_db.orders"
	minJSON := `{"region": "east"}`
	maxJSON := `{"region": "west"}`

	id := formatZoneKeyRangeID(ns, minJSON, maxJSON)
	gotNS, gotMin, gotMax, err := parseZoneKeyRangeID(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotNS != ns {
		t.Errorf("namespace: want %q, got %q", ns, gotNS)
	}
	if gotMin != minJSON {
		t.Errorf("min: want %q, got %q", minJSON, gotMin)
	}
	if gotMax != maxJSON {
		t.Errorf("max: want %q, got %q", maxJSON, gotMax)
	}
}

// ZONE-T17: ZONE-024 — parseZoneKeyRangeID rejects invalid IDs
func TestParseZoneKeyRangeID_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"only-one-part",
		"two::parts",
		"::empty-ns::bQ==",
	}
	for _, id := range invalid {
		_, _, _, err := parseZoneKeyRangeID(id)
		if err == nil {
			t.Errorf("parseZoneKeyRangeID(%q): expected error, got nil", id)
		}
	}
}

// ZONE-T18: jsonToBsonD converts valid JSON to bson.D
func TestJsonToBsonD_Valid(t *testing.T) {
	doc, err := jsonToBsonD(`{"region": "east", "zone": 1}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc) != 2 {
		t.Errorf("expected 2 elements, got %d", len(doc))
	}
}

// ZONE-T19: jsonToBsonD rejects invalid JSON
func TestJsonToBsonD_Invalid(t *testing.T) {
	_, err := jsonToBsonD("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
