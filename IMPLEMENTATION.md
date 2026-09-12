# Implementation Status

## Overview

Terraform provider for MongoDB that manages database users, roles, and replica set shard configuration.

## Architecture

```
main.go                          # Entry point
mongodb/
  provider.go                    # Provider schema (11 attrs, 4 resources)
  config.go                      # ClientConfig, MongoClient/MongoClientNoAuth, user/role CRUD
  helpers.go                     # validateDiagFunc wrapper
  resource_db_user.go            # mongodb_db_user resource
  resource_db_role.go            # mongodb_db_role resource
  resource_shard_config.go       # mongodb_shard_config resource
  resource_original_user.go      # mongodb_original_user resource (bootstrap, no-auth)
  replica_set_types.go           # MongoDB replica set types + GetReplSetConfig/SetReplSetConfig
  shard_discovery.go             # Shard auto-discovery via mongos (DetectConnectionType, ListShards, etc.)
```

## Resources

| Resource | Status | Operations |
|---|---|---|
| `mongodb_db_user` | Complete | CRUD + import |
| `mongodb_db_role` | Complete | CRUD + import |
| `mongodb_shard_config` | Complete | Create/Read/Update (Delete is no-op). Member-level config via `member` block; members not yet in the set are added one reconfig at a time, staged as non-voters and promoted once SECONDARY. Mongos auto-discovery via `listShards`. Create waits, up to `timeouts.create`, for hosts that are still starting and for the provider user to be created. |
| `mongodb_original_user` | Complete | CRUD (bootstrap no-auth, idempotent adopt) |

## Test Coverage

### Unit Tests (92 tests)

All pure Go tests, no MongoDB required. Run with `make test-unit`.

| File | Count | Covers |
|---|---|---|
| `config_test.go` | 20 | URI builder, proxy dialer, type strings, JSON round-trips, TLS, MongoClientNoAuth, no-auth via empty credentials |
| `replica_set_types_test.go` | 10 | GetSelf, GetMembersByState, Primary, constants, BSON round-trips |
| `helpers_test.go` | 4 | validateDiagFunc warning/error propagation |
| `provider_test.go` | 3 | Schema validation, resource map |
| `resource_db_user_test.go` | 4 | ID parsing |
| `resource_db_role_test.go` | 2 | ID parsing |
| `resource_shard_config_test.go` | 36 | ID parsing, MergeMembers, RSConfigMembersToState (block order), schema validation, member defaults, arbiter priority, duplicate hosts, oplog fan-out helpers |
| `shard_members_test.go` | 25 | PartitionMemberOverrides, HoldPromotions, NextMemberID, BuildConfigMember, AddMembersSequentially staging/promotion/rollback/pending, PromoteMembersSequentially, ReconcileMembers, CheckAddTarget, PreflightAddTargets, observeMember, WaitForMemberState |
| `shard_ready_test.go` | 16 | IsConnectionError (incl. a real refused dial), classifyAuthProbe, WaitForShardClient retry/probe/deadline paths, WaitForAddTargets, timeouts schema |
| `shard_discovery_test.go` | 22 | ParseShardHost, FindShardByName, SplitHostPort, BuildShardClientConfig, DetectConnectionType, ConnectionType.String(), host_override schema |
| `resource_original_user_test.go` | 11 | Schema validation, ID parsing, sensitive fields |

Spec: `docs/specs/unit-test-requirements.md` (TEST-001 through TEST-056), SHARD-011, `docs/specs/shard-discovery-requirements.md` (DISC-001 through DISC-010)

### Integration Tests (21 tests)

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
| INTEG-017 | MergeMembers priority update round-trip |
| INTEG-018 | MergeMembers tags update round-trip |
| INTEG-019 | MergeMembers votes update round-trip |
| INTEG-020 | MergeMembers host-not-found error |
| INTEG-021 | RSConfigMembersToState read-back round-trip |
| INTEG-022 | updateWithClient grows a one-member set to three, staging the voter and promoting it once SECONDARY |
| INTEG-023 | Second apply against the grown set adds nothing |
| INTEG-024 | Raising a live member's votes waits for SECONDARY and reconfigs once |
| INTEG-025 | An add whose member never answers is rolled back and state lists only the live members |
| INTEG-026 | A block naming a live member under another name is refused by the pre-flight before any reconfig |
| INTEG-027 | An arbiter whose host cannot be inspected is refused before any reconfig |
| INTEG-028 | A new member host that never answers is waited for until the deadline, then refused before any reconfig |
| INTEG-029 | A refused login and the unauthenticated probe classify correctly with and without access control |
| INTEG-030 | Create waits while the seed's bootstrap creates the provider user, then applies the settings |

