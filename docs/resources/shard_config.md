# mongodb_shard_config

~> **EXPERIMENTAL:** This resource requires opt-in via `TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_shard_config`. The API may change in future releases.

`mongodb_shard_config` manages replica set configuration settings for a MongoDB shard. This resource modifies the replica set settings via `replSetReconfig`.

~> **IMPORTANT:** Delete is a no-op. When this resource is destroyed, Terraform removes it from state but does **not** reset the MongoDB replica set configuration. To restore defaults, manually reconfigure the replica set.

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

### With oplog size

```hcl
resource "mongodb_shard_config" "shard01" {
  shard_name              = "shard01"
  chaining_allowed        = true
  election_timeout_millis = 10000
  oplog_size_mb           = 2048
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

~> **NOTE:** `oplog_size_mb` conflicts with `host_override`. This is enforced at validation time, so a configuration that sets both fails `terraform plan` after upgrading — remove one of the two (keep `host_override` and manage the oplog size outside Terraform, or drop `host_override` if the member hostnames are reachable). The per-member `replSetResizeOplog` and read-back connections use the member hostnames from the replica set configuration, which must be reachable from the Terraform runner — and, when TLS is enabled, the server certificates must be valid for those member hostnames. The resize applies to **every** data-bearing member, including members not listed in `member` blocks. Members that are not PRIMARY or SECONDARY are skipped during the resize, and unreachable members are skipped during the read-back (with a warning), so they surface as drift on a later plan instead of failing the run. If *every* member is skipped, the apply fails rather than reporting success for a resize that changed nothing; and if a resize ran but no member could be read back, `-1` is stored so the next plan re-applies it.

## Argument Reference

* `shard_name` - (Required) The name of the replica set (shard) to configure.
* `chaining_allowed` - (Optional) When `true`, allows secondary members to replicate from other secondaries. Default: `true`.
* `heartbeat_interval_millis` - (Optional) Frequency in milliseconds of the heartbeats. Default: `1000`.
* `heartbeat_timeout_secs` - (Optional) Number of seconds that the replica set members wait for a successful heartbeat before marking a member as unreachable. Default: `10`.
* `election_timeout_millis` - (Optional) Time limit in milliseconds for detecting when a primary is unreachable and calling an election. Default: `10000`.
* `catch_up_timeout_millis` - (Optional) Time in milliseconds that a newly elected primary waits for secondaries to catch up before accepting writes. `-1` means infinite (MongoDB default). Default: `-1`.
* `oplog_size_mb` - (Optional) Maximum oplog size in megabytes. The oplog is node-local storage, so the size is applied via `replSetResizeOplog` on **every data-bearing member** of the replica set over a direct connection to each member (secondaries first, primary last; arbiters are skipped). When reading state back, the common size across members is reported; if members disagree (for example after a partially failed resize), `-1` is stored so the divergence shows up as drift in the next plan. Requires the Terraform runner to be able to reach every member host listed in the replica set configuration. Conflicts with `host_override`. When not set, oplog sizes are left at their current values (MongoDB default).
* `init_timeout_secs` - (Optional) Timeout in seconds for replica set initialization (waiting for PRIMARY election) and, separately, for each added member to become reachable (see [Adding members](#adding-members)). Default: `60`.
* `host_override` - (Optional) Override the shard host:port discovered via `listShards`. Use when internal hostnames from `listShards` are unreachable from the Terraform runner.

### Member

Each `member` block configures an individual replica set member. Blocks are matched to live members by `host`. A block whose host is already a member updates that member in place. A block whose host is not in the set adds a new member (see [Adding members](#adding-members)). Members that are in the set but have no block are left untouched, and the resource never removes a member.

`priority` and `votes` are always sent, including `priority = 0`, so the defaults below are applied by the resource rather than by the server and hidden members (which require priority 0) can be configured.

* `host` - (Required) `host:port` address of the replica set member.
* `arbiter_only` - (Optional) Whether the member is an arbiter. Default: `false`.
* `build_indexes` - (Optional) Whether the member builds indexes. Default: `true`.
* `hidden` - (Optional) Whether the member is hidden from client connections. Default: `false`.
* `priority` - (Optional) Election priority. `0` means the member can never become primary. Default: `1`.
* `tags` - (Optional) Map of string key-value pairs for replica set tags.
* `votes` - (Optional) Number of votes the member has in elections (`0` or `1`). Default: `1`.

## Replica Set Initialization

When the target replica set has not yet been initialized (MongoDB returns error code 94 — `NotYetInitialized`), the resource automatically handles initialization:

1. Connects in direct mode to the first `member` block's host (with auth fallback for fresh instances).
2. Runs `replSetInitiate` with that host as the only member (`_id: 0`).
3. Waits for the member to reach PRIMARY state.
4. Continues exactly as for an already-initialized set: one `replSetReconfig` applies the settings and the first member's fields, then each remaining `member` block is added as described under [Adding members](#adding-members).

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

## Adding members

A `member` block whose `host` is not in the replica set is added on apply. This works the same for a set the provider initialized and for one initialized elsewhere, for example by a bootstrap script on the first host that runs `replSetInitiate` with a single member and creates the first user through the localhost exception.

Members are added one at a time, each with its own `replSetReconfig`, after the reconfig that applies the settings and the matched members. MongoDB 4.4 rejects a reconfig that adds more than one voting member; one member per reconfig is correct on every version. Before each add the configuration is re-read from the server, and the new member gets an `_id` one higher than the highest in use, never its position in the list.

After each add the resource waits, for up to `init_timeout_secs`, until the primary reports the new member as reachable (`health: 1` in any state, including `STARTUP2`). It does not wait for initial sync, which can take hours on a set with data. If the member does not become reachable in time, the resource removes it again and fails the apply, so a mistyped host does not stay in the configuration as a permanently unreachable member. Fix the host and apply again.

State is read back from the server after the last reconfig. A failed add leaves the settings and any members added before it in place; the next apply adds only what is still missing.

### Growing a one-member set

```hcl
resource "mongodb_shard_config" "shard01" {
  shard_name        = "shard01"
  init_timeout_secs = 120

  # Already the only member: merged in place.
  member {
    host     = "mongo1:27017"
    priority = 2
  }

  # Added first.
  member {
    host = "mongo2:27017"
  }

  # Added second.
  member {
    host     = "mongo3:27017"
    priority = 0
    votes    = 0
    hidden   = true
    tags = {
      nodeType = "analytics"
    }
  }
}
```

With `command_preview = true`, the plan lists one `replSetReconfig` line per host that will be added.

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

MongoDB shard configs can be imported using the shard name directly:

```sh
$ terraform import mongodb_shard_config.shard01 shard01
```

## Known Limitations

* **Delete is a no-op:** Destroying this resource only removes it from Terraform state. The replica set configuration in MongoDB is not reverted.
* **No force reconfiguration:** The provider does not support the `force` flag for `replSetReconfig`, which is needed when a majority of members are unreachable.
* **No member removal:** A live member without a `member` block stays in the set. Removing a member takes `rs.remove()` or a manual `replSetReconfig`. The only removal the resource performs is undoing an add whose member never became reachable.
