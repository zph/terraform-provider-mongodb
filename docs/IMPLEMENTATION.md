# terraform-provider-mongodb — Implementation

**Last Updated:** 2026-03-03

---

## Provider Resources

| Resource | File | Maturity | Description |
|----------|------|----------|-------------|
| `mongodb_db_user` | `mongodb/resource_db_user.go` | mature | Manage MongoDB database users |
| `mongodb_db_role` | `mongodb/resource_db_role.go` | mature | Manage MongoDB database roles |
| `mongodb_original_user` | `mongodb/resource_original_user.go` | mature | Bootstrap the initial admin user |
| `mongodb_shard_config` | `mongodb/resource_shard_config.go` | experimental | Configure replica set settings and initialize uninitialized RS |
| `mongodb_shard` | `mongodb/resource_shard.go` | experimental | Add/remove shards from a mongos router |

### Resource Capability Gating

Resources are classified as `mature` (always registered) or `experimental` (blocked by default). Experimental resources require opt-in via:

```bash
export TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_shard_config,mongodb_shard
```

The registry is defined in `mongodb/resource_registry.go`. See `docs/specs/resource-gating-requirements.md` for the EARS spec (GATE-001 through GATE-010).

## Provider Key Files

| File | Purpose |
|------|---------|
| `mongodb/provider.go` | Provider schema, resource registration, configuration |
| `mongodb/resource_registry.go` | Resource maturity classification, allowlist gating, env var parsing |
| `mongodb/parse_id.go` | Shared `parseResourceId` / `formatResourceId` helpers (IDFORMAT spec) |
| `mongodb/config.go` | Client configuration, connection, TLS, proxy |
| `mongodb/replica_set_types.go` | RS config types, `GetReplSetConfig`, `SetReplSetConfig`, `GetReplSetStatus` |
| `mongodb/shard_discovery.go` | Connection type detection, `ListShards`, `ResolveShardClient` |
| `mongodb/shard_init.go` | RS initialization: `IsNotYetInitialized`, `IsAlreadyInitialized`, `BuildInitialMembers`, `InitiateReplicaSet`, `WaitForPrimary`, `WaitForMajorityHealthy`, `ConnectForInit` |
| `mongodb/resource_shard.go` | `mongodb_shard` resource: `addShard`, `removeShard` with polling |
| `mongodb/resource_shard_config.go` | `mongodb_shard_config` resource: RS config + initialization flow |

## EARS Specifications

| Spec | File | ID Range |
|------|------|----------|
| Shard Config | `docs/specs/` (inline in code) | SHARD-001 through SHARD-011 |
| Shard Discovery | `docs/specs/` (inline in code) | DISC-001 through DISC-010 |
| Shard Initialization | `docs/specs/shard-init-requirements.md` | INIT-001 through INIT-024 |
| Shard Cluster Management | `docs/specs/shard-cluster-requirements.md` | CLUS-001 through CLUS-010 |
| Golden File Testing | `docs/specs/golden-test-requirements.md` | GOLDEN-001 through GOLDEN-021 |
| Sharded Integration Tests | `docs/specs/sharded-integration-test-requirements.md` | SINTEG-001 through SINTEG-014 |
| Resource Gating | `docs/specs/resource-gating-requirements.md` | GATE-001 through GATE-010 |
| ID Format | `docs/specs/id-format-requirements.md` | IDFORMAT-001 through IDFORMAT-005 |
| Command Logging | (inline in code) | LOG-001 through LOG-004 |

## Test Files

| File | Tests | Build Tag |
|------|-------|-----------|
| `mongodb/shard_init_test.go` | INIT-T01..T12 (12 tests) | none |
| `mongodb/resource_shard_test.go` | CLUS-T01..T06 (6 tests) | none |
| `mongodb/resource_shard_config_test.go` | SHARD-T01..T13 (15 tests) | none |
| `mongodb/shard_discovery_test.go` | DISC tests | none |
| `mongodb/replica_set_types_test.go` | RS type tests | none |
| `mongodb/config_test.go` | Config tests | none |
| `mongodb/golden_test.go` | Golden file tests | integration |
| `mongodb/resource_registry_test.go` | GATE-T01..T15 (15 tests) | none |
| `mongodb/parse_id_test.go` | IDFORMAT parse/format tests | none |
| `mongodb/command_recorder_test.go` | CommandRecorder tests | none |
| `mongodb/sharded_integration_test.go` | SINTEG sharded cluster tests (10 tests) | integration |