Spec: `docs/specs/integration-test-requirements.md` (INTEG-001 through INTEG-030), `docs/specs/shard-member-requirements.md` (SHARD-001 through SHARD-029), `docs/specs/shard-init-requirements.md` (INIT-001 through INIT-036)

## Make Targets

| Target | Description |
|---|---|
| `help` | Show available targets |
| `setup` | Set up dev environment (hermit, git hooks, go deps) |
| `build` | Build provider binary |
| `install` | Build + install to Terraform plugins dir |
| `test` | Run unit + plan tests |
| `test-unit` | Run Go unit tests |
| `test-integration` | Run integration tests (requires Docker) |
| `test-plan` | Build + terraform plan against examples |
| `test-shard-plan` | Build + terraform plan for shard_config |
| `test-examples-plan` | Build + terraform plan every example directory, offline |
| `lint` | Run all prek hooks on all files |
| `prek` | Alias for lint |
| `prek-install` | Install prek as git pre-commit hook |
| `run` | Alias for install |

## Verbose Command Logging

All MongoDB commands are logged via the Go driver's `event.CommandMonitor` attached in `config.go:commandMonitor()`. Logs use `tflog` and are controlled by Terraform's standard `TF_LOG` environment variable.

**Enable:** `TF_LOG=DEBUG terraform apply`

**What's logged:**
- **Started** (DEBUG): command name, database, request ID, full BSON command body
- **Succeeded** (DEBUG): command name, duration, request ID
- **Failed** (WARN): command name, duration, failure message, request ID

Authentication commands (`saslStart`, `saslContinue`) have their command bodies automatically redacted by the MongoDB driver.

## No-Auth Support

Provider `username` and `password` are optional (default `""`). When `username` is empty, `MongoClient` skips `SetAuth` entirely, allowing all resources (including `mongodb_shard_config`) to work against clusters with no authentication enabled.

EARS spec: SHARD-011 in `docs/specs/shard-member-requirements.md`.

EARS specs: LOG-001 through LOG-004 in `config.go`.

## Shard Auto-Discovery

When the provider connects to a **mongos** router, `mongodb_shard_config` automatically discovers the target shard's replica set topology and creates a temporary direct connection for `replSetGetConfig`/`replSetReconfig`. This eliminates the need for provider aliases per shard.

**Flow:**

1. `getShardClient` creates a provider client via `MongoClientInit`.
2. `DetectConnectionType` runs `isMaster` — if `msg == "isdbgrid"`, it's mongos.
3. `ListShards` runs the `listShards` admin command.
4. `FindShardByName` matches `shard_name` against shard `_id` values.
5. `ParseShardHost` extracts `rsName` and host list from `"rsName/host1:port,host2:port"`.
6. `BuildShardClientConfig` clones provider creds/TLS/proxy into a direct-mode config.
7. `MongoClientInit` creates the temporary shard client.
8. CRUD runs against the shard client; cleanup disconnects both clients.

If the provider connects directly to a replica set member (`setName` present in isMaster), the provider client is used as-is with no discovery.

**`host_override` escape hatch:** When `listShards` returns internal hostnames unreachable from the Terraform runner, set `host_override` on the resource to specify an accessible `host:port`.

EARS spec: DISC-001 through DISC-010 in `docs/specs/shard-discovery-requirements.md`.

Implementation: `mongodb/shard_discovery.go`.

## Examples

Standalone examples organized by capability. See [examples/README.md](examples/README.md) for the full index with descriptions. The attribute lists below are derived from the provider schema (`terraform providers schema -json`) matched against uncommented lines in each example; regenerate them when examples or schemas change.

### Provider Configuration (6 examples)

| Example | Attributes Covered |
|---|---|
| `provider/basic` | auth_database, host, password, port, username |
| `provider/direct` | auth_database, direct, host, password, port, username |
| `provider/env-vars` | auth_database |
| `provider/proxy` | auth_database, host, password, port, proxy, username |
| `provider/replica-set` | auth_database, host, password, port, replica_set, retrywrites, username |
| `provider/ssl` | auth_database, certificate, host, insecure_skip_verify, password, port, ssl, username |

