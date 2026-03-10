# Dangerous Operations Safety — EARS Requirements

**Spec ID prefix:** DANGER
**Created:** 2026-03-09

---

## Context

The audit identified three drop+recreate anti-patterns in Update functions:
- `mongodb_db_user` — `dropUser` + `createUser` (user lost if create fails)
- `mongodb_db_role` — `dropRole` + `createRole` (role lost if create fails)
- `mongodb_original_user` — drops the admin user it authenticated as (cluster lockout)

The root cause for `db_user` and `db_role` is fixed by replacing `drop+create` with
MongoDB's `updateUser` and `updateRole` commands for in-place modification.

The `original_user` case is fundamentally broken (authenticates as the user it drops).
Both Update and Delete unconditionally refuse to drop the original admin user. Delete
removes the resource from Terraform state only; Update always errors.

---

## Requirements

DANGER-001: WHEN `mongodb_db_user` is updated, the resource SHALL use the MongoDB
`updateUser` command for in-place modification instead of `dropUser` + `createUser`.

DANGER-002: WHEN `mongodb_db_role` is updated, the resource SHALL use the MongoDB
`updateRole` command for in-place modification instead of `dropRole` + `createRole`.

DANGER-003: WHEN `mongodb_original_user` has any attribute change during plan,
the `CustomizeDiff` SHALL always return an error. Updates are unconditionally
refused because the Update function would authenticate as the user it drops.

DANGER-004: WHEN `mongodb_original_user` Update is called (defense-in-depth past
the `CustomizeDiff` gate), it SHALL return an error without performing any
MongoDB operations.

DANGER-005: The `CustomizeDiff` for `mongodb_original_user` SHALL only block
updates on existing resources (resources with a non-empty ID), not on initial
creation.

DANGER-006: WHEN `mongodb_original_user` is destroyed, the resource SHALL NOT
execute `dropUser` against MongoDB. It SHALL only remove the resource from
Terraform state.

DANGER-007: The FCV resource's existing `danger_mode` attribute and `CustomizeDiff`
SHALL remain unaffected by the dangerous operations changes.

DANGER-008: WHEN `mongodb_original_user` Delete runs, it SHALL emit a
`diag.Warning` informing the operator that the user was not dropped from MongoDB.

---

## ForceNew Ban

`ForceNew: true` in Terraform causes destroy+recreate on field changes. For a
database provider this risks data loss or cluster lockout. All resources SHALL
use `CustomizeDiff` to block identity field changes at plan time instead.

DANGER-009: WHEN `mongodb_original_user` has any attribute change during plan,
the `CustomizeDiff` SHALL block all updates at plan time. (Already tagged in
code as DANGER-009.)

DANGER-010: The provider SHALL NOT use `ForceNew: true` in any resource schema
field, except WHERE explicitly allowlisted. ForceNew causes Terraform to
destroy and recreate the resource, which for a database provider risks data
loss.

DANGER-011: A static analysis linter (`noforceenew`) SHALL detect any usage of
`ForceNew: true` in `schema.Schema` composite literals and report a diagnostic
error. The linter SHALL support a built-in allowlist of `filename:fieldname`
pairs that are permanently exempt.

DANGER-012: A runtime test SHALL walk all resources returned by `AllResources()`
and inspect every `schema.Schema` field for `ForceNew == true`. The test SHALL
fail for any ForceNew field not present in a hardcoded allowlist.

DANGER-013: WHEN `mongodb_server_parameter` has a change to the `parameter`
field on an existing resource, the `CustomizeDiff` SHALL return an error
blocking the change.

DANGER-014: WHEN `mongodb_collection_balancing` has a change to the `namespace`
field on an existing resource, the `CustomizeDiff` SHALL return an error
blocking the change.

DANGER-015: WHEN `mongodb_profiler` has a change to the `database` field on an
existing resource, the `CustomizeDiff` SHALL return an error blocking the
change.

DANGER-016: WHEN `mongodb_shard` has a change to the `hosts` field on an
existing resource, the `CustomizeDiff` SHALL return an error blocking the
change.

DANGER-017: WHEN `mongodb_shard` has a change to the `shard_name` field on an
existing resource, the `CustomizeDiff` SHALL return an error blocking the
change. The `shard_name` field retains `ForceNew: true` as an allowlisted
exception (defense-in-depth).

DANGER-018: The `ForceNew: true` allowlist SHALL contain exactly one entry:
`mongodb_shard.shard_name`.

DANGER-019: The `mongodb_shard` Update function SHALL handle changes to
`remove_timeout_secs` gracefully (read-through, no MongoDB operation required)
since it is a client-side-only value.

DANGER-020: The `noforceenew` linter SHALL be integrated into the pre-commit
hook pipeline and `make lint` target.
