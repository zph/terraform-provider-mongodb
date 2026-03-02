# Implementation Status

## Overview

Terraform provider for MongoDB that manages database users, roles, and replica set shard configuration.

## Architecture

```
main.go                          # Entry point
mongodb/
  provider.go                    # Provider schema (11 attrs, 3 resources)
  config.go                      # ClientConfig, MongoClient, user/role CRUD
  helpers.go                     # validateDiagFunc wrapper
  resource_db_user.go            # mongodb_db_user resource
  resource_db_role.go            # mongodb_db_role resource
  resource_shard_config.go       # mongodb_shard_config resource
  replica_set_types.go           # MongoDB replica set types + GetReplSetConfig/SetReplSetConfig
```

## Resources

| Resource | Status | Operations |
|---|---|---|
| `mongodb_db_user` | Complete | CRUD + import |
| `mongodb_db_role` | Complete | CRUD + import |
| `mongodb_shard_config` | Complete | Create/Read/Update (Delete is no-op) |

## Test Coverage

### Unit Tests (40 tests)

All pure Go tests, no MongoDB required. Run with `make test-unit`.

| File | Count | Covers |
|---|---|---|
| `config_test.go` | 14 | URI builder, proxy dialer, type strings, JSON round-trips, TLS |
| `replica_set_types_test.go` | 10 | GetSelf, GetMembersByState, Primary, constants, BSON round-trips |
| `helpers_test.go` | 4 | validateDiagFunc warning/error propagation |
| `provider_test.go` | 3 | Schema validation, resource map |
| `resource_db_user_test.go` | 4 | ID parsing |
| `resource_db_role_test.go` | 2 | ID parsing |
| `resource_shard_config_test.go` | 2 | ID parsing |

Spec: `docs/specs/unit-test-requirements.md` (TEST-001 through TEST-040)

### Integration Tests (16 tests)

Testcontainer-based tests against a live MongoDB replica set. Run with `make test-integration`. Requires Docker.

| Test | Covers |
|---|---|
| INTEG-001 | MongoClientInit connects successfully |
| INTEG-002 | MongoClientInit rejects bad credentials |
| INTEG-003 | createUser + getUser round-trip |
| INTEG-004 | createUser with no roles |
| INTEG-005 | getUser for non-existent user |
| INTEG-006 | createRole + getRole round-trip |
| INTEG-007 | createRole rejects Db+Cluster conflict |
| INTEG-008 | getRole for non-existent role |
| INTEG-009 | GetReplSetConfig returns valid config |
| INTEG-010 | SetReplSetConfig persists changes |
| INTEG-011 | GetReplSetStatus returns valid status |
| INTEG-012 | GetReplSetStatus.GetSelf returns self member |
| INTEG-013 | ReplSetStatus.Primary matches GetSelf |
| INTEG-014 | GetMembersByState(SECONDARY) empty on single-node |
| INTEG-015 | createRole with Cluster=true privilege |
| INTEG-016 | SetReplSetConfig multi-setting update |

Spec: `docs/specs/integration-test-requirements.md` (INTEG-001 through INTEG-016)

## Make Targets

| Target | Description |
|---|---|
| `help` | Show available targets |
| `build` | Build provider binary |
| `install` | Build + install to Terraform plugins dir |
| `test` | Run unit + plan tests |
| `test-unit` | Run Go unit tests |
| `test-integration` | Run integration tests (requires Docker) |
| `test-plan` | Build + terraform plan against examples |
| `test-shard-plan` | Build + terraform plan for shard_config |
| `lint` | Run golangci-lint |
| `run` | Alias for install |

## Examples

Exhaustive standalone examples organized by capability. See [examples/README.md](examples/README.md) for full index.

### Provider Configuration (6 examples)

| Example | Attributes Covered |
|---|---|
| `provider/basic` | host, port, username, password, auth_database |
| `provider/ssl` | ssl, insecure_skip_verify, certificate |
| `provider/env-vars` | MONGO_HOST, MONGO_PORT, MONGO_USR, MONGO_PWD, MONGODB_CERT, ALL_PROXY |
| `provider/proxy` | proxy |
| `provider/direct` | direct |
| `provider/replica-set` | replica_set, retrywrites |

### Resource Examples (10 examples)

| Example | Attributes Covered |
|---|---|
| `resources/db_user/basic` | auth_database, name, password, role (single) |
| `resources/db_user/multiple-roles` | role (multiple, cross-database) |
| `resources/db_user/custom-role` | role referencing mongodb_db_role, depends_on |
| `resources/db_user/import` | Import workflow (base64 ID) |
| `resources/db_role/basic` | name, database, privilege (db/collection/actions) |
| `resources/db_role/cluster-privilege` | privilege with cluster=true |
| `resources/db_role/inherited` | inherited_role |
| `resources/db_role/composite` | privilege + inherited_role + depends_on chain |
| `resources/shard_config/all-settings` | All 5 shard_config attributes |
| `resources/shard_config/multi-shard` | Provider aliases for multi-shard |

### Pattern Examples (3 examples)

| Example | Demonstrates |
|---|---|
| `patterns/sharded-cluster` | mongos + 2 shards + roles + users + TLS |
| `patterns/role-hierarchy` | 3-layer role inheritance: viewer -> editor -> admin |
| `patterns/monitoring-user` | Least-privilege exporter role + user |

### Attribute Coverage

Every provider attribute and resource attribute appears in at least one example:

- **Provider:** host, port, username, password, auth_database, ssl, certificate, insecure_skip_verify, replica_set, retrywrites, direct, proxy
- **mongodb_db_user:** auth_database, name, password, role.role, role.db
- **mongodb_db_role:** name, database, privilege.db, privilege.collection, privilege.cluster, privilege.actions, inherited_role.role, inherited_role.db
- **mongodb_shard_config:** shard_name, chaining_allowed, heartbeat_interval_millis, heartbeat_timeout_secs, election_timeout_millis

### Cluster Configuration Audit Findings

| # | Severity | Location | Issue |
|---|---|---|---|
| 1 | HIGH | `resource_shard_config.go:86-100` | Read only sets shard_name/ID, never reads back settings. No drift detection. |
| 2 | HIGH | `resource_shard_config.go:102-123` | Delete is a no-op (returns nil). Documented in shard_config.md. |
| 3 | MED | `replica_set_types.go` | CatchUpTimeoutMillis in Settings type but not in resource schema |
| 4 | MED | `resource_shard_config.go` | No client Disconnect() after getClient() - potential connection leak |
| 5 | MED | `resource_db_user.go:142`, `resource_db_role.go:179,202` | Wrong error variable in error messages (3 instances) |
| 6 | LOW | `config.go:128` | MaxConnLifetime hardcoded to 10s |
| 7 | LOW | `replica_set_types.go` | No force flag support for replSetReconfig |

## Dependencies

- `terraform-plugin-sdk/v2` v2.34.0 - Terraform provider framework
- `go.mongodb.org/mongo-driver` v1.15.0 - MongoDB Go driver
- `testcontainers-go` v0.40.0 - Integration test containers (test only)
- `testcontainers-go/modules/mongodb` v0.40.0 - MongoDB container module (test only)
