package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// FCV-T01: FCV-001 — Schema has version (TypeString, Required) and danger_mode (TypeBool, Optional, Default false)
func TestFCVSchema_AllFields(t *testing.T) {
	res := resourceFCV()
	expected := map[string]schema.ValueType{
		"version":     schema.TypeString,
		"danger_mode": schema.TypeBool,
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

	// version must be Required
	if !res.Schema["version"].Required {
		t.Error("version should be Required")
	}

	// danger_mode must be Optional
	if !res.Schema["danger_mode"].Optional {
		t.Error("danger_mode should be Optional")
	}
}

// FCV-T02: FCV-002 — version ValidateFunc accepts/rejects correct patterns
func TestFCVSchema_VersionValidation(t *testing.T) {
	res := resourceFCV()
	field := res.Schema["version"]
	if field.ValidateFunc == nil {
		t.Fatal("version should have a ValidateFunc")
	}

	// Valid values
	for _, v := range []string{"6.0", "7.0", "10.0"} {
		_, errs := field.ValidateFunc(v, "version")
		if len(errs) > 0 {
			t.Errorf("version=%q should be valid, got errors: %v", v, errs)
		}
	}

	// Invalid values
	for _, v := range []string{"", "abc", "6", "6.0.1"} {
		_, errs := field.ValidateFunc(v, "version")
		if len(errs) == 0 {
			t.Errorf("version=%q should be invalid, got no errors", v)
		}
	}
}

// FCV-T03: FCV-001 — danger_mode Default is false
func TestFCVSchema_DangerModeDefault(t *testing.T) {
	res := resourceFCV()
	field := res.Schema["danger_mode"]
	if field.Default != false {
		t.Errorf("danger_mode default should be false, got %v", field.Default)
	}
}

// FCV-T04: FCV-001 — CustomizeDiff is non-nil
func TestFCVSchema_CustomizeDiffPresent(t *testing.T) {
	res := resourceFCV()
	if res.CustomizeDiff == nil {
		t.Error("CustomizeDiff should be non-nil")
	}
}

// FCV-T05: FCV-012 — compareFCV("7.0", "6.0") → +1
func TestCompareFCV_GreaterMajor(t *testing.T) {
	got, err := compareFCV("7.0", "6.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1 {
		t.Errorf("compareFCV(7.0, 6.0) = %d, want 1", got)
	}
}

// FCV-T06: FCV-012 — compareFCV("6.0", "7.0") → -1
func TestCompareFCV_LesserMajor(t *testing.T) {
	got, err := compareFCV("6.0", "7.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != -1 {
		t.Errorf("compareFCV(6.0, 7.0) = %d, want -1", got)
	}
}

// FCV-T07: FCV-012 — compareFCV("7.0", "7.0") → 0
func TestCompareFCV_Equal(t *testing.T) {
	got, err := compareFCV("7.0", "7.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("compareFCV(7.0, 7.0) = %d, want 0", got)
	}
}

// FCV-T08: FCV-012 — compareFCV("7.1", "7.0") → +1 (minor)
func TestCompareFCV_GreaterMinor(t *testing.T) {
	got, err := compareFCV("7.1", "7.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1 {
		t.Errorf("compareFCV(7.1, 7.0) = %d, want 1", got)
	}
}

// FCV-T09: FCV-012 — compareFCV("invalid", "7.0") → error
func TestCompareFCV_InvalidInput(t *testing.T) {
	_, err := compareFCV("invalid", "7.0")
	if err == nil {
		t.Error("compareFCV(invalid, 7.0) should return error")
	}

	_, err = compareFCV("7.0", "invalid")
	if err == nil {
		t.Error("compareFCV(7.0, invalid) should return error")
	}
}

// FCV-T10: FCV-003 — fcvResourceID constant equals "fcv"
func TestFCVResourceID(t *testing.T) {
	if fcvResourceID != "fcv" {
		t.Errorf("fcvResourceID = %q, want %q", fcvResourceID, "fcv")
	}
}
