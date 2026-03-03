# CDKTN Sharded MongoDB Cluster Generator Requirements

## Overview

This document specifies the functional requirements for a Go construct library built on CDK Terrain (CDKTN) that generates Terraform configurations for managing sharded MongoDB clusters using `terraform-provider-mongodb` (registry.terraform.io/zph/mongodb).

The generator replaces the existing Jinja2 template (`main.tf.j2`) with typed, composable Go constructs that encode MongoDB sharded cluster topology knowledge and produce deterministic Terraform JSON output via CDKTN synthesis.

**System Name:** CDKTN Sharded Cluster Generator
**Provider:** terraform-provider-mongodb (zph/mongodb)
**Implementation Language:** Go
**Framework:** CDK Terrain (CDKTN) v0.22+ — community fork of CDKTF
**Framework Go Module:** `github.com/open-constructs/cdk-terrain-go/cdktn`
**Constructs Base:** `github.com/aws/constructs-go/constructs/v10`
**Version:** 1.0
**Last Updated:** 2026-03-02

## Terminology

| Term | Definition |
|------|-----------|
| **CDKTN** | CDK Terrain — community-maintained fork of HashiCorp's CDKTF, under Open Constructs Foundation |
| **L1 Construct** | Auto-generated 1:1 Go binding to a single Terraform resource (via `cdktn get`) |
| **L2 Construct** | Higher-level Go struct composing multiple L1s with opinionated defaults |
| **L3 Construct** | Pattern-level Go struct composing multiple L2s into a complete architecture |
| **CSRS** | Config Server Replica Set — stores sharded cluster metadata |
| **mongos** | Stateless query router — client entry point to a sharded cluster |
| **Shard RS** | Shard Replica Set — holds a partition of the sharded data |
| **Provider Alias** | A named Terraform provider instance bound to a specific MongoDB host:port |
| **Synthesis** | The CDKTN process of converting the construct tree into Terraform JSON (`app.Synth()`) |
| **Construct Tree** | The hierarchical graph of constructs rooted at `cdktn.App`, walked during synthesis |

## MongoDB Sharded Cluster Topology Reference

```
                    +-----------+     +-----------+
  Clients -------->| mongos(1) |     | mongos(M) |
                    +-----+-----+     +-----+-----+
                          |                 |
                  +-------+-----------------+-------+
                  |                                 |
        +---------+---------+             +---------+---------+
        | Config Server RS  |             |   Shard N RS      |
        | (3+ members)      |             |   (3+ members)    |
        +-------------------+             +-------------------+
                                  ...
        +-------------------+
        |   Shard 1 RS      |
        |   (3+ members)    |
        +-------------------+
```

Minimal production cluster: 3 config servers + N shards x 3 members + M mongos = 10+ nodes.

## CDKTN Go Construct Pattern Reference

All constructs follow the CDKTN Go pattern: a Go struct that embeds `constructs.Construct` and composes child resources in its constructor. The `(scope, id, props)` convention applies:

```go
type MongoShard struct {
    constructs.Construct
}

type MongoShardProps struct {
    ReplicaSetName string
    Members        []MemberConfig
    // ...
}

func NewMongoShard(scope constructs.Construct, id string, props *MongoShardProps) *MongoShard {
    s := &MongoShard{}
    constructs.NewConstruct_Override(s, scope, &id)
    // compose L1 resources here
    return s
}
```

Synthesis is triggered by `app.Synth()` which walks all `cdktn.TerraformStack` children, calling `ToTerraform()` on each `TerraformElement` and deep-merging results into JSON output at `cdktf.out/stacks/<stackName>/`.

## Requirements

### 1. Construct Hierarchy

**CDKTN-001:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL expose three L2 construct types as exported Go structs: `MongoMongos`, `MongoConfigServer`, and `MongoShard`.

**Rationale:**
Each MongoDB sharded cluster component type has distinct configuration semantics (stateless router vs. metadata store vs. data holder). Separate constructs enable independent composition and configuration.

**Verification:**
Confirm that each struct is exported, has a `New*` constructor, and can be instantiated independently within a `cdktn.TerraformStack`.

---

**CDKTN-002:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL expose one L3 construct type `MongoShardedCluster` as an exported Go struct that composes `MongoMongos`, `MongoConfigServer`, and `MongoShard` constructs into a complete cluster topology.

**Rationale:**
Operators deploying a full sharded cluster need a single entry point that wires all components together with correct provider aliases, dependency ordering, and shared credentials.

**Verification:**
Instantiate a `MongoShardedCluster` with a minimal valid configuration and verify it synthesizes provider blocks, user/role resources, and shard config resources for all node types.

---

