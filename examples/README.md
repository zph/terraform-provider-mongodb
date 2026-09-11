# Examples

## Provider Configuration

| Example | Description |
|---|---|
| [provider/basic](provider/basic) | Minimal connection to a local MongoDB instance |
| [provider/ssl](provider/ssl) | TLS-enabled connection with a custom CA certificate |
| [provider/env-vars](provider/env-vars) | All connection details via environment variables |
| [provider/proxy](provider/proxy) | Connection through a SOCKS5 proxy |
| [provider/direct](provider/direct) | Direct connection mode (bypass replica set discovery) |
| [provider/replica-set](provider/replica-set) | Named replica set connection with retry writes |

## Resources

Experimental resources (every resource below except `mongodb_db_user`, `mongodb_db_role` and `mongodb_original_user`) are rejected at plan time until they are opted in. Each example does so with `features_enabled` in its provider block; the `TERRAFORM_PROVIDER_MONGODB_ENABLE` environment variable works the same way.

### mongodb_db_user

| Example | Description |
|---|---|
| [resources/db_user/basic](resources/db_user/basic) | Single user with one built-in role |
| [resources/db_user/multiple-roles](resources/db_user/multiple-roles) | User with roles across multiple databases |
| [resources/db_user/custom-role](resources/db_user/custom-role) | User assigned a custom role via depends_on |
| [resources/db_user/import](resources/db_user/import) | Importing an existing user into Terraform state |

### mongodb_db_role

| Example | Description |
|---|---|
| [resources/db_role/basic](resources/db_role/basic) | Simple privilege-based role |
| [resources/db_role/cluster-privilege](resources/db_role/cluster-privilege) | Cluster-level privilege (replSetGetStatus, etc.) |
| [resources/db_role/inherited](resources/db_role/inherited) | Role inheriting from another custom role |
| [resources/db_role/composite](resources/db_role/composite) | Privileges + inherited roles + depends_on chain |

### mongodb_original_user

| Example | Description |
|---|---|
| [resources/original_user/basic](resources/original_user/basic) | Bootstrap the first admin user on a no-auth instance |
| [resources/original_user/tls](resources/original_user/tls) | Bootstrap over TLS with the password from the environment |

### mongodb_shard_config

| Example | Description |
|---|---|
| [modules/shard_config/basic](modules/shard_config/basic) | Minimal shard configuration (existing) |
| [resources/shard_config/all-settings](resources/shard_config/all-settings) | Every setting explicitly set, including members, oplog size and timeouts |
| [resources/shard_config/multi-shard](resources/shard_config/multi-shard) | Multiple shards via provider aliases |
| [resources/shard_config/mongos-discovery](resources/shard_config/mongos-discovery) | Mongos auto-discovery (single provider, multiple shards) |

### mongodb_shard

| Example | Description |
|---|---|
| [resources/shard/basic](resources/shard/basic) | Register a single shard with a mongos router |
| [resources/shard/custom-timeout](resources/shard/custom-timeout) | Extended addShard retry and drain timeouts |
| [resources/shard/multi-shard](resources/shard/multi-shard) | Register multiple shards with varying timeouts |

### mongodb_profiler

| Example | Description |
|---|---|
| [resources/profiler/basic](resources/profiler/basic) | Profile slow operations on one database |
| [resources/profiler/multi-database](resources/profiler/multi-database) | Per-database levels and thresholds via for_each |

### mongodb_server_parameter

| Example | Description |
|---|---|
| [resources/server_parameter/basic](resources/server_parameter/basic) | Integer and boolean parameters with value coercion |
| [resources/server_parameter/ignore-read](resources/server_parameter/ignore-read) | Write-only parameter with ignore_read |

### mongodb_balancer_config

| Example | Description |
|---|---|
| [resources/balancer_config/disabled](resources/balancer_config/disabled) | Stop the cluster balancer |
| [resources/balancer_config/all-settings](resources/balancer_config/all-settings) | Active window, chunk size, throttle and wait_for_delete |

### mongodb_collection_balancing

| Example | Description |
|---|---|
| [resources/collection_balancing/disabled](resources/collection_balancing/disabled) | Disable balancing for one sharded collection |
| [resources/collection_balancing/chunk-size](resources/collection_balancing/chunk-size) | Per-collection chunk size (MongoDB 6.0+) |

### mongodb_feature_compatibility_version

| Example | Description |
|---|---|
| [resources/feature_compatibility_version/basic](resources/feature_compatibility_version/basic) | Pin FCV with prevent_destroy |
| [resources/feature_compatibility_version/upgrade](resources/feature_compatibility_version/upgrade) | Change FCV with danger_mode and command_preview |

### mongodb_shard_zone

| Example | Description |
|---|---|
| [resources/shard_zone/basic](resources/shard_zone/basic) | Assign two shards to two zones |
| [resources/shard_zone/multi-zone](resources/shard_zone/multi-zone) | Shards in several zones via for_each |

### mongodb_zone_key_range

| Example | Description |
|---|---|
| [resources/zone_key_range/basic](resources/zone_key_range/basic) | Route one region per zone on a compound shard key |
| [resources/zone_key_range/full-key-space](resources/zone_key_range/full-key-space) | Split a hashed key space with $minKey / $maxKey |

## Patterns

Compositions combining multiple resources for real-world scenarios.

| Example | Description |
|---|---|
| [patterns/full-cluster-setup](patterns/full-cluster-setup) | End to end from empty hosts: bootstrap users, replica sets, shards, balancer, zones, roles, users |
| [patterns/sharded-cluster](patterns/sharded-cluster) | Full sharded cluster: mongos + 2 shards, roles, users |
| [patterns/add-replicaset-to-cluster](patterns/add-replicaset-to-cluster) | Day-2 scale-out: bootstrap a new replica set and register it as a shard |
| [patterns/role-hierarchy](patterns/role-hierarchy) | Layered role hierarchy: viewer -> editor -> admin |
| [patterns/monitoring-user](patterns/monitoring-user) | Least-privilege Prometheus/Datadog exporter setup |

## Modules (existing)

Reusable module examples in [modules/](modules/). These use `variable` blocks for parameterization.