### Resource Examples (30 examples)

| Example | Resources | Top-level Attributes Covered |
|---|---|---|
| `resources/balancer_config/all-settings` | mongodb_balancer_config | active_window_start, active_window_stop, chunk_size_mb, enabled, secondary_throttle, wait_for_delete |
| `resources/balancer_config/disabled` | mongodb_balancer_config | enabled |
| `resources/collection_balancing/chunk-size` | mongodb_collection_balancing | chunk_size_mb, enabled, namespace |
| `resources/collection_balancing/disabled` | mongodb_collection_balancing | enabled, namespace |
| `resources/db_role/basic` | mongodb_db_role | database, name, privilege |
| `resources/db_role/cluster-privilege` | mongodb_db_role | database, name, privilege |
| `resources/db_role/composite` | mongodb_db_role | database, inherited_role, name, privilege |
| `resources/db_role/inherited` | mongodb_db_role | database, inherited_role, name, privilege |
| `resources/db_user/basic` | mongodb_db_user | auth_database, name, password, role |
| `resources/db_user/custom-role` | mongodb_db_role, mongodb_db_user | auth_database, database, name, password, privilege, role |
| `resources/db_user/import` | mongodb_db_user | auth_database, name, password, role |
| `resources/db_user/multiple-roles` | mongodb_db_user | auth_database, name, password, role |
| `resources/feature_compatibility_version/basic` | mongodb_feature_compatibility_version | version |
| `resources/feature_compatibility_version/upgrade` | mongodb_feature_compatibility_version | danger_mode, version |
| `resources/original_user/basic` | mongodb_original_user | host, password, port, role, username |
| `resources/original_user/tls` | mongodb_original_user | auth_database, certificate, host, insecure_skip_verify, port, replica_set, role, ssl, username |
| `resources/profiler/basic` | mongodb_profiler | database, level, slowms |
| `resources/profiler/multi-database` | mongodb_profiler | database, level, slowms |
| `resources/server_parameter/basic` | mongodb_server_parameter | parameter, value |
| `resources/server_parameter/ignore-read` | mongodb_server_parameter | ignore_read, parameter, value |
| `resources/shard/basic` | mongodb_shard | hosts, shard_name |
| `resources/shard/custom-timeout` | mongodb_shard | add_timeout_secs, hosts, remove_timeout_secs, shard_name |
| `resources/shard/multi-shard` | mongodb_shard | hosts, remove_timeout_secs, shard_name |
| `resources/shard_config/all-settings` | mongodb_shard_config | catch_up_timeout_millis, chaining_allowed, election_timeout_millis, heartbeat_interval_millis, heartbeat_timeout_secs, init_timeout_secs, member, oplog_size_mb, shard_name, timeouts |
| `resources/shard_config/mongos-discovery` | mongodb_db_user, mongodb_shard_config | auth_database, chaining_allowed, election_timeout_millis, heartbeat_interval_millis, heartbeat_timeout_secs, name, password, role, shard_name |
| `resources/shard_config/multi-shard` | mongodb_shard_config | chaining_allowed, election_timeout_millis, heartbeat_interval_millis, heartbeat_timeout_secs, shard_name |
| `resources/shard_zone/basic` | mongodb_shard_zone | shard_name, zone |
| `resources/shard_zone/multi-zone` | mongodb_shard_zone | shard_name, zone |
| `resources/zone_key_range/basic` | mongodb_shard_zone, mongodb_zone_key_range | max, min, namespace, shard_name, zone |
| `resources/zone_key_range/full-key-space` | mongodb_shard_zone, mongodb_zone_key_range | max, min, namespace, shard_name, zone |

### Pattern Examples (5 examples)

| Example | Resources Used |
|---|---|
| `patterns/add-replicaset-to-cluster` | mongodb_original_user (1), mongodb_shard (1), mongodb_shard_config (1), mongodb_shard_zone (1), mongodb_zone_key_range (1) |
| `patterns/full-cluster-setup` | mongodb_balancer_config (1), mongodb_db_role (1), mongodb_db_user (1), mongodb_original_user (4), mongodb_shard (2), mongodb_shard_config (2), mongodb_shard_zone (2), mongodb_zone_key_range (2) |
| `patterns/monitoring-user` | mongodb_db_role (1), mongodb_db_user (1) |
| `patterns/role-hierarchy` | mongodb_db_role (3), mongodb_db_user (3) |
| `patterns/sharded-cluster` | mongodb_db_role (2), mongodb_db_user (2), mongodb_shard_config (2) |