**CDKTN-003:** Ubiquitous

**Requirement:**
Each L2 construct (`MongoMongos`, `MongoConfigServer`, `MongoShard`) SHALL accept a `Members` slice in its props struct where each element specifies `Host` (string) and `Port` (int).

**Rationale:**
Every MongoDB node in a sharded cluster is addressed by host:port. The provider requires a separate alias per node, so each member must be individually addressable.

**Verification:**
Instantiate an L2 construct with 3 members and verify 3 distinct provider alias blocks appear in synthesized output.

---

### 2. Provider Alias Generation

**CDKTN-004:** Event Driven

**Requirement:**
WHEN a member is added to any L2 construct, the CDKTN Sharded Cluster Generator SHALL generate a uniquely-named Terraform provider alias using the pattern `<component_type>_<replica_set_name>_<member_index>`.

**Rationale:**
Terraform requires unique alias names for each provider instance. Deterministic naming from component type and index ensures no collisions across construct boundaries and makes the output readable.

**Verification:**
Add members to a `MongoShard` with `ReplicaSetName: "shard01"` and verify aliases `shard_shard01_0`, `shard_shard01_1`, `shard_shard01_2` appear in output.

---

**CDKTN-005:** Ubiquitous

**Requirement:**
Each generated provider alias SHALL include the `host`, `port`, `username`, `password`, and `auth_database` attributes from the member and credential configuration.

**Rationale:**
These are the minimum required attributes for `terraform-provider-mongodb` to establish a connection (per `provider.go` schema).

**Verification:**
Synthesize a stack and verify each provider block contains all five attributes with correct values.

---

**CDKTN-006:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL set the provider `source` to `registry.terraform.io/zph/mongodb` in the generated `required_providers` block.

**Rationale:**
The provider is published under the `zph` namespace. Incorrect source would cause `terraform init` to fail.

**Verification:**
Synthesize a stack and verify the `required_providers` block contains the correct source string.

---

### 3. Credential Management

**CDKTN-007:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL accept credentials through a `CredentialSource` Go interface that supports at minimum two implementations: `DirectCredentials` (literal username/password strings) and `EnvCredentials` (environment variable name references).

**Rationale:**
The existing provider supports env vars (`MONGO_USR`, `MONGO_PWD`) and direct values. Operators need flexibility to avoid hardcoding secrets in generated Terraform.

**Verification:**
Instantiate a cluster with each credential implementation and verify the synthesized provider blocks use literal values or empty attributes (letting env vars take effect) accordingly.

---

**CDKTN-008:** Optional Feature

**Requirement:**
WHERE AWS Secrets Manager credential source is configured via a `SecretsManagerCredentials` implementation, the CDKTN Sharded Cluster Generator SHALL generate `data.aws_secretsmanager_secret_version` data source references in the provider credential attributes.

**Rationale:**
Production deployments store credentials in secret managers. The existing Jinja2 template has a TODO for AWS Secrets Manager support (`main.tf.j2:25`).

**Verification:**
Configure `SecretsManagerCredentials` and verify the synthesized output includes the data source block and provider attributes reference `data.aws_secretsmanager_secret_version.*.secret_string`.

---

**CDKTN-009:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL support applying a single `CredentialSource` at the cluster level that propagates to all member provider aliases.

**Rationale:**
Most clusters share a single admin credential across all nodes. Per-node credential overrides are handled by CDKTN-010.

**Verification:**
Set cluster-level credentials and verify all provider alias blocks use those credentials.

---

**CDKTN-010:** Optional Feature

**Requirement:**
WHERE per-member credential overrides are specified via the `Credentials` field on a `MemberConfig`, the CDKTN Sharded Cluster Generator SHALL use the member-level credentials for that member's provider alias instead of the cluster-level credentials.

**Rationale:**
Some deployments use distinct credentials per shard or per node for security isolation.

**Verification:**
Override credentials on one member and verify only that member's provider alias uses the override.

---

### 4. User and Role Management

**CDKTN-011:** Event Driven

**Requirement:**
WHEN a `Users` slice is provided to an L2 construct's props, the CDKTN Sharded Cluster Generator SHALL generate a `mongodb_db_user` resource for each user on each member's provider alias.

**Rationale:**
MongoDB users must be created independently on each node in a sharded cluster (mongos users differ from shard-local users). The provider's `mongodb_db_user` resource operates against a single connection.

**Verification:**
Add 2 users to a 3-member construct and verify 6 `mongodb_db_user` resources appear in synthesized output, each bound to the correct provider alias.

---

**CDKTN-012:** Event Driven

