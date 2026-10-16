package mongodb

import (
	"fmt"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

// schemaField is one row of a resource schema table. Every column is
// asserted exactly, so a zero value means "must be unset".
type schemaField struct {
	name      string
	typ       schema.ValueType
	required  bool
	optional  bool
	computed  bool
	sensitive bool
	def       interface{}
}

func checkSchemaFields(t *testing.T, res *schema.Resource, fields []schemaField) {
	t.Helper()
	for _, f := range fields {
		t.Run(f.name, func(t *testing.T) {
			s, ok := res.Schema[f.name]
			if !ok {
				t.Fatalf("schema missing field %q", f.name)
			}
			if s.Type != f.typ {
				t.Errorf("type: want %v, got %v", f.typ, s.Type)
			}
			if s.Required != f.required {
				t.Errorf("Required: want %v, got %v", f.required, s.Required)
			}
			if s.Optional != f.optional {
				t.Errorf("Optional: want %v, got %v", f.optional, s.Optional)
			}
			if s.Computed != f.computed {
				t.Errorf("Computed: want %v, got %v", f.computed, s.Computed)
			}
			if s.Sensitive != f.sensitive {
				t.Errorf("Sensitive: want %v, got %v", f.sensitive, s.Sensitive)
			}
			if s.Default != f.def {
				t.Errorf("Default: want %v, got %v", f.def, s.Default)
			}
			if s.ForceNew {
				t.Error("ForceNew is banned (DANGER-010); use CustomizeDiff instead")
			}
		})
	}
}

// checkValidateFunc runs a SchemaValidateFunc over values that must pass
// and values that must fail; a nil fn fails the test.
func checkValidateFunc(t *testing.T, fn schema.SchemaValidateFunc, key string, valid, invalid []interface{}) {
	t.Helper()
	if fn == nil {
		t.Fatalf("%s should have a ValidateFunc", key)
	}
	for _, v := range valid {
		t.Run(fmt.Sprintf("valid/%v", v), func(t *testing.T) {
			if _, errs := fn(v, key); len(errs) > 0 {
				t.Errorf("%s=%v should be valid, got errors: %v", key, v, errs)
			}
		})
	}
	for _, v := range invalid {
		t.Run(fmt.Sprintf("invalid/%v", v), func(t *testing.T) {
			if _, errs := fn(v, key); len(errs) == 0 {
				t.Errorf("%s=%v should be invalid, got no errors", key, v)
			}
		})
	}
}

// parseIdCase drives the "database.name" parsers; wantErr cases ignore the
// name and database columns.
type parseIdCase struct {
	name     string
	input    string
	wantName string
	wantDB   string
	wantErr  bool
}

func checkParseId(t *testing.T, parse func(string) (string, string, error), cases []parseIdCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, db, err := parse(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name != tc.wantName {
				t.Errorf("name: want %q, got %q", tc.wantName, name)
			}
			if db != tc.wantDB {
				t.Errorf("database: want %q, got %q", tc.wantDB, db)
			}
		})
	}
}

// TEST-026..029: validateDiagFunc maps warnings to Warning and errors to
// Error diagnostics, in any mix.
func TestValidateDiagFunc(t *testing.T) {
	cases := []struct {
		name         string
		warnings     []string
		errors       []error
		wantWarnings int
		wantErrors   int
	}{
		{"TEST-026 warnings only", []string{"warn1"}, nil, 1, 0},
		{"TEST-027 errors only", nil, []error{fmt.Errorf("bad")}, 0, 1},
		{"TEST-028 mixed", []string{"w1", "w2"}, []error{fmt.Errorf("e1")}, 2, 1},
		{"TEST-029 empty", nil, nil, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn := validateDiagFunc(func(interface{}, string) ([]string, []error) {
				return tc.warnings, tc.errors
			})
			diags := fn("value", cty.Path{})
			if len(diags) != tc.wantWarnings+tc.wantErrors {
				t.Fatalf("expected %d diagnostics, got %d", tc.wantWarnings+tc.wantErrors, len(diags))
			}
			warnings, errors := 0, 0
			for i, d := range diags {
				switch d.Severity {
				case diag.Warning:
					warnings++
					if d.Summary != tc.warnings[i] {
						t.Errorf("warning summary: want %q, got %q", tc.warnings[i], d.Summary)
					}
				case diag.Error:
					errors++
				}
			}
			if warnings != tc.wantWarnings {
				t.Errorf("expected %d warnings, got %d", tc.wantWarnings, warnings)
			}
			if errors != tc.wantErrors {
				t.Errorf("expected %d errors, got %d", tc.wantErrors, errors)
			}
		})
	}
}

// SHARD-031, CLUS-017: seedClientSideDefaults writes schema defaults only
// where state holds nothing, and skips attributes without a default.
func TestSeedClientSideDefaults(t *testing.T) {
	s := resourceShardConfig().Schema
	data := resourceShardConfig().Data(&terraform.InstanceState{
		ID:         "rs0",
		Attributes: map[string]string{"init_timeout_secs": "120"},
	})
	if err := seedClientSideDefaults(data, s, "init_timeout_secs", "host_override", "no_such_attribute"); err != nil {
		t.Fatalf("seedClientSideDefaults returned error: %v", err)
	}
	if v := data.Get("init_timeout_secs").(int); v != 120 {
		t.Errorf("init_timeout_secs = %d, want the existing 120 kept", v)
	}
	if _, set := data.GetOk("host_override"); set {
		t.Errorf("host_override has no default and must stay unset, got %q", data.Get("host_override"))
	}

	data = resourceShardConfig().Data(&terraform.InstanceState{ID: "rs0"})
	if err := seedClientSideDefaults(data, s, "init_timeout_secs"); err != nil {
		t.Fatalf("seedClientSideDefaults returned error: %v", err)
	}
	if v := data.Get("init_timeout_secs").(int); v != DefaultInitTimeoutSecs {
		t.Errorf("init_timeout_secs = %d, want default %d", v, DefaultInitTimeoutSecs)
	}
}
