# Command Preview — EARS Requirements

**Spec ID prefix:** PREVIEW
**Created:** 2026-03-10

---

## Context

Operators managing production MongoDB clusters need to see the exact commands
that `terraform apply` will execute before approving a plan. This feature adds
a `command_preview` toggle at the provider level that, when enabled, populates
a computed `planned_commands` attribute on each resource during `terraform plan`
with sanitized MongoDB shell-style commands.

No MongoDB connection is made during plan. Commands are computed deterministically
from the Terraform config diff.

---

## Provider Toggle

PREVIEW-001: WHEN the provider schema is defined, it SHALL include a
`command_preview` field (Optional, TypeBool, Default false).

PREVIEW-002: WHEN `command_preview` is not set or set to false, the provider
SHALL NOT populate `planned_commands` on any resource.

PREVIEW-003: WHEN `command_preview` is true, the provider SHALL set the
`CommandPreview` field on `MongoDatabaseConfiguration` to true.

PREVIEW-004: The `command_preview` field SHALL support the environment variable
`TERRAFORM_PROVIDER_MONGODB_COMMAND_PREVIEW` as a default value source.

## Planned Commands Attribute

PREVIEW-005: Each resource that supports command preview SHALL include a
`planned_commands` attribute (Computed, TypeString).

PREVIEW-006: WHEN a resource is being created (d.Id() == "") AND command
preview is enabled, the `CustomizeDiff` SHALL populate `planned_commands`
with the Create commands.

PREVIEW-007: WHEN an existing resource has changes AND command preview is
enabled, the `CustomizeDiff` SHALL populate `planned_commands` with the
Update commands.

PREVIEW-008: WHEN a resource has no changes, the `CustomizeDiff` SHALL NOT
modify `planned_commands`.

PREVIEW-009: WHEN a command preview includes password or credential values,
the preview SHALL replace them with `[REDACTED]`.

PREVIEW-010: WHEN the provider metadata is nil (provider not yet configured),
the `CustomizeDiff` SHALL skip command preview without error.

PREVIEW-011: WHEN Read is called for any resource, the `planned_commands`
attribute SHALL be set to "" (empty string) to avoid stale preview data
persisting in state.

## Per-Resource Preview Commands

PREVIEW-012: WHEN `mongodb_profiler` is previewed, the `planned_commands`
SHALL show:
`db.getSiblingDB("<database>").runCommand({profile: <level>, slowms: <N>, ratelimit: <N>})`

PREVIEW-013: WHEN `mongodb_server_parameter` is previewed, the
`planned_commands` SHALL show:
`db.adminCommand({setParameter: 1, <param>: <value>})`

PREVIEW-014: WHEN `mongodb_collection_balancing` is previewed, the
`planned_commands` SHALL show both the FCV >= 6.0 path
(`configureCollectionBalancing`) and the legacy path
(`config.collections` update), since FCV is unknown at plan time.

PREVIEW-015: WHEN `mongodb_shard` is previewed for Create, the
`planned_commands` SHALL show:
`db.adminCommand({addShard: "<rsName>/host1:port,host2:port"})`

PREVIEW-016: WHEN `mongodb_db_user` is previewed for Create, the
`planned_commands` SHALL show `createUser` with `pwd: [REDACTED]`.

PREVIEW-017: WHEN `mongodb_db_user` is previewed for Update, the
`planned_commands` SHALL show `updateUser` with `pwd: [REDACTED]` only when
the password attribute has changed; otherwise the preview SHALL omit `pwd`,
matching the command the apply sends (DANGER-021). The preview SHALL compute
this with the same decision function as Update (`includePasswordForUpdate`);
a password unknown at plan time SHALL render as a send.

PREVIEW-018: WHEN `mongodb_db_role` is previewed for Create, the
`planned_commands` SHALL show `createRole` with privileges and inherited roles.

PREVIEW-019: WHEN `mongodb_db_role` is previewed for Update, the
`planned_commands` SHALL show `updateRole` with privileges and inherited roles.

PREVIEW-020: WHEN `mongodb_feature_compatibility_version` is previewed, the
`planned_commands` SHALL show:
`db.adminCommand({setFeatureCompatibilityVersion: "<version>"})`

PREVIEW-021: WHEN `mongodb_balancer_config` is previewed, the
`planned_commands` SHALL list all commands that would execute
(balancerStart/Stop, config.settings writes, chunksize write).

PREVIEW-022: WHEN `mongodb_shard_config` is previewed for Create (new RS),
the `planned_commands` SHALL show `replSetInitiate`, then `replSetReconfig`
for the settings, then for each `member` block after the first one
`replSetReconfig` line adding the host as a non-voter (or, for an arbiter, in
final form) and, when the block gives it votes or a non-zero priority, a
second line applying those once the member is SECONDARY (SHARD-013,
SHARD-022).

PREVIEW-023: WHEN `mongodb_shard_config` is previewed for Update, the
`planned_commands` SHALL show `replSetReconfig` for the settings and the
matched members, then the same add and promotion lines as PREVIEW-022 for
each `member` block whose host is not in state, and one promotion line for
each block that gives a member in state more votes than it has (SHARD-023).
Whether an added host is already in the live set is only known at apply
time, so the add line says the member is added unless it is already one.

PREVIEW-024: WHEN `mongodb_original_user` is previewed for Create, the
`planned_commands` SHALL show `createUser` with `pwd: [REDACTED]`.