**Requirement:**
WHEN a `Roles` slice is provided to an L2 construct's props, the CDKTN Sharded Cluster Generator SHALL generate a `mongodb_db_role` resource for each role on each member's provider alias.

**Rationale:**
Custom roles must exist on each node before users referencing those roles can be created.

**Verification:**
Add 1 custom role to a 3-member construct and verify 3 `mongodb_db_role` resources appear in synthesized output.

---

**CDKTN-013:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL generate `depends_on` references from each `mongodb_db_user` resource to all `mongodb_db_role` resources on the same provider alias.

**Rationale:**
Users reference roles. If a role does not exist when the user is created, Terraform apply fails. This mirrors the pattern in `examples/main.tf.sample:41`.

**Verification:**
Synthesize a stack with roles and users and verify each user resource's `depends_on` includes the corresponding role resources.

---

**CDKTN-014:** Ubiquitous

**Requirement:**
Each generated `mongodb_db_user` resource SHALL include `username`, `password`, `database`, and `roles` attributes matching the user definition.

**Rationale:**
These are the required attributes for the `mongodb_db_user` resource schema.

**Verification:**
Synthesize and verify user resources contain all required attributes with correct values.

---

### 5. Shard Configuration

**CDKTN-015:** Event Driven

**Requirement:**
WHEN `ShardConfig` settings are provided to a `MongoShard` or `MongoConfigServer` construct, the CDKTN Sharded Cluster Generator SHALL generate a `mongodb_shard_config` resource targeting one member of that replica set.

**Rationale:**
`replSetReconfig` commands must be issued to a single member (typically the primary) of each replica set. One resource per RS is sufficient since the configuration propagates to all members.

**Verification:**
Provide shard config settings to a `MongoShard` and verify exactly one `mongodb_shard_config` resource is generated per replica set.

---

**CDKTN-016:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL use the following defaults for `mongodb_shard_config` attributes WHEN not explicitly provided: `chaining_allowed: true`, `heartbeat_interval_millis: 1000`, `heartbeat_timeout_secs: 10`, `election_timeout_millis: 10000`.

**Rationale:**
These match the provider's schema defaults (`resource_shard_config.go:169-188`) and MongoDB's recommended production values.

**Verification:**
Synthesize a shard config without explicit settings and verify defaults appear in the output.

---

**CDKTN-017:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL NOT generate `mongodb_shard_config` resources for `MongoMongos` constructs.

**Rationale:**
mongos instances are stateless routers and do not form replica sets. `replSetGetConfig` is not valid against a mongos.

**Verification:**
Add shard config settings at the cluster level and verify no shard config resource targets a mongos provider alias.

---

### 6. SSL/TLS Configuration

**CDKTN-018:** Optional Feature

**Requirement:**
WHERE `SSL` is set to `true` in the cluster-level `SSLConfig`, the CDKTN Sharded Cluster Generator SHALL set `ssl = true` on all generated provider aliases.

**Rationale:**
Production MongoDB clusters typically require TLS for all connections. Cluster-wide SSL avoids repetitive per-node configuration.

**Verification:**
Enable cluster-level SSL and verify all provider blocks include `ssl = true`.

---

**CDKTN-019:** Optional Feature

**Requirement:**
WHERE a `Certificate` PEM string is provided in `SSLConfig`, the CDKTN Sharded Cluster Generator SHALL include the `certificate` attribute in the corresponding provider alias blocks.

**Rationale:**
Self-signed or private CA certificates must be passed to the provider for TLS verification.

**Verification:**
Provide a certificate string and verify it appears in the provider blocks.

---

**CDKTN-020:** Optional Feature

**Requirement:**
WHERE `InsecureSkipVerify` is set to `true` in `SSLConfig`, the CDKTN Sharded Cluster Generator SHALL set `insecure_skip_verify = true` on the corresponding provider alias blocks.

**Rationale:**
Development and testing environments often use self-signed certificates without proper hostname matching.

**Verification:**
Set `InsecureSkipVerify` to true and verify the attribute appears in provider blocks.

---

### 7. Validation

**CDKTN-021:** Unwanted Behaviour

**Requirement:**
IF a `MongoConfigServer` construct is instantiated with fewer than 3 members, THEN the CDKTN Sharded Cluster Generator SHALL return an error stating the minimum member count.

**Rationale:**
MongoDB requires a minimum of 3 config server replica set members for production. Catching this at construction time prevents deploy-time failures.

**Verification:**
Instantiate a `MongoConfigServer` with 2 members and verify the constructor returns a non-nil error.

---

**CDKTN-022:** Unwanted Behaviour

**Requirement:**
IF a `MongoShard` construct is instantiated with fewer than 3 members, THEN the CDKTN Sharded Cluster Generator SHALL return an error stating the minimum member count.

