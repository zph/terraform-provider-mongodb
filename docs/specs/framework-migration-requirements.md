# Framework Migration Requirements — EARS Specification

**Prefix:** FWMIG
**Status:** Planned
**Last Updated:** 2026-03-03

---

## Overview

This specification defines requirements for migrating the MongoDB Terraform provider from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The migration enables write-only attributes (passwords never stored in state), ephemeral resources, and improved type safety.

### Current State

- 14 files import `terraform-plugin-sdk/v2`
- 5 resources, 0 data sources
- 22 CRUD functions using context-based patterns
- Entrypoint uses `plugin.Serve` (SDKv2 legacy)
- No existing `terraform-plugin-framework` code

### Migration Strategy

Use `terraform-plugin-mux` to run both SDKv2 and framework providers simultaneously. Migrate one resource at a time behind the mux, validate with existing integration tests, then remove the SDKv2 resource. This avoids a big-bang rewrite and allows incremental validation.

---

## Phase 0: Mux Entrypoint

FWMIG-001: WHEN the provider starts, it SHALL use `terraform-plugin-mux` to serve both an SDKv2 provider and a framework provider simultaneously.

> Rationale: The mux allows incremental migration. Resources can be moved one at a time from SDKv2 to framework without breaking existing users. The mux routes requests to whichever provider implements the resource.

FWMIG-002: WHEN the mux entrypoint is configured, the `main.go` SHALL replace `plugin.Serve` with `tf6server.Serve` wrapping a `tf5to6server` adapter for the SDKv2 provider and the native framework provider.

> Rationale: The framework uses protocol v6. The SDKv2 provider must be wrapped in a v5-to-v6 adapter. Both are combined via `tf6muxserver.NewMuxServer`.

FWMIG-003: WHEN a resource exists in both providers during migration, the framework provider SHALL take precedence.

> Rationale: The framework implementation is the target. If both register the same resource type, the mux routes to the first provider in the list. Place the framework provider first.

---

## Phase 1: Provider Schema

FWMIG-004: WHEN the framework provider is initialized, it SHALL define the same provider schema as the SDKv2 provider: `host`, `port`, `username`, `password`, `auth_database`, `certificate`, `ssl`, `insecure_skip_verify`, `replica_set`, `direct`, `retrywrites`, `proxy`.

FWMIG-005: WHEN the framework provider schema defines `password`, the field SHALL use the `Sensitive` attribute AND the `WriteOnly` modifier (Terraform 1.11+) so the value is never persisted to state.

> Rationale: WriteOnly attributes are sent to the provider during configure/apply but are never stored in state. This is the primary motivation for the migration.

FWMIG-006: WHEN the framework provider schema defines `certificate`, the field SHALL use both `Sensitive` and `WriteOnly` modifiers.

FWMIG-007: WHEN the framework provider reads configuration, it SHALL use `provider.ConfigureRequest` and set the configured MongoDB client on `provider.ConfigureResponse` for resource access via `req.ProviderData`.

---

## Phase 2: Resource Migration — mongodb_db_role

FWMIG-008: WHEN `mongodb_db_role` is migrated, the framework resource SHALL implement `resource.Resource` with `Metadata`, `Schema`, `Create`, `Read`, `Update`, `Delete`, and `ImportState` methods.

FWMIG-009: WHEN the framework `mongodb_db_role` schema is defined, it SHALL use typed attributes (`types.String`, `types.Bool`, `types.List`) instead of `schema.TypeString`, `schema.TypeBool`, etc.

FWMIG-010: WHEN the framework `mongodb_db_role` is registered, the SDKv2 registration in `resource_registry.go` SHALL be removed for that resource.

> Rationale: `mongodb_db_role` is the simplest stable resource (no sensitive fields, no complex lifecycle). Migrate it first to establish patterns.

---

## Phase 3: Resource Migration — mongodb_db_user

FWMIG-011: WHEN `mongodb_db_user` is migrated, the `password` attribute SHALL use the `WriteOnly` schema modifier so the password value is never stored in Terraform state.

FWMIG-012: WHEN Terraform reads `mongodb_db_user` state, the `password` field SHALL be absent from state (not empty string, not redacted — absent).

> Rationale: This is the primary security improvement. Currently passwords are stored in plain text in state. WriteOnly attributes are only sent during apply and never persisted.

FWMIG-013: WHEN `mongodb_db_user` is updated and the password has not changed, the provider SHALL skip the drop+recreate cycle.

> Rationale: Without state, the provider cannot detect password changes via diff. The `password` attribute should use `RequiresReplace` or the resource should accept a `password_version` trigger attribute that the user bumps when they want to rotate.

---

## Phase 4: Resource Migration — mongodb_original_user

