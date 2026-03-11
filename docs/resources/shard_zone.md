# mongodb_shard_zone

`mongodb_shard_zone` maps a shard to a zone name in a MongoDB sharded cluster. Zones control which shards store which data based on zone key ranges.

~> **Experimental.** Requires `features_enabled = ["mongodb_shard_zone"]` in the provider block or the `TERRAFORM_PROVIDER_MONGODB_ENABLE` environment variable.

~> **Mongos required.** This resource must be used with a provider connected to a mongos router.

## Example Usage

### Assign shards to geographic zones

```hcl
resource "mongodb_shard_zone" "shard01_east" {
  shard_name = "shard01"
  zone       = "US-East"
}

resource "mongodb_shard_zone" "shard02_west" {
  shard_name = "shard02"
  zone       = "US-West"
}
```

### Multi-zone shard (shard participates in multiple zones)

```hcl
resource "mongodb_shard_zone" "shard01_east" {
  shard_name = "shard01"
  zone       = "US-East"
}

resource "mongodb_shard_zone" "shard01_backup" {
  shard_name = "shard01"
  zone       = "Backup"
}
```

## Argument Reference

* `shard_name` - (Required) The name of the shard to associate with the zone.
* `zone` - (Required) The zone name to assign to the shard.

## How It Works

### Create

Runs `addShardToZone` on the admin database. This command is idempotent.

### Read

Queries the `config.shards` collection and checks the `tags` array contains the zone. If the shard or zone assignment no longer exists, the resource is removed from state.

### Update

All fields are identity fields — changes are blocked at plan time.

### Delete

Runs `removeShardFromZone` on the admin database. This only removes the shard-to-zone association; it does not remove zone key ranges.

## Import

Import using `shard_name:zone` format:

```bash
terraform import mongodb_shard_zone.east shard01:US-East
```
