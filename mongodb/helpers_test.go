package mongodb

import (
	"fmt"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

// TEST-026: Warnings propagated as DiagWarning
func TestValidateDiagFunc_WarningsOnly(t *testing.T) {
	fn := validateDiagFunc(func(i interface{}, s string) ([]string, []error) {
		return []string{"warn1"}, nil
	})
	diags := fn("value", cty.Path{})
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Severity != diag.Warning {
		t.Errorf("expected Warning severity, got %v", diags[0].Severity)
	}
	if diags[0].Summary != "warn1" {
		t.Errorf("expected summary 'warn1', got %q", diags[0].Summary)
	}
}

// TEST-027: Errors propagated as DiagError
func TestValidateDiagFunc_ErrorsOnly(t *testing.T) {
	fn := validateDiagFunc(func(i interface{}, s string) ([]string, []error) {
		return nil, []error{fmt.Errorf("bad")}
	})
	diags := fn("value", cty.Path{})
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Severity != diag.Error {
		t.Errorf("expected Error severity, got %v", diags[0].Severity)
	}
}

// TEST-028: Mixed warnings and errors produce correct diagnostics
func TestValidateDiagFunc_Mixed(t *testing.T) {
	fn := validateDiagFunc(func(i interface{}, s string) ([]string, []error) {
		return []string{"w1", "w2"}, []error{fmt.Errorf("e1")}
	})
	diags := fn("value", cty.Path{})
	if len(diags) != 3 {
		t.Fatalf("expected 3 diagnostics, got %d", len(diags))
	}
	warnings := 0
	errors := 0
	for _, d := range diags {
		switch d.Severity {
		case diag.Warning:
			warnings++
		case diag.Error:
			errors++
		}
	}
	if warnings != 2 {
		t.Errorf("expected 2 warnings, got %d", warnings)
	}
	if errors != 1 {
		t.Errorf("expected 1 error, got %d", errors)
	}
}

// TEST-029: No warnings or errors produces empty diagnostics
func TestValidateDiagFunc_Empty(t *testing.T) {
	fn := validateDiagFunc(func(i interface{}, s string) ([]string, []error) {
		return nil, nil
	})
	diags := fn("value", cty.Path{})
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics, got %d", len(diags))
	}
}
