# mongodb_shard_config

`mongodb_shard_config` manages replica set configuration settings for a MongoDB shard. This resource modifies the replica set settings via `replSetReconfig`.

~> **IMPORTANT:** Delete is a no-op. When this resource is destroyed, Terraform removes it from state but does **not** reset the MongoDB replica set configuration. To restore defaults, manually reconfigure the replica set.

~> **IMPORTANT:** Read currently only retrieves the `shard_name` and resource ID. Changes made to settings outside of Terraform (drift) will not be detected. This is a known limitation.

## Example Usage

### Basic settings

```hcl
resource "mongodb_shard_config" "shard01" {
  shard_name              = "shard01"
  chaining_allowed        = false
  election_timeout_millis = 5000
}
```

### All settings

```hcl
resource "mongodb_shard_config" "shard01" {
  shard_name                = "shard01"
  chaining_allowed          = true
  heartbeat_interval_millis = 2000
  heartbeat_timeout_secs    = 10
  election_timeout_millis   = 10000
}
```

### Mongos auto-discovery (sharded cluster)

When the provider is connected to a mongos router, the resource automatically discovers shard topology via `listShards` and creates temporary direct connections to the appropriate replica set member.

```hcl
provider "mongodb" {
  host     = "mongos.example.com"
  port     = "27017"
  username = "admin"
  password = "secret"
}

resource "mongodb_shard_config" "shard01" {
  shard_name                = "shard01"
  chaining_allowed          = true
  heartbeat_interval_millis = 2000
  heartbeat_timeout_secs    = 10
  election_timeout_millis   = 10000
}

resource "mongodb_shard_config" "shard02" {
  shard_name                = "shard02"
  election_timeout_millis   = 5000
}
```

### Using host_override

When the hostnames returned by `listShards` are internal to the cluster and unreachable from the Terraform runner, use `host_override` to specify an accessible address.

```hcl
resource "mongodb_shard_config" "shard01" {
  shard_name    = "shard01"
  host_override = "shard01-external.example.com:27018"
}
```

## Argument Reference

* `shard_name` - (Required) The name of the replica set (shard) to configure.
* `chaining_allowed` - (Optional) When `true`, allows secondary members to replicate from other secondaries. Default: `true`.
* `heartbeat_interval_millis` - (Optional) Frequency in milliseconds of the heartbeats. Default: `1000`.
* `heartbeat_timeout_secs` - (Optional) Number of seconds that the replica set members wait for a successful heartbeat before marking a member as unreachable. Default: `10`.
* `election_timeout_millis` - (Optional) Time limit in milliseconds for detecting when a primary is unreachable and calling an election. Default: `10000`.
* `init_timeout_secs` - (Optional) Timeout in seconds for replica set initialization (waiting for PRIMARY election and majority health). Default: `60`.
* `host_override` - (Optional) Override the shard host:port discovered via `listShards`. Use when internal hostnames from `listShards` are unreachable from the Terraform runner.

## Replica Set Initialization

When the target replica set has not yet been initialized (MongoDB returns error code 94 — `NotYetInitialized`), the resource automatically handles initialization:

1. Connects in direct mode to the first `member` block's host (with auth fallback for fresh instances).
2. Runs `replSetInitiate` with a single-member config.
3. Waits for the member to reach PRIMARY state.
4. If additional `member` blocks exist, runs `replSetReconfig` to add them with all configured fields.
5. Waits for a majority of members to be healthy (PRIMARY or SECONDARY).

If `replSetInitiate` returns code 23 (`AlreadyInitialized`), the resource falls through to the standard reconfiguration flow.

### Initialization example

```hcl
resource "mongodb_shard_config" "shard01" {
  shard_name              = "shard01"
  chaining_allowed        = true
  election_timeout_millis = 10000
  init_timeout_secs       = 120

  member {
    host     = "mongo1:27017"
    priority = 2
    votes    = 1
  }

  member {
    host     = "mongo2:27017"
    priority = 1
    votes    = 1
  }

  member {
    host     = "mongo3:27017"
    priority = 1
    votes    = 1
  }
}
```

## Mongos Auto-Discovery

When the provider connects to a **mongos** router instead of a direct replica set member, the resource automatically:

1. Runs `isMaster` to detect the connection type (`msg: "isdbgrid"` indicates mongos).
2. Runs `listShards` to discover all shard replica sets.
3. Matches `shard_name` against the shard `_id` in the response.
4. Parses the shard's `host` field (format: `rsName/host1:port,host2:port`).
5. Creates a temporary direct connection to the first host, inheriting the provider's credentials, TLS, and proxy settings.
6. Executes `replSetGetConfig`/`replSetReconfig` against the temporary connection.
7. Disconnects the temporary client when done.

If the provider is already connected directly to a replica set member, no discovery is performed and the provider connection is used as-is.

## Import

MongoDB shard configs can be imported using the base64-encoded shard name:

```sh
$ printf '%s' "shard01" | base64
c2hhcmQwMQ==

$ terraform import mongodb_shard_config.shard01 c2hhcmQwMQ==
```

## Known Limitations

* **Delete is a no-op:** Destroying this resource only removes it from Terraform state. The replica set configuration in MongoDB is not reverted.
* **No drift detection on settings:** Read only populates `shard_name` and the resource ID. If settings are changed outside Terraform, `terraform plan` will not show a diff.
* **No force reconfiguration:** The provider does not support the `force` flag for `replSetReconfig`, which is needed when a majority of members are unreachable.
