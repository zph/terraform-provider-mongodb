package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// PROF-T01: PROF-001 — All fields present with correct types
func TestProfilerSchema_AllFields(t *testing.T) {
	res := resourceProfiler()
	expected := map[string]schema.ValueType{
		"database":  schema.TypeString,
		"level":     schema.TypeInt,
		"slowms":    schema.TypeInt,
		"ratelimit": schema.TypeInt,
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

// PROF-T02: PROF-002 — Level validation rejects outside [0,2]
func TestProfilerSchema_LevelValidation(t *testing.T) {
	res := resourceProfiler()
	field := res.Schema["level"]
	if field.ValidateFunc == nil {
		t.Fatal("level should have a ValidateFunc")
	}

	// Valid values
	for _, v := range []int{0, 1, 2} {
		_, errs := field.ValidateFunc(v, "level")
		if len(errs) > 0 {
			t.Errorf("level=%d should be valid, got errors: %v", v, errs)
		}
	}

	// Invalid values
	for _, v := range []int{-1, 3, 100} {
		_, errs := field.ValidateFunc(v, "level")
		if len(errs) == 0 {
			t.Errorf("level=%d should be invalid, got no errors", v)
		}
	}
}

// PROF-T03: PROF-003 — slowms rejects negative
func TestProfilerSchema_SlowmsValidation(t *testing.T) {
	res := resourceProfiler()
	field := res.Schema["slowms"]
	if field.ValidateFunc == nil {
		t.Fatal("slowms should have a ValidateFunc")
	}

	// Valid
	_, errs := field.ValidateFunc(0, "slowms")
	if len(errs) > 0 {
		t.Errorf("slowms=0 should be valid, got errors: %v", errs)
	}
	_, errs = field.ValidateFunc(100, "slowms")
	if len(errs) > 0 {
		t.Errorf("slowms=100 should be valid, got errors: %v", errs)
	}

	// Invalid
	_, errs = field.ValidateFunc(-1, "slowms")
	if len(errs) == 0 {
		t.Error("slowms=-1 should be invalid, got no errors")
	}
}

// PROF-T04: DANGER-015 — database is immutable via CustomizeDiff (not ForceNew)
func TestProfilerSchema_DatabaseNoForceNew(t *testing.T) {
	res := resourceProfiler()
	field := res.Schema["database"]
	if field.ForceNew {
		t.Error("database should not be ForceNew (DANGER-010); use CustomizeDiff instead")
	}
	if !field.Required {
		t.Error("database should be Required")
	}
}

// PROF-T05: PROF-004 — ID format correct
func TestProfilerIdFormat(t *testing.T) {
	id := formatResourceId("mydb", "profiler")
	if id != "mydb.profiler" {
		t.Errorf("expected 'mydb.profiler', got %q", id)
	}
}

// PROF-T06: PROF-001 — Default values (slowms=100, ratelimit=1)
func TestProfilerSchema_Defaults(t *testing.T) {
	res := resourceProfiler()
	if res.Schema["slowms"].Default != 100 {
		t.Errorf("slowms default should be 100, got %v", res.Schema["slowms"].Default)
	}
	if res.Schema["ratelimit"].Default != 1 {
		t.Errorf("ratelimit default should be 1, got %v", res.Schema["ratelimit"].Default)
	}
}
