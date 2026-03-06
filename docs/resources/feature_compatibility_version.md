# mongodb_feature_compatibility_version

~> **EXPERIMENTAL:** This resource requires opt-in via `TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_feature_compatibility_version`. The API may change in future releases.

`mongodb_feature_compatibility_version` manages the `featureCompatibilityVersion` (FCV) setting for a MongoDB deployment. FCV controls which features are available on the cluster and is used during upgrade/downgrade procedures.

**Warning:** Changing FCV is a cluster-wide operation that may be irreversible. Upgrading FCV enables new features that cannot be disabled without a full downgrade. Downgrading FCV may disable features your application depends on. Use `danger_mode = true` to acknowledge this risk.

## Example Usage

### Set FCV (initial)

```hcl
resource "mongodb_feature_compatibility_version" "this" {
  version = "7.0"
}
```

### Change FCV with danger_mode

```hcl
resource "mongodb_feature_compatibility_version" "this" {
  version     = "8.0"
  danger_mode = true
}
```

### Protect against accidental changes

Use `prevent_destroy` to add an additional safety layer:

```hcl
resource "mongodb_feature_compatibility_version" "this" {
  version = "7.0"

  lifecycle {
    prevent_destroy = true
  }
}
```

## Argument Reference

* `version` - (Required) The target featureCompatibilityVersion in `X.Y` format (e.g., `"7.0"`).
* `danger_mode` - (Optional) Must be `true` to allow version changes on an existing resource. Default: `false`. When `false`, any plan that changes `version` is blocked with an error.

## Safety Behavior

The `danger_mode` attribute acts as a safety gate:

1. **Initial create** always proceeds (no gate needed).
2. **Version change with `danger_mode = false`** blocks the plan with an error.
3. **Version change with `danger_mode = true`** proceeds with warnings during apply.
4. **Downgrades** emit an additional warning about potential feature loss.

## Delete Behavior

When this resource is destroyed, no changes are made to MongoDB. FCV always has a value and cannot be unset. Terraform simply removes the resource from state.

## Import

This is a singleton resource. Import using the fixed ID `fcv`:

```sh
$ terraform import mongodb_feature_compatibility_version.this fcv
```
