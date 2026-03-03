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

### mongodb_shard_config

| Example | Description |
|---|---|
| [modules/shard_config/basic](modules/shard_config/basic) | Minimal shard configuration (existing) |
| [resources/shard_config/all-settings](resources/shard_config/all-settings) | All configurable settings explicitly set |
| [resources/shard_config/multi-shard](resources/shard_config/multi-shard) | Multiple shards via provider aliases |
| [resources/shard_config/mongos-discovery](resources/shard_config/mongos-discovery) | Mongos auto-discovery (single provider, multiple shards) |

### mongodb_shard

| Example | Description |
|---|---|
| (no examples yet) | Register/remove shards via `addShard`/`removeShard` on a mongos router |

## Patterns

Compositions combining multiple resources for real-world scenarios.

| Example | Description |
|---|---|
| [patterns/sharded-cluster](patterns/sharded-cluster) | Full sharded cluster: mongos + 2 shards, roles, users |
| [patterns/role-hierarchy](patterns/role-hierarchy) | Layered role hierarchy: viewer -> editor -> admin |
| [patterns/monitoring-user](patterns/monitoring-user) | Least-privilege Prometheus/Datadog exporter setup |

## Modules (existing)

Reusable module examples in [modules/](modules/). These use `variable` blocks for parameterization.
