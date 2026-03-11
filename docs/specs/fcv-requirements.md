# Feature Compatibility Version (FCV) Resource — EARS Requirements

**Resource:** `mongodb_feature_compatibility_version`
**Prefix:** FCV
**Status:** Draft
**Date:** 2026-03-06

---

## Schema

**FCV-001:** The resource schema SHALL define `version` as TypeString Required and `danger_mode` as TypeBool Optional with Default false.

**FCV-002:** WHEN a `version` value is provided, THEN it SHALL be validated against the pattern `^\d+\.\d+$`.

**FCV-003:** The resource ID SHALL be the fixed string `"fcv"` (singleton resource, one per cluster).

## Lifecycle

**FCV-004:** WHEN the resource is created, THEN the provider SHALL run the `setFeatureCompatibilityVersion` admin command with the specified `version`.

**FCV-005:** WHEN the resource is read, THEN the provider SHALL call `GetFCV()` and set the `version` attribute from the result.

**FCV-006:** WHEN the resource is updated, THEN the provider SHALL delegate to the Create function (idempotent).

**FCV-007:** WHEN the resource is deleted, THEN the provider SHALL perform a no-op and clear the Terraform ID (FCV always has a value and cannot be unset).

## Safety Gate

**FCV-008:** WHEN `version` changes on an existing resource AND `danger_mode` is `false`, THEN the plan SHALL be blocked with an error.

**FCV-009:** WHEN `version` changes on an existing resource AND `danger_mode` is `true`, THEN the plan SHALL proceed.

**FCV-010:** WHEN `version` changes during apply, THEN the provider SHALL emit a `diag.Warning` indicating the FCV change.

**FCV-011:** WHEN the new version is less than the old version during apply, THEN the provider SHALL emit an additional downgrade-specific warning.

## Helpers

**FCV-012:** `compareFCV` SHALL compare two FCV strings as `(major, minor)` integer pairs and return -1, 0, or +1.

## Classification

**FCV-013:** The resource SHALL be classified as `ResourceExperimental` in the resource registry.

## Error Handling

**FCV-014:** Error messages SHALL include the command name, target version, and MongoDB error details.