---

# CDKTN Sharded MongoDB Cluster Construct Library — Implementation

**Status:** Core implementation complete. Tests passing. Integration and E2E tests pending.
**Module:** `github.com/zph/terraform-provider-mongodb/cdktn`
**Last Updated:** 2026-03-02

---

## What This Is

The `cdktn/` directory is a Go construct library that replaces a Jinja2 template (`main.tf.j2`) with typed, composable Go structs that generate deterministic Terraform JSON for sharded MongoDB clusters. It uses `terraform-provider-mongodb` (registry.terraform.io/zph/mongodb) as the target provider.

The library is intentionally standalone: it does not depend on the CDKTN framework packages (`github.com/open-constructs/cdk-terrain-go/cdktn`) because those packages are not yet reliably available. Instead, it implements its own lightweight synthesis engine (`TerraformStack`) that mirrors the CDKTN `App → Stack → Synth()` pipeline. When CDKTN packages ship, the custom engine can be replaced with the real framework (see Future Work below).

---

## Architecture

### Sub-Module Structure

`cdktn/` is a separate Go module (`go.mod`) within the repo rather than a package inside the provider module. This is intentional:

- The provider module (`go.mod` at root) has no dependency on CDKTN tooling.
- The construct library can be versioned, released, and consumed independently.
- The two modules cannot import each other's types without a circular dependency. Constants required by both (`MaxMembers`, `MaxVotingMembers`, etc.) are duplicated in `cdktn/constants.go` with a comment explaining why. (CDKTN-026, CDKTN-027)

```
terraform-provider-mongodb/
  go.mod                     # provider module
  mongodb/                   # provider implementation
  cdktn/
    go.mod                   # construct library module (separate)
    constants.go
    types.go
    credentials.go
    validation.go
    stack.go
    provider_factory.go
    resource_builder.go
    shard.go
    config_server.go
    mongos.go
    cluster.go
    *_test.go
    testdata/                # golden JSON files for synthesis tests
      cluster_minimal.json
      cluster_full.json
      shard_basic.json
      config_server_basic.json
      mongos_basic.json
```

### Synthesis Engine

`TerraformStack` accumulates provider aliases and resource blocks, then serializes them to deterministic JSON on `Synth()`. Determinism is achieved by sorting providers and resources by alias/type+name before marshaling. (CDKTN-029)

The output structure matches Terraform's native JSON format:

```json
{
  "terraform": { "required_version": "...", "required_providers": { ... } },
  "provider": { "mongodb": [ { "alias": "...", ... }, ... ] },
  "resource": { "mongodb_db_user": { ... }, "mongodb_shard_config": { ... } }
}
```

---

## File Layout

| File | Purpose |
|------|---------|
| `constants.go` | MongoDB limits (`MaxMembers`, `MaxVotingMembers`), shard config defaults, provider metadata, `ComponentType` iota with `String()` |
| `types.go` | All props structs (`MongoShardProps`, `ConfigServerProps`, `MongosProps`, `MongoShardedClusterProps`) and value types (`MemberConfig`, `UserConfig`, `RoleConfig`, `SSLConfig`, `ShardConfigSettings`) |
| `credentials.go` | `CredentialSource` interface + `DirectCredentials` and `EnvCredentials` implementations |
| `validation.go` | Pure validation functions returning `error`: member count bounds, duplicate host:port, duplicate RS names, mongos/shard presence |
| `stack.go` | `TerraformStack` synthesis engine: `AddProvider`, `AddResource`, `Synth()`, `SynthToMap()` |
| `provider_factory.go` | `ProviderAliasName`, `BuildProviderConfig`, `BuildProviders`, `BuildProvidersWithOffset` |
| `resource_builder.go` | `BuildRoles`, `BuildUsers`, `BuildShardConfig` — emit `mongodb_db_role`, `mongodb_db_user`, `mongodb_shard_config` resources |
| `shard.go` | `MongoShard` L2 construct |
| `config_server.go` | `MongoConfigServer` L2 construct |
| `mongos.go` | `MongoMongos` L2 construct |
| `cluster.go` | `MongoShardedCluster` L3 construct — composes all L2s on a shared stack |

---

## Construct Hierarchy

### L2 Constructs

