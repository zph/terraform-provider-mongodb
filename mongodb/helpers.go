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
			return fmt.Errorf(
				"changing %q is not allowed on an existing resource; "+
					"remove the resource from state and re-create it instead",
				fieldName,
			)
		}
		return nil
	}
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