### Attribute Coverage

- **Provider:** auth_database, certificate, command_preview, direct, features_enabled, host, insecure_skip_verify, password, port, proxy, replica_set, retrywrites, ssl, username
- **mongodb_balancer_config:** active_window_start, active_window_stop, chunk_size_mb, enabled, secondary_throttle, wait_for_delete
- **mongodb_collection_balancing:** chunk_size_mb, enabled, namespace
- **mongodb_db_role:** database, name, inherited_role, inherited_role.db, inherited_role.role, privilege, privilege.actions, privilege.cluster, privilege.collection, privilege.db
- **mongodb_db_user:** auth_database, name, password, role, role.db, role.role
- **mongodb_feature_compatibility_version:** danger_mode, version
- **mongodb_original_user:** auth_database, certificate, host, insecure_skip_verify, password, port, replica_set, ssl, username, role, role.db, role.role
- **mongodb_profiler:** database, level, slowms — not shown: `ratelimit` (honoured by Percona Server only; described in a comment in `profiler/multi-database`)
- **mongodb_server_parameter:** ignore_read, parameter, value
- **mongodb_shard:** add_timeout_secs, hosts, remove_timeout_secs, shard_name
- **mongodb_shard_config:** catch_up_timeout_millis, chaining_allowed, election_timeout_millis, heartbeat_interval_millis, heartbeat_timeout_secs, init_timeout_secs, oplog_size_mb, shard_name, member, member.arbiter_only, member.build_indexes, member.hidden, member.host, member.priority, member.tags, member.votes, timeouts, timeouts.create, timeouts.update — not shown: `host_override` (conflicts with `oplog_size_mb`; shown commented out in `shard_config/mongos-discovery`)
- **mongodb_shard_zone:** shard_name, zone
- **mongodb_zone_key_range:** max, min, namespace, zone

### Cluster Configuration Audit Findings

| # | Severity | Location | Issue |
|---|---|---|---|
| 1 | RESOLVED | `resource_shard_config.go` | Read now reads back settings and member config for drift detection. |
| 2 | HIGH | `resource_shard_config.go:102-123` | Delete is a no-op (returns nil). Documented in shard_config.md. |
| 3 | RESOLVED | `resource_shard_config.go` | `catch_up_timeout_millis` is in the resource schema (default -1). |
| 4 | RESOLVED | `resource_shard_config.go` | getShardClient now returns cleanup function; all CRUD methods defer cleanup(). |
| 5 | MED | `resource_db_user.go:142`, `resource_db_role.go:179,202` | Wrong error variable in error messages (3 instances) |
| 6 | LOW | `config.go:128` | MaxConnLifetime hardcoded to 10s |
| 7 | LOW | `replica_set_types.go` | No force flag support for replSetReconfig |

## CDKTN Construct Library

Go construct library for generating Terraform JSON targeting `terraform-provider-mongodb`. Lives in `cdktn/` sub-module.

### Resource Coverage

| Provider Resource | CDKTN Support | Notes |
|---|---|---|
| `mongodb_db_user` | Complete | Per-member + cluster-level propagation |
| `mongodb_db_role` | Complete | Per-member + cluster-level merging |
| `mongodb_shard_config` | Complete | Per-RS config + per-member overrides via `member` blocks (CDKTN-051) |
| `mongodb_original_user` | Complete | Inline connection params, L2 + cluster-level cascade (CDKTN-052) |

Spec: `docs/specs/cdktn-sharded-cluster-generator-requirements.md` (CDKTN-001 through CDKTN-052)

## Dependencies

- `terraform-plugin-sdk/v2` v2.34.0 - Terraform provider framework
- `go.mongodb.org/mongo-driver` v1.15.0 - MongoDB Go driver
- `testcontainers-go` v0.40.0 - Integration test containers (test only)
- `testcontainers-go/modules/mongodb` v0.40.0 - MongoDB container module (test only)
