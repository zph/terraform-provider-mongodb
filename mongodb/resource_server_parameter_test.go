package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// PARAM-T01: PARAM-001 — Schema fields present with correct types
func TestServerParameterSchema_AllFields(t *testing.T) {
	res := resourceServerParameter()
	expected := map[string]schema.ValueType{
		"parameter":   schema.TypeString,
		"value":       schema.TypeString,
		"ignore_read": schema.TypeBool,
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

// PARAM-T02: PARAM-001 — parameter is ForceNew
func TestServerParameterSchema_ParameterForceNew(t *testing.T) {
	res := resourceServerParameter()
	field := res.Schema["parameter"]
	if !field.ForceNew {
		t.Error("parameter should be ForceNew")
	}
	if !field.Required {
		t.Error("parameter should be Required")
	}
}

// PARAM-T03: PARAM-001 — ignore_read defaults to false
func TestServerParameterSchema_IgnoreReadDefault(t *testing.T) {
	res := resourceServerParameter()
	field := res.Schema["ignore_read"]
	if field.Default != false {
		t.Errorf("ignore_read default should be false, got %v", field.Default)
	}
}

// PARAM-T04: PARAM-008 — coerceParameterValue: bool "true"
func TestCoerceParameterValue_BoolTrue(t *testing.T) {
	result := coerceParameterValue("true")
	if result != true {
		t.Errorf("expected bool true, got %v (%T)", result, result)
	}
}

// PARAM-T05: PARAM-008 — coerceParameterValue: bool "false"
func TestCoerceParameterValue_BoolFalse(t *testing.T) {
	result := coerceParameterValue("false")
	if result != false {
		t.Errorf("expected bool false, got %v (%T)", result, result)
	}
}

// PARAM-T06: PARAM-008 — coerceParameterValue: int
func TestCoerceParameterValue_Int(t *testing.T) {
	result := coerceParameterValue("42")
	v, ok := result.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T", result)
	}
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

// PARAM-T07: PARAM-008 — coerceParameterValue: float
func TestCoerceParameterValue_Float(t *testing.T) {
	result := coerceParameterValue("3.14")
	v, ok := result.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T", result)
	}
	if v != 3.14 {
		t.Errorf("expected 3.14, got %f", v)
	}
}

// PARAM-T08: PARAM-008 — coerceParameterValue: plain string
func TestCoerceParameterValue_String(t *testing.T) {
	result := coerceParameterValue("cacheSizeGB=2")
	v, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if v != "cacheSizeGB=2" {
		t.Errorf("expected 'cacheSizeGB=2', got %q", v)
	}
}

// PARAM-T09: PARAM-008 — coerceParameterValue: "1" is int not float
func TestCoerceParameterValue_OneIsInt(t *testing.T) {
	result := coerceParameterValue("1")
	_, ok := result.(int64)
	if !ok {
		t.Errorf("expected int64 for '1', got %T (%v)", result, result)
	}
}

// PARAM-T10: PARAM-007 — ID format correct
func TestServerParameterIdFormat(t *testing.T) {
	id := formatResourceId("admin", "wiredTigerConcurrentReadTransactions")
	if id != "admin.wiredTigerConcurrentReadTransactions" {
		t.Errorf("expected 'admin.wiredTigerConcurrentReadTransactions', got %q", id)
	}
}