**Rationale:**
MongoDB shard replica sets require a minimum of 3 members for automatic failover. The provider's `MaxMembers` constant is 50 (`replica_set_types.go:35`).

**Verification:**
Instantiate a `MongoShard` with 1 member and verify the constructor returns a non-nil error.

---

**CDKTN-023:** Unwanted Behaviour

**Requirement:**
IF a `MongoShardedCluster` is instantiated with an empty `Mongos` slice, THEN the CDKTN Sharded Cluster Generator SHALL return an error stating that at least one mongos instance is required.

**Rationale:**
A sharded cluster without a mongos has no client-accessible entry point.

**Verification:**
Instantiate a cluster with shards and config servers but empty mongos and verify the error.

---

**CDKTN-024:** Unwanted Behaviour

**Requirement:**
IF a `MongoShardedCluster` is instantiated with an empty `Shards` slice, THEN the CDKTN Sharded Cluster Generator SHALL return an error stating that at least one shard is required.

**Rationale:**
A sharded cluster with no shards cannot store data.

**Verification:**
Instantiate a cluster with mongos and config servers but no shards and verify the error.

---

**CDKTN-025:** Unwanted Behaviour

**Requirement:**
IF two members within the same construct share an identical `Host:Port` combination, THEN the CDKTN Sharded Cluster Generator SHALL return an error identifying the duplicate.

**Rationale:**
Duplicate host:port means two provider aliases would target the same mongod, causing conflicting Terraform state.

**Verification:**
Add two members with the same host and port and verify the error identifies the duplicates.

---

**CDKTN-026:** Unwanted Behaviour

**Requirement:**
IF a `MongoShard` or `MongoConfigServer` construct has more than 50 members, THEN the CDKTN Sharded Cluster Generator SHALL return an error stating the maximum member count.

**Rationale:**
MongoDB limits replica sets to 50 members (`MaxMembers = 50` in `replica_set_types.go:35`).

**Verification:**
Instantiate a construct with 51 members and verify the error.

---

**CDKTN-027:** Unwanted Behaviour

**Requirement:**
IF a `MongoShard` or `MongoConfigServer` construct has more than 7 voting members, THEN the CDKTN Sharded Cluster Generator SHALL log a warning via the standard `log` package stating the MongoDB voting member limit.

**Rationale:**
MongoDB limits voting members to 7 (`MaxVotingMembers = 7` in `replica_set_types.go:34`). This is a warning rather than an error because non-voting members are valid.

**Verification:**
Instantiate a construct with 8 members where all have votes enabled and verify the warning is logged.

---

**CDKTN-028:** Unwanted Behaviour

**Requirement:**
IF two shard RS definitions within a `MongoShardedCluster` share the same `ReplicaSetName`, THEN the CDKTN Sharded Cluster Generator SHALL return an error identifying the duplicate name.

**Rationale:**
MongoDB requires unique replica set names across all components of a sharded cluster.

**Verification:**
Define two shards with the same `ReplicaSetName` and verify the error.

---

### 8. Synthesis and Output

**CDKTN-029:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL produce deterministic Terraform JSON output such that two `app.Synth()` runs with identical inputs produce byte-identical output.

**Rationale:**
Deterministic output is required for reliable `terraform plan` diffs and CI/CD pipelines. Non-deterministic output (e.g., from Go map iteration order) creates phantom diffs.

**Verification:**
Synthesize the same stack configuration twice and compare output with a diff tool.

---

**CDKTN-030:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL generate a single `terraform.required_providers` block containing one entry for `mongodb` regardless of how many provider aliases are generated.

**Rationale:**
Terraform requires exactly one `required_providers` entry per provider type. Multiple aliases share the same provider binary.

**Verification:**
Synthesize a multi-shard cluster and verify only one `required_providers` entry exists.

---

**CDKTN-031:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL set the Terraform `required_version` constraint to `>= 1.7.5` in the generated output.

**Rationale:**
This matches the existing constraint in `examples/main.tf.sample:2` and ensures compatibility with the provider.

**Verification:**
Synthesize and verify the `required_version` constraint in output.

---

### 9. Construct Configuration Interface

**CDKTN-032:** Ubiquitous

**Requirement:**
The `MongoShardedCluster` L3 construct SHALL accept a `MongoShardedClusterProps` struct with the following fields: `Mongos` (slice of `MongosConfig`), `ConfigServers` (`ConfigServerConfig`), `Shards` (slice of `ShardConfig`), `Credentials` (`CredentialSource` interface), `SSL` (`*SSLConfig`), and `Users` (slice of `UserConfig`).

