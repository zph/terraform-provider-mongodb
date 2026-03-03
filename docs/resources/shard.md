# mongodb_shard

`mongodb_shard` manages adding and removing shards from a MongoDB sharded cluster. This resource runs `addShard` on create and `removeShard` on delete against a mongos router.

~> **IMPORTANT:** The provider must be connected to a **mongos** router for this resource to work. It will not function against a direct replica set connection.

~> **IMPORTANT:** Both `shard_name` and `hosts` are ForceNew. Changing either attribute forces replacement (remove + re-add).

## Example Usage

### Basic shard registration

```hcl
resource "mongodb_shard" "shard01" {
  shard_name = "shard01"
  hosts      = ["mongo1:27017", "mongo2:27017", "mongo3:27017"]
}
```

### With custom remove timeout

```hcl
resource "mongodb_shard" "shard02" {
  shard_name          = "shard02"
  hosts               = ["mongo4:27017", "mongo5:27017", "mongo6:27017"]
  remove_timeout_secs = 600
}
```

## Argument Reference

* `shard_name` - (Required, ForceNew) The replica set name of the shard to add.
* `hosts` - (Required, ForceNew) List of `host:port` addresses for the shard replica set members.
* `remove_timeout_secs` - (Optional) Timeout in seconds for shard removal (draining). Default: `300`.

## Attribute Reference

* `state` - (Computed) The state of the shard as reported by `listShards`.

## How It Works

### Create

1. Builds a connection string: `shard_name/host1:port,host2:port,host3:port`
2. Runs `addShard` against the mongos router
3. Reads back the shard state via `listShards`

### Read

1. Runs `listShards` against the mongos router
2. Finds the shard by `_id` matching `shard_name`
3. Updates the `state` attribute
4. If the shard is not found, removes the resource from state

### Delete

1. Runs `removeShard` against the mongos router
2. Polls every 5 seconds until the state is `"completed"` or the timeout is reached
3. Returns an error if the timeout is exceeded

## Known Limitations

* **No update support:** Both `shard_name` and `hosts` are ForceNew. Any change destroys and re-creates the shard registration.
* **Draining can be slow:** The `removeShard` operation drains data from the shard, which can take a long time for large datasets. Adjust `remove_timeout_secs` accordingly.
