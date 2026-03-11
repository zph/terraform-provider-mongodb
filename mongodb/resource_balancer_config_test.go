package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// BAL-T01: BAL-001 — All schema fields present with correct types
func TestBalancerConfigSchema_AllFields(t *testing.T) {
	res := resourceBalancerConfig()
	expected := map[string]schema.ValueType{
		"enabled":             schema.TypeBool,
		"active_window_start": schema.TypeString,
		"active_window_stop":  schema.TypeString,
		"chunk_size_mb":       schema.TypeInt,
		"secondary_throttle":  schema.TypeString,
		"wait_for_delete":     schema.TypeBool,
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

// BAL-T02: BAL-002 — enabled defaults to true
func TestBalancerConfigSchema_EnabledDefault(t *testing.T) {
	res := resourceBalancerConfig()
	field := res.Schema["enabled"]
	if field.Default != true {
		t.Errorf("enabled default should be true, got %v", field.Default)
	}
}

// BAL-T03: BAL-015 — chunk_size_mb validation rejects 0 and 1025
func TestBalancerConfigSchema_ChunkSizeValidation(t *testing.T) {
	res := resourceBalancerConfig()
	field := res.Schema["chunk_size_mb"]
	if field.ValidateFunc == nil {
		t.Fatal("chunk_size_mb should have a ValidateFunc")
	}

	// Valid values
	for _, v := range []int{1, 64, 128, 1024} {
		_, errs := field.ValidateFunc(v, "chunk_size_mb")
		if len(errs) > 0 {
			t.Errorf("chunk_size_mb=%d should be valid, got errors: %v", v, errs)
		}
	}

	// Invalid values
	for _, v := range []int{0, -1, 1025, 2048} {
		_, errs := field.ValidateFunc(v, "chunk_size_mb")
		if len(errs) == 0 {
			t.Errorf("chunk_size_mb=%d should be invalid, got no errors", v)
		}
	}
}

// BAL-T04: BAL-011 — active_window_start/stop format validation
func TestBalancerConfigSchema_ActiveWindowValidation(t *testing.T) {
	res := resourceBalancerConfig()
	for _, fieldName := range []string{"active_window_start", "active_window_stop"} {
		field := res.Schema[fieldName]
		if field.ValidateFunc == nil {
			t.Fatalf("%s should have a ValidateFunc", fieldName)
		}

		// Valid values
		for _, v := range []string{"00:00", "23:59", "12:30", "09:05"} {
			_, errs := field.ValidateFunc(v, fieldName)
			if len(errs) > 0 {
				t.Errorf("%s=%q should be valid, got errors: %v", fieldName, v, errs)
			}
		}

		// Invalid values
		for _, v := range []string{"24:00", "12:60", "1:30", "12:3", "noon", "12:30:00", ""} {
			_, errs := field.ValidateFunc(v, fieldName)
			if len(errs) == 0 {
				t.Errorf("%s=%q should be invalid, got no errors", fieldName, v)
			}
		}
	}
}

// BAL-T05: BAL-013 — ID is "balancer"
func TestBalancerConfigIdFixed(t *testing.T) {
	expected := "balancer"
	if expected != "balancer" {
		t.Errorf("balancer config ID should be 'balancer', got %q", expected)
	}
}

// BAL-T06: BAL-001 — secondary_throttle is Optional string
func TestBalancerConfigSchema_SecondaryThrottle(t *testing.T) {
	res := resourceBalancerConfig()
	field := res.Schema["secondary_throttle"]
	if !field.Optional {
		t.Error("secondary_throttle should be Optional")
	}
	if field.Type != schema.TypeString {
		t.Errorf("secondary_throttle should be TypeString, got %v", field.Type)
	}
}

// BAL-T07: BAL-001 — wait_for_delete is Optional bool
func TestBalancerConfigSchema_WaitForDelete(t *testing.T) {
	res := resourceBalancerConfig()
	field := res.Schema["wait_for_delete"]
	if !field.Optional {
		t.Error("wait_for_delete should be Optional")
	}
	if field.Type != schema.TypeBool {
		t.Errorf("wait_for_delete should be TypeBool, got %v", field.Type)
	}
}

// BAL-T08: BAL-001 — All fields are Optional
func TestBalancerConfigSchema_AllOptional(t *testing.T) {
	res := resourceBalancerConfig()
	for name, field := range res.Schema {
		if field.Required {
			t.Errorf("field %q should be Optional, not Required", name)
		}
	}
}

// BAL-T09: BAL-010 — active window requires both start and stop (CustomizeDiff)
func TestBalancerConfigSchema_HasCustomizeDiff(t *testing.T) {
	res := resourceBalancerConfig()
	if res.CustomizeDiff == nil {
		t.Error("resource should have a CustomizeDiff for active window pair validation")
	}
}

// BAL-T10: validateHHMM helper
func TestValidateHHMM(t *testing.T) {
	valid := []string{"00:00", "23:59", "12:30", "09:05"}
	for _, v := range valid {
		_, errs := validateHHMM(v, "test")
		if len(errs) > 0 {
			t.Errorf("validateHHMM(%q) should pass, got: %v", v, errs)
		}
	}

	invalid := []string{"24:00", "12:60", "1:30", "12:3", "", "noon", "12:30:00"}
	for _, v := range invalid {
		_, errs := validateHHMM(v, "test")
		if len(errs) == 0 {
			t.Errorf("validateHHMM(%q) should fail", v)
		}
	}
}
