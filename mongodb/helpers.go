package mongodb

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// blockFieldChange returns a CustomizeDiff function that blocks changes to the
// named field on existing resources. This replaces ForceNew semantics with an
// explicit plan-time error instead of silent destroy+recreate. // DANGER-010
func blockFieldChange(fieldName string) schema.CustomizeDiffFunc {
	return func(_ context.Context, d *schema.ResourceDiff, _ interface{}) error {
		if d.Id() == "" {
			return nil
		}
		if d.HasChange(fieldName) {
			return immutableFieldError(fieldName)
		}
		return nil
	}
}

func immutableFieldError(fieldName string) error {
	return fmt.Errorf(
		"changing %q is not allowed on an existing resource; "+
			"remove the resource from state and re-create it instead",
		fieldName,
	)
}

func validateDiagFunc(validateFunc func(interface{}, string) ([]string, []error)) schema.SchemaValidateDiagFunc {
	return func(i interface{}, path cty.Path) diag.Diagnostics {
		warnings, errs := validateFunc(i, fmt.Sprintf("%+v", path))
		var diags diag.Diagnostics
		for _, warning := range warnings {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Warning,
				Summary:  warning,
			})
		}
		for _, err := range errs {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  err.Error(),
			})
		}
		return diags
	}
}

// seedClientSideDefaults writes the schema default of each named attribute
// into state when the attribute has no value yet. An importer calls it for
// attributes MongoDB cannot supply, so that the first plan after an import
// does not record the default as an in-place change. Attributes without a
// default are left alone. SHARD-031, CLUS-017
func seedClientSideDefaults(data *schema.ResourceData, s map[string]*schema.Schema, keys ...string) error {
	for _, k := range keys {
		sch, ok := s[k]
		if !ok || sch.Default == nil {
			continue
		}
		if _, set := data.GetOk(k); set {
			continue
		}
		if err := data.Set(k, sch.Default); err != nil {
			return fmt.Errorf("seeding default for %s: %w", k, err)
		}
	}
	return nil
}