**Rationale:**
A single props struct provides a declarative, type-safe cluster definition. The Go compiler enforces required fields via struct initialization.

**Verification:**
Verify the Go struct definition contains all listed fields and that the Go compiler rejects invalid types for each field.

---

**CDKTN-033:** Ubiquitous

**Requirement:**
Each `ShardConfig` struct SHALL include a `ReplicaSetName` (string) and `Members` (slice of `MemberConfig`).

**Rationale:**
The `shard_name` attribute in `mongodb_shard_config` maps to the replica set `_id`. Each shard must have a unique RS name.

**Verification:**
Verify that a `ShardConfig` with an empty `ReplicaSetName` triggers a validation error at construction time.

---

### 10. Proxy Configuration

**CDKTN-034:** Optional Feature

**Requirement:**
WHERE a `Proxy` URL string is provided in the cluster-level props, the CDKTN Sharded Cluster Generator SHALL set the `proxy` attribute on all generated provider alias blocks.

**Rationale:**
The provider supports SOCKS5 proxy connections (`provider.go` proxy attribute). Some deployments require proxy access to reach MongoDB nodes.

**Verification:**
Provide a proxy URL and verify all provider blocks include the `proxy` attribute.

---

### 11. Direct Connection Mode

**CDKTN-035:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL set `direct = true` on all provider aliases targeting shard replica set members and config server replica set members.

**Rationale:**
When configuring individual replica set members (for `replSetReconfig` or user management), the provider must connect directly to the specified host rather than discovering the replica set topology and redirecting to the primary.

**Verification:**
Synthesize a cluster and verify all shard and config server provider aliases include `direct = true`.

---

**CDKTN-036:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL set `direct = false` (or omit the attribute) on all provider aliases targeting mongos instances.

**Rationale:**
mongos instances are standalone routers. Direct mode is unnecessary and the default driver behavior (service discovery) is correct for mongos.

**Verification:**
Synthesize a cluster and verify mongos provider aliases do not include `direct = true`.

---

### 12. Cluster-Level User Propagation

**CDKTN-037:** Event Driven

**Requirement:**
WHEN users are defined at the `MongoShardedCluster` level via the `Users` field, the CDKTN Sharded Cluster Generator SHALL generate `mongodb_db_user` resources on all mongos provider aliases AND on all shard primary provider aliases (first member of each shard).

**Rationale:**
Cluster-wide users (e.g., application users) must be accessible via mongos. Shard-local copies are needed for direct shard access during maintenance. This matches the pattern in `examples/main.tf.sample` where the same user module is applied to both mongos and shard1.

**Verification:**
Define cluster-level users with 2 mongos and 2 shards and verify user resources are generated for all mongos members and the first member of each shard.

---

**CDKTN-038:** Event Driven

**Requirement:**
WHEN users are defined at an individual L2 construct level, the CDKTN Sharded Cluster Generator SHALL generate `mongodb_db_user` resources ONLY on that construct's member provider aliases.

**Rationale:**
Some users (e.g., monitoring exporters, failover managers) are scoped to specific nodes. Per the `examples/modules/users/` structure, different user types target different nodes.

**Verification:**
Define a user only on one shard construct and verify the user resource does not appear on other shards or mongos.

---

### 13. Retry Writes Configuration

**CDKTN-039:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL set `retrywrites = true` on all generated provider aliases by default.

**Rationale:**
The provider defaults to `retrywrites = true` (`provider.go` schema). Retryable writes are a MongoDB best practice for sharded clusters to handle transient failovers.

**Verification:**
Synthesize a cluster without explicit retry config and verify `retrywrites = true` on all provider blocks.

---

### 14. Auth Database Configuration

**CDKTN-040:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL default the `auth_database` attribute to `"admin"` on all generated provider aliases.

**Rationale:**
Administrative operations (user creation, role management, replica set configuration) authenticate against the `admin` database. This matches the provider's default and all existing examples.

**Verification:**
Synthesize without explicit auth_database and verify all provider blocks use `"admin"`.

---

**CDKTN-041:** Optional Feature

**Requirement:**
WHERE a custom `AuthDatabase` is specified on a `MemberConfig` or construct-level props, the CDKTN Sharded Cluster Generator SHALL use that value instead of the default `"admin"`.

**Rationale:**
Some deployments use non-standard auth databases for isolation.

**Verification:**
Set a custom auth database on one member and verify it appears in that member's provider block while others retain `"admin"`.

---

### 15. Provider Version Constraint

**CDKTN-042:** Ubiquitous

**Requirement:**
The `MongoShardedClusterProps` struct SHALL include a `ProviderVersion` string field that is used as the version constraint in the generated `required_providers` block.

