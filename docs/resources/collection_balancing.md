# mongodb_collection_balancing

~> **EXPERIMENTAL:** This resource requires opt-in via `TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_collection_balancing`. The API may change in future releases.

`mongodb_collection_balancing` manages per-collection balancer settings for a sharded MongoDB collection. It can enable/disable balancing and set a per-collection chunk size override (MongoDB 6.0+).

**Requires a mongos connection.**

## Example Usage

### Disable balancing for a collection

```hcl
resource "mongodb_collection_balancing" "users" {
  namespace = "mydb.users"
  enabled   = false
}
```

### Per-collection chunk size (MongoDB 6.0+)

```hcl
resource "mongodb_collection_balancing" "logs" {
  namespace     = "mydb.logs"
  enabled       = true
  chunk_size_mb = 256
}
```

## Argument Reference

* `namespace` - (Required) The full namespace in `db.collection` format. This field is immutable (changes blocked at plan time).
* `enabled` - (Optional) Whether balancing is enabled for this collection. Default: `true`. When `false`, sets `noBalance: true`.
* `chunk_size_mb` - (Optional) Per-collection chunk size override in megabytes. Only supported on MongoDB 6.0+; ignored with a warning on older versions.

## Version Behavior

On MongoDB 6.0+, this resource uses the `configureCollectionBalancing` admin command. On older versions, it writes directly to `config.collections`.

## Delete Behavior

When this resource is destroyed, balancing is re-enabled for the collection. On 6.0+, chunk size is reset to `0` (cluster default). On older versions, the `noBalance` field is removed.

## Import

Import using the namespace with a `.balancing` suffix:

```sh
$ terraform import mongodb_collection_balancing.users mydb.users.balancing
```
