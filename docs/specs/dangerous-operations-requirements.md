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