**Rationale:**
The provider uses version `9.9.9` during development. Production deployments need real semver constraints. The version must be configurable, not hardcoded.

**Verification:**
Set `ProviderVersion` to `">= 1.0.0"` and verify the generated `required_providers` block includes that constraint.

---

### 16. Testing Infrastructure

**CDKTN-043:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL include Go unit tests (using `go test`) that verify synthesized Terraform JSON output matches expected golden files for each L2 and L3 construct.

**Rationale:**
Golden file tests catch unintended changes to generated output. The TDD workflow requires tests before implementation. Go's `testdata/` convention provides a standard location for golden files.

**Verification:**
Run `go test ./...` and verify golden file comparisons pass for all construct types.

---

**CDKTN-044:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL include integration tests that run `terraform validate` against synthesized output using a locally-built provider binary.

**Rationale:**
Golden file tests verify structure but not Terraform schema compliance. `terraform validate` confirms the output is parseable and matches the provider's resource schemas.

**Verification:**
Run integration tests and verify `terraform validate` exits 0 for all test fixtures.

---

**CDKTN-045:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL include end-to-end tests using `testcontainers-go` that deploy a minimal sharded cluster (1 config server RS of 3 members, 1 shard RS of 3 members, 1 mongos) and verify user/role creation via the provider.

**Rationale:**
End-to-end tests against real MongoDB instances verify the full pipeline from construct definition through synthesis through `terraform apply`.

**Verification:**
Run E2E tests with `go test -tags=e2e ./...` and verify users and roles are queryable on the containerized cluster after apply.

---

### 17. Build and Distribution

**CDKTN-046:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL be buildable via `make build` and testable via `make test` from the repository root Makefile.

**Rationale:**
Consistent with the project's Makefile-driven build convention per coding guidelines.

**Verification:**
Run `make build` and `make test` and verify both complete successfully.

---

**CDKTN-047:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL be published as a Go module consumable via `go get`.

**Rationale:**
The implementation language is Go. Go modules are the standard distribution mechanism for Go libraries. This integrates naturally with the existing `terraform-provider-mongodb` Go module.

**Verification:**
Run `go get <module-path>` from an external project and verify the construct types are importable and usable.

---

**CDKTN-048:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator Go module SHALL declare `github.com/open-constructs/cdk-terrain-go/cdktn` and `github.com/aws/constructs-go/constructs/v10` as dependencies in its `go.mod` file.

**Rationale:**
These are the core CDKTN framework packages. The construct library cannot function without them. Declaring them explicitly ensures `go mod tidy` resolves them correctly.

**Verification:**
Inspect `go.mod` and verify both dependencies are listed in the `require` block.

---

### 18. L1 Provider Bindings

**CDKTN-049:** Ubiquitous

**Requirement:**
The CDKTN Sharded Cluster Generator SHALL generate Go L1 bindings for `terraform-provider-mongodb` via `cdktn get` and include them as a vendored package within the module.

**Rationale:**
L1 bindings provide the typed Go structs for `mongodb_db_user`, `mongodb_db_role`, `mongodb_shard_config`, and the provider configuration. Vendoring them avoids requiring consumers to run `cdktn get` themselves.

**Verification:**
Verify the generated bindings directory exists, contains Go source files for all three resource types and the provider, and compiles without errors.

---

**CDKTN-050:** Ubiquitous

**Requirement:**
The L2 constructs SHALL compose L1 binding types (generated provider, resource, and data source structs) rather than emitting raw Terraform JSON directly.

**Rationale:**
Using L1 bindings ensures the generated output matches the provider schema exactly. Direct JSON emission bypasses CDKTN's synthesis pipeline and would miss dependency tracking, logical ID allocation, and override support.

**Verification:**
Inspect L2 construct source code and verify all Terraform resources are created via L1 constructor functions (e.g., `mongodb.NewDbUser()`), not via raw JSON or `addOverride`.

---

### 19. Per-Member Shard Configuration Overrides

**CDKTN-051:** Optional Feature

**Requirement:**
WHERE `Members` overrides are specified in `ShardConfigSettings`, the CDKTN Sharded Cluster Generator SHALL emit a `member` block list in the `mongodb_shard_config` resource containing each override's `host`, `priority`, `votes`, `hidden`, `arbiter_only`, `build_indexes`, `secondary_delay_secs`, and `tags` (when non-nil). WHEN `Members` is empty or nil, the `member` key SHALL be omitted entirely.

**Rationale:**
The provider's `mongodb_shard_config` resource supports per-member configuration via the `member` block (SHARD-001 through SHARD-008). The CDKTN generator MUST be able to produce these blocks for operators who need to set member-level priorities, votes, hidden mode, or tags.

