# mongodb_profiler

~> **EXPERIMENTAL:** This resource requires opt-in via `TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_profiler`. The API may change in future releases.

`mongodb_profiler` manages the database profiler configuration for a MongoDB database. The profiler captures slow operations for analysis.

## Example Usage

### Basic — enable profiling for slow queries

```hcl
resource "mongodb_profiler" "mydb" {
  database = "mydb"
  level    = 1
  slowms   = 200
}
```

### Full profiling (all operations)

```hcl
resource "mongodb_profiler" "mydb_full" {
  database  = "mydb"
  level     = 2
  slowms    = 50
  ratelimit = 10
}
```

## Argument Reference

* `database` - (Required, ForceNew) The name of the database to configure profiling on. Changing this forces replacement.
* `level` - (Required) The profiling level. `0` = off, `1` = slow operations only, `2` = all operations.
* `slowms` - (Optional) The threshold in milliseconds for slow operations. Default: `100`.
* `ratelimit` - (Optional) The fraction of slow operations to profile (Percona Server only). Default: `1`.

## Delete Behavior

When this resource is destroyed, profiling is disabled (`level = 0`) on the target database.

## Import

Profiler configuration can be imported using the database name:

```sh
$ terraform import mongodb_profiler.mydb mydb.profiler
```