FWMIG-014: WHEN `mongodb_original_user` is migrated, the `password` attribute SHALL use `WriteOnly`.

FWMIG-015: WHEN `mongodb_original_user` is migrated, the `certificate` attribute SHALL use `WriteOnly`.

FWMIG-016: WHEN `mongodb_original_user` is migrated, the resource SHALL retain its independent connection parameters (not using the provider client).

---

## Phase 5: Resource Migration — mongodb_shard_config

FWMIG-017: WHEN `mongodb_shard_config` is migrated, the `member` block SHALL use `schema.ListNestedBlock` with typed member attributes.

FWMIG-018: WHEN `mongodb_shard_config` is migrated, the `priority` attribute SHALL be `types.Float64` (matching the current `TypeFloat` schema).

FWMIG-019: WHEN `mongodb_shard_config` is migrated, the experimental gating mechanism SHALL use the framework's `ConfigValidators` or a custom `ValidateConfig` method to check `TERRAFORM_PROVIDER_MONGODB_ENABLE`.

> Rationale: The SDKv2 registry pattern (`BuildResourceMap`) does not exist in the framework. Gating moves to validation-time rather than registration-time.

---

## Phase 6: Resource Migration — mongodb_shard

FWMIG-020: WHEN `mongodb_shard` is migrated, the `hosts` attribute SHALL use `types.List` of `types.String`.

FWMIG-021: WHEN `mongodb_shard` is migrated, the `ForceNew` behavior SHALL use `resource.RequiresReplace()` plan modifiers on `shard_name` and `hosts`.

FWMIG-022: WHEN `mongodb_shard` is migrated, the `remove_timeout_secs` SHALL be implemented as a configurable timeout via the framework's `resource.ResourceWithConfigureTimeout` interface or a schema default.

---

## Phase 7: SDKv2 Removal

FWMIG-023: WHEN all 5 resources are migrated, the `terraform-plugin-sdk/v2` dependency SHALL be removed from `go.mod`.

FWMIG-024: WHEN SDKv2 is removed, the mux entrypoint SHALL be replaced with a direct `tf6server.Serve` call for the framework provider only.

FWMIG-025: WHEN SDKv2 is removed, `resource_registry.go` SHALL be refactored to return framework `resource.Resource` implementations instead of `*schema.Resource` factories.

---

## Phase 8: WriteOnly Validation

FWMIG-026: WHEN the migration is complete, an integration test SHALL verify that `terraform show -json` output for `mongodb_db_user` does NOT contain the password value.

FWMIG-027: WHEN the migration is complete, an integration test SHALL verify that `terraform state show` for `mongodb_db_user` does NOT contain the password value.

FWMIG-028: WHEN the migration is complete, a golden test SHALL verify that `createUser` commands still contain the correct password (the value reaches MongoDB even though it is not in state).

---

## Dependencies

| Dependency | Minimum Version | Purpose |
|---|---|---|
| `terraform-plugin-framework` | v1.5+ | Framework provider, resource, schema types |
| `terraform-plugin-mux` | v0.16+ | SDKv2 + framework co-serving during migration |
| `terraform-plugin-go` | v0.23+ | Protocol types (already a transitive dep) |
| `terraform-plugin-testing` | v1.6+ | Acceptance testing for framework resources |
| Terraform CLI | >= 1.11 | WriteOnly attribute support |

---

## Migration Order

| Phase | Resource | Rationale |
|---|---|---|
| 0 | (entrypoint) | Mux setup, no resource changes |
| 1 | (provider schema) | Framework provider with WriteOnly password/cert |
| 2 | `mongodb_db_role` | Simplest resource, no sensitive fields |
| 3 | `mongodb_db_user` | Primary security win (WriteOnly password) |
| 4 | `mongodb_original_user` | WriteOnly password + certificate |
| 5 | `mongodb_shard_config` | Most complex resource (nested blocks, init flow) |
| 6 | `mongodb_shard` | Simple but experimental |
| 7 | (cleanup) | Remove SDKv2, remove mux |
| 8 | (validation) | Prove passwords are absent from state |

---

## Known Risks

* **Terraform 1.11 requirement:** WriteOnly attributes require Terraform >= 1.11. Users on older versions will not benefit from the state exclusion. The provider should document this minimum version.
* **State migration:** Existing users will have passwords in their state from before the migration. A one-time `terraform state rm` + `terraform import` cycle may be needed, or the provider should handle missing password in state gracefully.
* **Password change detection:** Without the password in state, the provider cannot detect when the user changes it in config. A `password_version` or `password_sha256` attribute may be needed as a change trigger.
* **Testing complexity:** The mux introduces a second provider instance during migration. Integration tests must exercise both SDKv2 and framework resources to prevent regressions.
