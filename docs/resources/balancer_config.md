# mongodb_balancer_config

~> **EXPERIMENTAL:** This resource requires opt-in via `TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_balancer_config`. The API may change in future releases.

`mongodb_balancer_config` manages the global balancer configuration for a MongoDB sharded cluster. This resource controls whether the balancer is enabled, its active window, chunk size, and migration throttling.

**Requires a mongos connection.**

## Example Usage

### Disable the balancer

```hcl
resource "mongodb_balancer_config" "this" {
  enabled = false
}
```

### Active window with chunk size

```hcl
resource "mongodb_balancer_config" "this" {
  enabled             = true
  active_window_start = "02:00"
  active_window_stop  = "06:00"
  chunk_size_mb       = 128
}
```

### Full configuration

```hcl
resource "mongodb_balancer_config" "this" {
  enabled             = true
  active_window_start = "01:00"
  active_window_stop  = "05:00"
  chunk_size_mb       = 64
  secondary_throttle  = "majority"
  wait_for_delete     = true
}
```

## Argument Reference

* `enabled` - (Optional) Whether the balancer is enabled. Default: `true`. When `false`, runs `balancerStop`.
* `active_window_start` - (Optional) Start of the balancer active window in `HH:MM` format (24-hour). Must be set together with `active_window_stop`.
* `active_window_stop` - (Optional) End of the balancer active window in `HH:MM` format (24-hour). Must be set together with `active_window_start`.
* `chunk_size_mb` - (Optional) The default chunk size in megabytes. Must be between 1 and 1024.
* `secondary_throttle` - (Optional) The `_secondaryThrottle` write concern for chunk migrations (e.g., `"majority"`, `"1"`).
* `wait_for_delete` - (Optional) Whether the balancer waits for deletion of migrated chunks before starting new migrations.

## Delete Behavior

When this resource is destroyed, the balancer is re-enabled, the active window is cleared, secondary throttle and wait-for-delete settings are unset, and the chunk size document is removed.

## Import

This is a singleton resource. Import using the fixed ID `balancer`:

```sh
$ terraform import mongodb_balancer_config.this balancer
```