**Verification:**
Synthesize a shard config with member overrides and verify the `member` list appears with correct field mappings. Synthesize without overrides and verify no `member` key is present.

---

### 20. Original User Bootstrap Resource

**CDKTN-052:** Optional Feature

**Requirement:**
WHERE `OriginalUsers` are specified on an L2 construct's props or at the cluster level, the CDKTN Sharded Cluster Generator SHALL generate a `mongodb_original_user` resource for each entry with inline connection parameters (`host`, `port`, `username`, `password`, `auth_database`) and no provider alias reference. WHERE `SSL` is configured, SSL fields SHALL be emitted. WHERE `ReplicaSet` is specified, the `replica_set` field SHALL be emitted. Cluster-level `OriginalUsers` SHALL cascade to the first alias of each component (mongos, config server, and each shard).

**Rationale:**
The `mongodb_original_user` resource (ORIG-001) bootstraps an admin user on a no-auth MongoDB instance. It carries its own connection parameters independent of the provider config. The CDKTN generator MUST support generating these resources for initial cluster bootstrap workflows.

**Verification:**
Synthesize with original users at L2 level and verify resources have inline connection params and no provider ref. Synthesize with cluster-level original users and verify they cascade to all component first aliases.

---

## Traceability Matrix

| Req ID | Category | EARS Pattern | Priority |
|--------|----------|-------------|----------|
| CDKTN-001 | Construct Hierarchy | Ubiquitous | Must |
| CDKTN-002 | Construct Hierarchy | Ubiquitous | Must |
| CDKTN-003 | Construct Hierarchy | Ubiquitous | Must |
| CDKTN-004 | Provider Alias | Event Driven | Must |
| CDKTN-005 | Provider Alias | Ubiquitous | Must |
| CDKTN-006 | Provider Alias | Ubiquitous | Must |
| CDKTN-007 | Credentials | Ubiquitous | Must |
| CDKTN-008 | Credentials | Optional Feature | Should |
| CDKTN-009 | Credentials | Ubiquitous | Must |
| CDKTN-010 | Credentials | Optional Feature | Should |
| CDKTN-011 | User/Role Mgmt | Event Driven | Must |
| CDKTN-012 | User/Role Mgmt | Event Driven | Must |
| CDKTN-013 | User/Role Mgmt | Ubiquitous | Must |
| CDKTN-014 | User/Role Mgmt | Ubiquitous | Must |
| CDKTN-015 | Shard Config | Event Driven | Must |
| CDKTN-016 | Shard Config | Ubiquitous | Must |
| CDKTN-017 | Shard Config | Ubiquitous | Must |
| CDKTN-018 | SSL/TLS | Optional Feature | Should |
| CDKTN-019 | SSL/TLS | Optional Feature | Should |
| CDKTN-020 | SSL/TLS | Optional Feature | Could |
| CDKTN-021 | Validation | Unwanted Behaviour | Must |
| CDKTN-022 | Validation | Unwanted Behaviour | Must |
| CDKTN-023 | Validation | Unwanted Behaviour | Must |
| CDKTN-024 | Validation | Unwanted Behaviour | Must |
| CDKTN-025 | Validation | Unwanted Behaviour | Must |
| CDKTN-026 | Validation | Unwanted Behaviour | Must |
| CDKTN-027 | Validation | Unwanted Behaviour | Should |
| CDKTN-028 | Validation | Unwanted Behaviour | Must |
| CDKTN-029 | Synthesis | Ubiquitous | Must |
| CDKTN-030 | Synthesis | Ubiquitous | Must |
| CDKTN-031 | Synthesis | Ubiquitous | Must |
| CDKTN-032 | Config Interface | Ubiquitous | Must |
| CDKTN-033 | Config Interface | Ubiquitous | Must |
| CDKTN-034 | Proxy | Optional Feature | Could |
| CDKTN-035 | Direct Connection | Ubiquitous | Must |
| CDKTN-036 | Direct Connection | Ubiquitous | Must |
| CDKTN-037 | User Propagation | Event Driven | Must |
| CDKTN-038 | User Propagation | Event Driven | Must |
| CDKTN-039 | Retry Writes | Ubiquitous | Must |
| CDKTN-040 | Auth Database | Ubiquitous | Must |
| CDKTN-041 | Auth Database | Optional Feature | Could |
| CDKTN-042 | Provider Version | Ubiquitous | Must |
| CDKTN-043 | Testing | Ubiquitous | Must |
| CDKTN-044 | Testing | Ubiquitous | Must |
| CDKTN-045 | Testing | Ubiquitous | Should |
| CDKTN-046 | Build/Dist | Ubiquitous | Must |
| CDKTN-047 | Build/Dist | Ubiquitous | Must |
| CDKTN-048 | Build/Dist | Ubiquitous | Must |
| CDKTN-049 | L1 Bindings | Ubiquitous | Must |
| CDKTN-050 | L1 Bindings | Ubiquitous | Must |
| CDKTN-051 | Member Overrides | Optional Feature | Should |
| CDKTN-052 | Original User | Optional Feature | Should |

