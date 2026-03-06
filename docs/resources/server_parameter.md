# mongodb_server_parameter

~> **EXPERIMENTAL:** This resource requires opt-in via `TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_server_parameter`. The API may change in future releases.

`mongodb_server_parameter` manages MongoDB server parameters via `setParameter`/`getParameter`. This allows tuning runtime parameters that are not exposed through dedicated resources.

~> **IMPORTANT:** Delete is a no-op. Server parameters cannot be unset — destroying this resource only removes it from Terraform state.

## Example Usage

### Simple integer parameter

```hcl
resource "mongodb_server_parameter" "concurrent_reads" {
  parameter = "wiredTigerConcurrentReadTransactions"
  value     = "256"
}
```

### Boolean parameter

```hcl
resource "mongodb_server_parameter" "quiet" {
  parameter = "quiet"
  value     = "true"
}
```

### WiredTiger engine config (with ignore_read)

Some parameters like `wiredTigerEngineRuntimeConfig` accept a config string on write but return a different format on read. Use `ignore_read = true` to skip drift detection for these parameters.

```hcl
resource "mongodb_server_parameter" "wt_cache" {
  parameter   = "wiredTigerEngineRuntimeConfig"
  value       = "cache_size=2G"
  ignore_read = true
}
```

## Argument Reference

* `parameter` - (Required, ForceNew) The name of the server parameter to set. Changing this forces replacement.
* `value` - (Required) The value to set. Always specified as a string; the provider automatically coerces to the appropriate type (bool, int, float, or string) before sending to MongoDB.
* `ignore_read` - (Optional) When `true`, the provider skips `getParameter` on read and trusts the configured value. Use for parameters that return a different format on read than what was written. Default: `false`.

## Value Coercion

The `value` field is always a string in HCL. Before sending to MongoDB, the provider coerces it:

1. `"true"` / `"false"` → boolean
2. Integer string (e.g., `"256"`) → int64
3. Float string (e.g., `"3.14"`) → float64
4. Everything else → string (as-is)

## Delete Behavior

Server parameters cannot be unset via the MongoDB API. When this resource is destroyed, it is removed from Terraform state only — the parameter value remains in MongoDB.

## Import

Server parameters can be imported:

```sh
$ terraform import mongodb_server_parameter.concurrent_reads admin.wiredTigerConcurrentReadTransactions
```