All three L2 constructs follow the same pattern: validate props, create provider aliases for each member, create role resources, create user resources with `depends_on` role references. Shard and config server additionally create one `mongodb_shard_config` resource per replica set.

**`MongoShard`** (`shard.go`) — CDKTN-001, CDKTN-022, CDKTN-035
- Represents one shard replica set.
- Requires `>= 3` members, `<= 50` members.
- Sets `direct = true` on all provider aliases.
- Creates `mongodb_shard_config` targeting the first member (primary).

**`MongoConfigServer`** (`config_server.go`) — CDKTN-001, CDKTN-021, CDKTN-035
- Represents the config server replica set (CSRS).
- Same validation and synthesis as `MongoShard` with `ComponentType = configsvr`.

**`MongoMongos`** (`mongos.go`) — CDKTN-001, CDKTN-017, CDKTN-036
- Represents one or more mongos query routers.
- No minimum member count (one is valid).
- Sets `direct = false` on all provider aliases.
- Does NOT create `mongodb_shard_config` resources.
- Supports `WithOffset` variant for sequential alias numbering across multiple mongos groups.

### L3 Construct

**`MongoShardedCluster`** (`cluster.go`) — CDKTN-002

Composes all L2 constructs on a single shared `TerraformStack`. Constructor signature:

```go
func NewMongoShardedCluster(id string, props *MongoShardedClusterProps) (*MongoShardedCluster, error)
```

Cluster-level validation before constructing any L2:
1. At least one mongos (CDKTN-023)
2. At least one shard (CDKTN-024)
3. No duplicate RS names across config server and all shards (CDKTN-028)

Then constructs components in this order: config server, shards, mongos groups. Cluster-level credentials, SSL, and proxy cascade to all L2s (CDKTN-009, CDKTN-018, CDKTN-034). Cluster-level roles are merged (union) with component-level roles.

Cluster-level users (CDKTN-037) are placed on:
- All mongos aliases
- First alias of each shard (first member = presumed primary)

---

## Key Design Decisions

### Error Returns, Not Panics

All constructors return `(*Type, error)`. The CDK convention panics on invalid input; idiomatic Go returns errors. Callers can `log.Fatal(err)` if they want panic-equivalent behavior. This makes validation testable without `recover`. (CDKTN-021 through CDKTN-028)

### Plain Go Types, No Framework Dependency

No import of `github.com/open-constructs/cdk-terrain-go/cdktn` or `github.com/aws/constructs-go/constructs`. The `TerraformStack` in `stack.go` is a plain struct — not a CDKTN `TerraformStack` embedding a construct node. This was a deliberate tradeoff to ship working code before framework packages are stable. (CDKTN-048, CDKTN-049, CDKTN-050 are not yet satisfied — see Future Work.)

### Port as String in Provider Config

The `mongodb` provider schema declares `port` as `*string`, not `int`. `BuildProviderConfig` converts `MemberConfig.Port` (int) via `fmt.Sprintf("%d", port)`. The provider then parses it back. This matches the provider's own schema behavior.

### Deterministic Synthesis

`TerraformStack.Synth()` sorts providers by alias and resources by `type.name` before JSON serialization. Go map iteration is non-deterministic; every internal map is either copied-and-sorted before emission or uses a list accumulator. Golden file tests catch any regression. (CDKTN-029)

### Constants Duplicated Across Modules

`cdktn/constants.go` duplicates `MaxMembers`, `MaxVotingMembers`, `MinVotingMembers`, `DefaultPriority`, `DefaultVotes` from `mongodb/replica_set_types.go`. Importing the parent module would create a circular dependency between two Go modules in the same repo and pull provider build dependencies into the construct library. The duplication is explicit and commented. (CDKTN-026, CDKTN-027)

### Provider Alias Naming

Pattern: `<component_type>_<replica_set_name>_<member_index>` for shards and config servers; `mongos_<member_index>` for mongos (no RS name). Multiple mongos groups share a global sequential index maintained by `NewMongoMongosWithOffset`. (CDKTN-004)

### Shard Config Defaults

`DefaultShardConfigSettings()` returns values that match the provider schema defaults (`resource_shard_config.go`): `chaining_allowed: true`, `heartbeat_interval_millis: 1000`, `heartbeat_timeout_secs: 10`, `election_timeout_millis: 10000`. These are applied when `ShardConfig` is nil. (CDKTN-016)

---

## EARS Requirement Traceability

