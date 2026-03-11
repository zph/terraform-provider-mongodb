package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// CBAL-T01: CBAL-001 — All schema fields present with correct types
func TestCollectionBalancingSchema_AllFields(t *testing.T) {
	res := resourceCollectionBalancing()
	expected := map[string]schema.ValueType{
		"namespace":     schema.TypeString,
		"enabled":       schema.TypeBool,
		"chunk_size_mb": schema.TypeInt,
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

// CBAL-T02: DANGER-014 — namespace is Required, immutable via CustomizeDiff (not ForceNew)
func TestCollectionBalancingSchema_NamespaceNoForceNew(t *testing.T) {
	res := resourceCollectionBalancing()
	field := res.Schema["namespace"]
	if !field.Required {
		t.Error("namespace should be Required")
	}
	if field.ForceNew {
		t.Error("namespace should not be ForceNew (DANGER-010); use CustomizeDiff instead")
	}
}

// CBAL-T03: CBAL-001 — enabled defaults to true
func TestCollectionBalancingSchema_EnabledDefault(t *testing.T) {
	res := resourceCollectionBalancing()
	field := res.Schema["enabled"]
	if field.Default != true {
		t.Errorf("enabled default should be true, got %v", field.Default)
	}
}

// CBAL-T04: CBAL-002 — namespace validation rejects strings without a dot
func TestCollectionBalancingSchema_NamespaceValidation(t *testing.T) {
	res := resourceCollectionBalancing()
	field := res.Schema["namespace"]
	if field.ValidateFunc == nil {
		t.Fatal("namespace should have a ValidateFunc")
	}

	// Valid values
	for _, v := range []string{"mydb.mycoll", "test.users", "admin.system.roles"} {
		_, errs := field.ValidateFunc(v, "namespace")
		if len(errs) > 0 {
			t.Errorf("namespace=%q should be valid, got errors: %v", v, errs)
		}
	}

	// Invalid values
	for _, v := range []string{"nodot", "", "."} {
		_, errs := field.ValidateFunc(v, "namespace")
		if len(errs) == 0 {
			t.Errorf("namespace=%q should be invalid, got no errors", v)
		}
	}
}

// CBAL-T05: CBAL-009 — ID format correct
func TestCollectionBalancingIdFormat(t *testing.T) {
	id := formatResourceId("mydb.mycollection", "balancing")
	if id != "mydb.mycollection.balancing" {
		t.Errorf("expected 'mydb.mycollection.balancing', got %q", id)
	}
}

// CBAL-T06: CBAL-001 — chunk_size_mb is Optional
func TestCollectionBalancingSchema_ChunkSizeOptional(t *testing.T) {
	res := resourceCollectionBalancing()
	field := res.Schema["chunk_size_mb"]
	if !field.Optional {
		t.Error("chunk_size_mb should be Optional")
	}
	if field.Required {
		t.Error("chunk_size_mb should not be Required")
	}
}

// CBAL-T07: CBAL-001 — enabled is Optional
func TestCollectionBalancingSchema_EnabledOptional(t *testing.T) {
	res := resourceCollectionBalancing()
	field := res.Schema["enabled"]
	if !field.Optional {
		t.Error("enabled should be Optional")
	}
}

// CBAL-T08: parseFCVMajor helper
func TestParseFCVMajor(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"6.0", 6},
		{"5.0", 5},
		{"7.0", 7},
		{"4.4", 4},
	}
	for _, tc := range cases {
		got, err := parseFCVMajor(tc.input)
		if err != nil {
			t.Errorf("parseFCVMajor(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseFCVMajor(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}

	// Invalid
	for _, v := range []string{"", "abc", "6"} {
		_, err := parseFCVMajor(v)
		if err == nil {
			t.Errorf("parseFCVMajor(%q) should fail", v)
		}
	}
}
