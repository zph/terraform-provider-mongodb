# mongodb_shard

`mongodb_shard` manages adding and removing shards from a MongoDB sharded cluster. This resource runs [`addShard`](https://www.mongodb.com/docs/manual/reference/command/addShard/) on create and [`removeShard`](https://www.mongodb.com/docs/manual/reference/command/removeShard/) on delete against a mongos router.

~> **IMPORTANT:** The provider must be connected to a **mongos** router for this resource to work. It will not function against a direct replica set connection.

~> **IMPORTANT:** Both `shard_name` and `hosts` are ForceNew. Changing either attribute forces replacement (remove + re-add).

## Example Usage

### Basic shard registration

```hcl
provider "mongodb" {
  host     = "mongos.example.com"
  port     = "27017"
  username = "admin"
  password = var.mongo_password
}

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

### Multiple shards

```hcl
resource "mongodb_shard" "shard01" {
  shard_name = "shard01"
  hosts      = ["shard01-a:27018", "shard01-b:27018", "shard01-c:27018"]
}

resource "mongodb_shard" "shard02" {
  shard_name = "shard02"
  hosts      = ["shard02-a:27018", "shard02-b:27018", "shard02-c:27018"]
}

resource "mongodb_shard" "shard03" {
  shard_name          = "shard03"
  hosts               = ["shard03-a:27018", "shard03-b:27018", "shard03-c:27018"]
  remove_timeout_secs = 600
}
```

## Argument Reference

* `shard_name` - (Required, ForceNew) The replica set name of the shard to add.
* `hosts` - (Required, ForceNew) List of `host:port` addresses for the shard replica set members.
* `remove_timeout_secs` - (Optional) Timeout in seconds for shard removal (draining). Default: `300`.

## Attribute Reference

* `state` - (Computed) The state of the shard as reported by [`listShards`](https://www.mongodb.com/docs/manual/reference/command/listShards/). This is an integer matching the shard's `state` field in the `listShards` response (typically `1` for active).

## How It Works

### Create (`addShard`)

1. Builds a connection string from `shard_name` and `hosts`: `shard01/host1:port,host2:port,host3:port`
2. Runs [`addShard`](https://www.mongodb.com/docs/manual/reference/command/addShard/) against the mongos router with the connection string
3. Sets the resource ID to the `shard_name`
4. Reads back the shard state via `listShards`

The `addShard` command registers the replica set with the sharded cluster. The shard's replica set must already be initialized before calling `addShard`.

### Read (`listShards`)

1. Runs [`listShards`](https://www.mongodb.com/docs/manual/reference/command/listShards/) against the mongos router
2. Searches the response for a shard whose `_id` matches `shard_name`
3. Updates the `state` attribute from the shard's `state` field
4. If the shard is not found, removes the resource from Terraform state (the shard was removed outside of Terraform)

### Update

Not supported. Both `shard_name` and `hosts` are ForceNew, so any change triggers a destroy + create cycle.

### Delete (`removeShard`)

1. Runs [`removeShard`](https://www.mongodb.com/docs/manual/reference/command/removeShard/) against the mongos router
2. MongoDB begins draining data from the shard (moving chunks to other shards)
3. Polls `removeShard` every 5 seconds, checking the `state` field in the response
4. When `state` is `"completed"`, the shard has been fully removed
5. Returns an error if `remove_timeout_secs` is exceeded before completion

The `removeShard` operation is asynchronous in MongoDB. The first call initiates draining, and subsequent calls return progress. The resource polls until MongoDB reports the removal is complete.

## Import

MongoDB shards can be imported using the shard name:

```sh
$ terraform import mongodb_shard.shard01 shard01
```

Import runs `listShards` to read back the shard state. The shard must already exist in the cluster.

## Known Limitations

* **No update support:** Both `shard_name` and `hosts` are ForceNew. Any change destroys and re-creates the shard registration.
* **Draining can be slow:** The `removeShard` operation drains data from the shard, which can take a long time for large datasets. Adjust `remove_timeout_secs` accordingly.
* **Replica set must be pre-initialized:** The shard's replica set must already be initialized before using this resource. Use [`mongodb_shard_config`](shard_config.md) with `member` blocks to initialize replica sets, or initialize them manually before running `terraform apply`.
* **Single mongos connection:** The provider connects to a single mongos instance. If that mongos is unavailable, all shard operations will fail.
