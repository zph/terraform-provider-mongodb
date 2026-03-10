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

The `original_user` case is fundamentally broken (authenticates as the user it drops),
so it retains a resource-level `allow_dangerous_update` safety flag following the
same pattern as FCV's `danger_mode`.

---

## Requirements

DANGER-001: WHEN `mongodb_db_user` is updated, the resource SHALL use the MongoDB
`updateUser` command for in-place modification instead of `dropUser` + `createUser`.

DANGER-002: WHEN `mongodb_db_role` is updated, the resource SHALL use the MongoDB
`updateRole` command for in-place modification instead of `dropRole` + `createRole`.

DANGER-003: WHEN `mongodb_original_user` has any attribute change during plan AND
`allow_dangerous_update` is `false` (the default), the `CustomizeDiff` SHALL
return an error blocking the plan.

DANGER-004: WHEN `mongodb_original_user` has any attribute change during plan AND
`allow_dangerous_update` is `true`, the `CustomizeDiff` SHALL allow the plan to
proceed.

DANGER-005: The `mongodb_original_user` resource SHALL accept an optional boolean
attribute `allow_dangerous_update` that defaults to `false`.

DANGER-006: The `CustomizeDiff` for `mongodb_original_user` SHALL only block
updates on existing resources (resources with a non-empty ID), not on initial
creation.

DANGER-007: The FCV resource's existing `danger_mode` attribute and `CustomizeDiff`
SHALL remain unaffected by the dangerous operations changes.