## Design Considerations

### Framework Choice: CDK Terrain (CDKTN)

CDK Terrain is the community-maintained fork of HashiCorp's CDKTF, maintained by the Open Constructs Foundation. First release v0.22.0 shipped February 2026. Key facts:

- **Go SDK**: `github.com/open-constructs/cdk-terrain-go/cdktn`
- **CLI**: `cdktn` / `cdktn-cli` (replaces `cdktf` CLI)
- **Config file**: Still `cdktf.json` (backward compatible)
- **Output dir**: Still `cdktf.out/` (backward compatible)
- **Construct base**: `github.com/aws/constructs-go/constructs/v10` (same as AWS CDK)
- **API surface**: Identical to CDKTF v0.21 — `App`, `TerraformStack`, `TerraformResource`, `TerraformProvider`, `Testing` utilities all preserved
- **Website**: [cdktn.io](https://cdktn.io/)

The migration from CDKTF to CDKTN is a package rename only (`cdktf` -> `cdktn`). All construct patterns, synthesis, and testing APIs are identical.

### Go Module Structure

The construct library should live as a sub-module or sub-package of the existing `terraform-provider-mongodb` repository:

```
terraform-provider-mongodb/
  go.mod                          # existing provider module
  mongodb/                        # existing provider code
  cdktn/                          # new construct library package
    go.mod                        # separate Go module for the construct lib
    go.sum
    cdktf.json                    # CDKTN provider config for L1 generation
    generated/                    # L1 bindings (vendored, generated via cdktn get)
      mongodb/
        provider.go
        db_user.go
        db_role.go
        shard_config.go
    cluster.go                    # MongoShardedCluster L3
    mongos.go                     # MongoMongos L2
    config_server.go              # MongoConfigServer L2
    shard.go                      # MongoShard L2
    credentials.go                # CredentialSource interface + impls
    types.go                      # MemberConfig, UserConfig, SSLConfig, etc.
    validation.go                 # Validation logic
    cluster_test.go               # Unit tests
    mongos_test.go
    config_server_test.go
    shard_test.go
    validation_test.go
    integration_test.go           # terraform validate tests
    e2e_test.go                   # testcontainers-go E2E tests
    testdata/                     # Golden files
      minimal_cluster.json
      full_cluster.json
      ...
```

### Go Map Ordering and Determinism (CDKTN-029)

Go maps have non-deterministic iteration order. All internal maps used during synthesis (e.g., provider alias maps, resource maps) MUST use sorted key iteration or `sort.Strings()` on keys before emitting. The CDKTN framework handles this for its own deep-merge, but any custom map iteration in L2/L3 constructs must also be deterministic.

### Error Handling Pattern

Go constructors should return `(*Construct, error)` rather than panicking. This differs from the CDKTN/CDK convention of panicking on invalid input, but is idiomatic Go. Callers can choose to `log.Fatal(err)` if they want panic-like behavior.

### Existing Provider Constants

The construct library should import and reuse constants from the provider package where applicable:
- `MaxMembers = 50` (`replica_set_types.go:35`)
- `MaxVotingMembers = 7` (`replica_set_types.go:34`)
- `MinVotingMembers = 1` (`replica_set_types.go:33`)
- `DefaultPriority = 2` (`replica_set_types.go:36`)
- `DefaultVotes = 1` (`replica_set_types.go:37`)

### Testing with CDKTN

CDKTN provides a `Testing` utility (Go: `cdktn.Testing_*` functions):
- `cdktn.Testing_App()` — creates a test app
- `cdktn.Testing_Synth(stack)` — synthesizes to JSON string
- `cdktn.Testing_ToHaveResource(json, type)` — asserts resource exists
- `cdktn.Testing_ToHaveResourceWithProperties(json, type, props)` — asserts resource with properties
- `cdktn.Testing_ToHaveProvider(json, type)` — asserts provider exists
- `cdktn.Testing_ToHaveProviderWithProperties(json, type, props)` — asserts provider with properties
- `cdktn.Testing_ToBeValidTerraform(dir)` — runs `terraform validate`

These integrate with Go's `testing.T` and should be used alongside `testify/assert` for ergonomic test assertions.