| EARS ID | Requirement Summary | Status | File / Function |
|---------|---------------------|--------|-----------------|
| CDKTN-001 | Three L2 constructs: `MongoShard`, `MongoConfigServer`, `MongoMongos` | Done | `shard.go`, `config_server.go`, `mongos.go` |
| CDKTN-002 | L3 `MongoShardedCluster` composing all L2s | Done | `cluster.go:NewMongoShardedCluster` |
| CDKTN-003 | L2 props accept `Members []MemberConfig` with `Host` + `Port` | Done | `types.go:MemberConfig` |
| CDKTN-004 | Provider alias pattern `<type>_<rsname>_<idx>` | Done | `provider_factory.go:ProviderAliasName` |
| CDKTN-005 | Provider alias includes host, port, username, password, auth_database | Done | `provider_factory.go:BuildProviderConfig` |
| CDKTN-006 | `required_providers` source = `registry.terraform.io/zph/mongodb` | Done | `constants.go:ProviderSource`, `stack.go:Synth` |
| CDKTN-007 | `CredentialSource` interface + `DirectCredentials` + `EnvCredentials` | Done | `credentials.go` |
| CDKTN-008 | `SecretsManagerCredentials` implementation | Not done | See Future Work |
| CDKTN-009 | Cluster-level credentials cascade to all member aliases | Done | `cluster.go:NewMongoShardedCluster` |
| CDKTN-010 | Per-member credential override on `MemberConfig.Credentials` | Done | `provider_factory.go:BuildProviderConfig` |
| CDKTN-011 | `mongodb_db_user` resource per user per member | Done | `resource_builder.go:BuildUsers` |
| CDKTN-012 | `mongodb_db_role` resource per role per member | Done | `resource_builder.go:BuildRoles` |
| CDKTN-013 | User resources have `depends_on` all roles on same alias | Done | `resource_builder.go:BuildUsers` |
| CDKTN-014 | User resource includes username, password, database, roles | Done | `resource_builder.go:buildUserConfig` |
| CDKTN-015 | One `mongodb_shard_config` per RS targeting first member | Done | `resource_builder.go:BuildShardConfig` |
| CDKTN-016 | Shard config defaults match provider schema defaults | Done | `types.go:DefaultShardConfigSettings`, `constants.go` |
| CDKTN-017 | No `mongodb_shard_config` for mongos | Done | `mongos.go:NewMongoMongos` (omitted) |
| CDKTN-018 | SSL enabled at cluster level propagates to all providers | Done | `provider_factory.go:BuildProviderConfig`, `cluster.go` |
| CDKTN-019 | Certificate PEM included in provider when set | Done | `provider_factory.go:BuildProviderConfig` |
| CDKTN-020 | `insecure_skip_verify` propagated when set | Done | `provider_factory.go:BuildProviderConfig` |
| CDKTN-021 | `MongoConfigServer` with < 3 members returns error | Done | `validation.go:ValidateReplicaSetMembers` |
| CDKTN-022 | `MongoShard` with < 3 members returns error | Done | `validation.go:ValidateReplicaSetMembers` |
| CDKTN-023 | Empty `Mongos` slice returns error | Done | `validation.go:ValidateClusterMongos` |
| CDKTN-024 | Empty `Shards` slice returns error | Done | `validation.go:ValidateClusterShards` |
| CDKTN-025 | Duplicate `host:port` within a construct returns error | Done | `validation.go:ValidateDuplicateHostPort` |
| CDKTN-026 | > 50 members returns error | Done | `validation.go:ValidateReplicaSetMembers`, `constants.go:MaxMembers` |
| CDKTN-027 | > 7 voting members logs warning | Done | `validation.go:WarnVotingMemberCount` |
| CDKTN-028 | Duplicate RS names across cluster returns error | Done | `validation.go:ValidateDuplicateRSNames`, `cluster.go` |
| CDKTN-029 | Deterministic JSON output (byte-identical across runs) | Done | `stack.go:Synth` (sorted providers + resources) |
| CDKTN-030 | Single `required_providers` block regardless of alias count | Done | `stack.go:Synth` |
| CDKTN-031 | `required_version = ">= 1.7.5"` | Done | `constants.go:DefaultTerraformVersion`, `stack.go:Synth` |
| CDKTN-032 | `MongoShardedClusterProps` with Mongos, ConfigServers, Shards, Credentials, SSL, Users | Done | `types.go:MongoShardedClusterProps` |
| CDKTN-033 | `ShardConfig` includes `ReplicaSetName` + `Members`; empty RS name is an error | Done | `types.go:ShardConfig`, `validation.go:ValidateReplicaSetName` |
| CDKTN-034 | Cluster-level `Proxy` URL propagates to all provider aliases | Done | `provider_factory.go:BuildProviderConfig` |
| CDKTN-035 | `direct = true` for shard and config server providers | Done | `provider_factory.go:BuildProvidersWithOffset` |
| CDKTN-036 | `direct = false` for mongos providers | Done | `provider_factory.go:BuildProvidersWithOffset` |
| CDKTN-037 | Cluster-level users on all mongos + first member of each shard | Done | `cluster.go:collectClusterUserTargets` |
| CDKTN-038 | L2-level users scoped to that construct's members only | Done | `resource_builder.go:BuildUsers` (alias-scoped) |
| CDKTN-039 | `retrywrites = true` on all provider aliases | Done | `provider_factory.go:BuildProviderConfig` |
| CDKTN-040 | `auth_database = "admin"` default on all providers | Done | `provider_factory.go:BuildProviderConfig`, `constants.go:DefaultAuthDatabase` |
| CDKTN-041 | Custom `auth_database` per member override | Not done | Not implemented in `MemberConfig` |
| CDKTN-042 | `ProviderVersion` field in cluster props | Done | `types.go:MongoShardedClusterProps`, `cluster.go` |
| CDKTN-043 | Unit tests with golden file comparisons | Done | `*_test.go`, `testdata/*.json` |
| CDKTN-044 | Integration tests running `terraform validate` | Not done | See Future Work |
| CDKTN-045 | E2E tests using `testcontainers-go` | Not done | See Future Work |
| CDKTN-046 | Buildable via `make cdktn-build`, testable via `make cdktn-test` | Done | `Makefile` targets |
| CDKTN-047 | Published as consumable Go module | Not done | Not yet tagged/released |
| CDKTN-048 | `go.mod` declares CDKTN + constructs-go dependencies | Not done | Intentional deferral — see Future Work |
| CDKTN-049 | L1 bindings generated via `cdktn get` | Not done | Intentional deferral — see Future Work |
| CDKTN-050 | L2 constructs use L1 binding types | Not done | Intentional deferral — see Future Work |

