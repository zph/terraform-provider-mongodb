# mongodb_zone_key_range

`mongodb_zone_key_range` assigns a shard key range to a zone in a MongoDB sharded cluster. This controls which zone (and therefore which shards) store documents with shard keys falling within the range.

~> **Experimental.** Requires `features_enabled = ["mongodb_zone_key_range"]` in the provider block or the `TERRAFORM_PROVIDER_MONGODB_ENABLE` environment variable.

~> **Mongos required.** This resource must be used with a provider connected to a mongos router.

## Example Usage

### Geographic routing with range-based shard key

```hcl
resource "mongodb_zone_key_range" "orders_east" {
  namespace = "app_db.orders"
  zone      = "US-East"
  min       = jsonencode({"region" = "east-min"})
  max       = jsonencode({"region" = "east-max"})
}
```

### Full key space coverage using $minKey / $maxKey

Use MongoDB's `$minKey` and `$maxKey` special values to cover the entire key space for a shard key. This is the standard pattern for hashed shard keys or when you want to route all data in a range to a specific zone.

```hcl
# Route the lower half of the hash space to US-East
resource "mongodb_zone_key_range" "orders_east" {
  namespace = "app_db.orders"
  zone      = "US-East"
  min       = jsonencode({"_id" = {"$minKey" = 1}})
  max       = jsonencode({"_id" = 0})
}

# Route the upper half of the hash space to US-West
resource "mongodb_zone_key_range" "orders_west" {
  namespace = "app_db.orders"
  zone      = "US-West"
  min       = jsonencode({"_id" = 0})
  max       = jsonencode({"_id" = {"$maxKey" = 1}})
}
```

### Compound shard key

```hcl
resource "mongodb_zone_key_range" "logs_recent" {
  namespace = "analytics.logs"
  zone      = "Hot"
  min       = jsonencode({"tenant" = {"$minKey" = 1}, "timestamp" = {"$minKey" = 1}})
  max       = jsonencode({"tenant" = {"$maxKey" = 1}, "timestamp" = {"$maxKey" = 1}})
}
```

## Argument Reference

* `namespace` - (Required) The namespace in `db.collection` format.
* `zone` - (Required) The zone name to assign the key range to.
* `min` - (Required) Lower bound of the shard key range (inclusive), as a JSON string. Use `{"$minKey": 1}` for the absolute minimum.
* `max` - (Required) Upper bound of the shard key range (exclusive), as a JSON string. Use `{"$maxKey": 1}` for the absolute maximum.

## How It Works

### Create

Parses `min` and `max` from JSON to BSON and runs `updateZoneKeyRange` on the admin database.

### Read

Queries the `config.tags` collection for an exact match on namespace, min, and max bounds. If no matching document is found, the resource is removed from state.

### Update

All fields are identity fields — changes are blocked at plan time.

### Delete

Runs `updateZoneKeyRange` with `zone: null` to remove the range assignment. The `min` and `max` must match exactly.

## Import

Import using the resource ID format `namespace::base64(min)::base64(max)`:

```bash
# The min and max JSON strings must be base64-encoded
terraform import mongodb_zone_key_range.east 'app_db.orders::eyJyZWdpb24iOiAiZWFzdC1taW4ifQ==::eyJyZWdpb24iOiAiZWFzdC1tYXgifQ=='
```

## Limitations

* The `min` and `max` JSON must produce identical BSON documents on every plan. Avoid non-deterministic JSON key ordering if generating min/max dynamically.
* Zone key ranges cannot overlap — MongoDB enforces this server-side.
