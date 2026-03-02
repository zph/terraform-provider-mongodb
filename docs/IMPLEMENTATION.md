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