---

## Build Commands

All commands run from the repository root. Terraform is managed via [Hermit](https://github.com/cashapp/hermit) (`.hermit/`) to pin the version. Activate the Hermit environment before running any `terraform` or `make` commands that invoke Terraform:

```bash
. .hermit/bin/activate-hermit
```

| Command | Description |
|---------|-------------|
| `make cdktn-build` | Build the construct library (`go build ./...` in `cdktn/`) |
| `make cdktn-test` | Run unit tests (`go test ./...` in `cdktn/`) |
| `make cdktn-test-golden` | Regenerate golden files (`UPDATE_GOLDEN=1 go test ./...`) |
| `make test-golden` | Run golden file tests against MongoDB container |
| `make test-golden-update` | Regenerate provider golden files |
| `make test` | Run all tests: unit + cdktn + terraform plan |
| `make help` | Show all Makefile targets |

To update golden files after an intentional synthesis change:

```bash
make cdktn-test-golden
```

To run only cdktn tests with verbose output:

```bash
cd cdktn && go test -v ./...
```

---

## Testing Approach

Tests live in `cdktn/*_test.go` (package `cdktn`, same package — white-box access). All tests use `github.com/stretchr/testify`.

**Unit tests** validate:
- Constructor error paths (invalid member counts, duplicates, empty slices)
- Synthesized JSON structure via `SynthToMap()` assertions
- Golden file comparisons via `goldenCompare()` in `testutil_test.go`

**Golden files** (`testdata/*.json`) are committed JSON snapshots of known-good synthesis output. The test helper reads the file, compares byte-for-byte, and on mismatch either fails (default) or overwrites the file when `UPDATE_GOLDEN=1` is set.

**Integration and E2E tests are not yet implemented** (CDKTN-044, CDKTN-045).

---

## Golden File Testing Engine

The provider includes a golden file testing engine that captures deterministic snapshots of all MongoDB commands sent during each example's resource lifecycle (Create, Read, Update, Delete). Golden files are stored in `mongodb/testdata/golden/` as human-readable audit trails.

### Architecture

| File | Purpose |
|------|---------|
| `mongodb/command_recorder_test.go` | `CommandRecorder` type + unit tests (no build tag) |
| `mongodb/golden_compare_test.go` | `goldenCompare` helper (integration tag) |
| `mongodb/golden_test.go` | All golden integration tests (integration tag) |
| `mongodb/testdata/golden/*.golden` | Auto-generated golden files |
| `docs/specs/golden-test-requirements.md` | EARS specification (GOLDEN-001 through GOLDEN-018) |

### CommandRecorder

`CommandRecorder` hooks into the MongoDB driver's `event.CommandMonitor` to capture commands:

- **Filters** noise commands: `hello`, `saslStart`, `saslContinue`, `ping`, `endSessions`, `isMaster`, `ismaster`, `buildInfo`, `getFreeMonitoringStatus`, `getLog`
- **Strips** driver-injected metadata: `$db`, `$readPreference`, `lsid`, `$clusterTime`
- **Redacts** password fields: `pwd` values become `[REDACTED]`
- **Output format**: deterministic multi-line string with `Source:` (test name + file), `Command:`, `Database:`, and `Body:` sections

### Golden Files

Each test captures commands for one example configuration's lifecycle and compares against a committed `.golden` file:

- `db_user_basic.golden` — single user with one role (CRUD)
- `db_user_custom_role.golden` — custom role + user (create + delete)
- `db_user_multiple_roles.golden` — user with 4 roles (CRUD)
- `db_user_import.golden` — import existing user
- `db_role_basic.golden` — single privilege role (CRUD)
- `db_role_cluster_privilege.golden` — cluster-level privilege
- `db_role_composite.golden` — 3 roles with inheritance
- `db_role_inherited.golden` — base + derived role
- `shard_config_basic.golden` — replSetReconfig + read (normalized)
- `shard_config_mongos_discovery.golden` — mongos discovery + shard RS reconfig round-trip (sharded normalization)
- `shard_config_multi_shard.golden` — mongos discovery + independent RS reads on both shards (sharded normalization)
- `original_user.golden` — bootstrap admin user
- `pattern_monitoring_user.golden` — monitoring role + exporter user
- `pattern_role_hierarchy.golden` — 3-tier role hierarchy with 3 users

### Shard Config Normalization

Shard config output contains dynamic values (ObjectIDs, container-assigned host:port, version numbers). `normalizeReplSetBody()` replaces these with stable placeholders before golden comparison.

### Usage

```bash
# Run golden tests (requires Docker)
make test-golden

# Regenerate golden files after intentional changes
make test-golden-update
```

### Sharded Golden Tests

`TestGolden_ShardConfig_MongosDiscovery` and `TestGolden_ShardConfig_MultiShard` require a mongos + multi-shard topology and run as part of `make test-sharded-integration`. They use `normalizeShardedBody` which extends `normalizeReplSetBody` with shard host string and state normalization.

---

## Sharded Integration Tests

End-to-end integration tests exercising shard discovery and multi-shard configuration against a real MongoDB sharded cluster running in Docker via testcontainers-go.

### Cluster Topology

| Component | Network Alias | RS Name | Internal Port | Process |
|-----------|--------------|---------|---------------|---------|
| Config Server | `configsvr0` | `configRS` | 27019 | `mongod --configsvr` |
| Shard 1 | `shard01svr0` | `shard01` | 27018 | `mongod --shardsvr` |
| Shard 2 | `shard02svr0` | `shard02` | 27018 | `mongod --shardsvr` |
| Mongos | `mongos0` | N/A | 27017 | `mongos` |

The cluster is lazily initialized on first test use via `sync.Once` and torn down in `TestMain`. No inter-component authentication (no keyfile). Admin user created on mongos and each shard after formation.

### Tests

| Test | EARS ID | Validates |
|------|---------|-----------|
| `TestShardedIntegration_DetectConnectionType_Mongos` | SINTEG-005 | `DetectConnectionType` returns `ConnTypeMongos` against mongos |
| `TestShardedIntegration_DetectConnectionType_ShardRS` | SINTEG-013 | `DetectConnectionType` returns `ConnTypeReplicaSet` against shard direct |
| `TestShardedIntegration_ListShards_ReturnsBothShards` | SINTEG-006 | `ListShards` returns 2 shards with correct IDs |
| `TestShardedIntegration_FindShardByName_Found` | SINTEG-007 | `FindShardByName` matches real shard |
| `TestShardedIntegration_FindShardByName_NotFound` | SINTEG-008 | `FindShardByName` errors for bogus name, lists available |
| `TestShardedIntegration_ResolveShardClient_WithHostOverride` | SINTEG-009 | `ResolveShardClient` with `host_override` returns working client |
| `TestShardedIntegration_ResolveShardClient_GetReplSetConfig` | SINTEG-010 | `GetReplSetConfig` via resolved shard client returns correct RS name |
| `TestShardedIntegration_ResolveShardClient_SetReplSetConfig_RoundTrip` | SINTEG-011 | `SetReplSetConfig` on shard via discovery persists and round-trips |
| `TestShardedIntegration_MultiShard_IndependentClients` | SINTEG-012 | Both shards discoverable, have different RS names |
| `TestShardedIntegration_ResolveShardClient_DirectRS_Passthrough` | SINTEG-014 | `ResolveShardClient` on direct RS returns same client (no mongos discovery) |

### host_override Strategy

Containers use internal Docker hostnames (e.g., `shard01svr0:27018`). `ListShards` returns these internal names, which are unreachable from the test host. Each shard container exposes its port via testcontainers port mapping. Tests pass `host_override = "localhost:<mapped_port>"` to `ResolveShardClient`, which uses the override instead of the internal hostname.

### Usage

```bash
# Run sharded integration tests (requires Docker)
make test-sharded-integration

# Or directly:
go test -tags integration -run TestShardedIntegration -v -timeout 600s ./mongodb/
```

---

## Future Work

### CDKTN Framework Integration (CDKTN-048, CDKTN-049, CDKTN-050)

When `github.com/open-constructs/cdk-terrain-go/cdktn` packages are stable and available:

1. Replace `TerraformStack` in `stack.go` with the real `cdktn.TerraformStack` embedding a construct node.
2. Run `cdktn get` to generate L1 bindings under `cdktn/generated/mongodb/`.
3. Replace `map[string]interface{}` resource construction in `resource_builder.go` with typed L1 constructor calls (e.g., `mongodb.NewDbUser()`).
4. Update `go.mod` to declare `github.com/open-constructs/cdk-terrain-go/cdktn` and `github.com/aws/constructs-go/constructs/v10`.
5. Add a `cdktf.json` for L1 generation configuration.

The construct shapes (`MongoShard`, `MongoConfigServer`, `MongoMongos`, `MongoShardedCluster`) and props structs do not need to change — only the internal synthesis mechanism.

### SecretsManagerCredentials (CDKTN-008)

Add a third `CredentialSource` implementation that emits `data.aws_secretsmanager_secret_version` references instead of literal credential values. Requires access to the CDKTN `TerraformStack` for data source block emission; currently deferred because the custom `TerraformStack` does not model data sources.

### Custom Auth Database per Member (CDKTN-041)

`MemberConfig` does not currently support an `AuthDatabase` field. When added, `BuildProviderConfig` should prefer `member.AuthDatabase` over `DefaultAuthDatabase`.

### Integration Tests — terraform validate (CDKTN-044)

Add `integration_test.go` with build tag `integration` that:
1. Synthesizes a cluster to a temp directory.
2. Runs `terraform init` + `terraform validate` against the output using a locally-built provider binary.

### E2E Tests — testcontainers (CDKTN-045)

Add `e2e_test.go` with build tag `e2e` using `testcontainers-go` to:
1. Start a minimal sharded cluster in Docker (1 config RS + 1 shard RS + 1 mongos).
2. Synthesize cluster configuration and run `terraform apply`.
3. Assert users and roles are queryable on the containerized cluster.

### Go Module Release (CDKTN-047)

Tag a release (`v0.1.0`) once integration tests pass so the library can be consumed via `go get github.com/zph/terraform-provider-mongodb/cdktn@v0.1.0`.

---

## Known Limitations

- `auth_database` cannot be customized per member (always `"admin"`).
- No data source support in the synthesis engine (needed for Secrets Manager).
- No `cdktf.json` for L1 generation — resources are emitted as raw maps.
- `WarnVotingMemberCount` writes to `log.Default()`. In test contexts this goes to stderr; no mechanism to capture or suppress it.
