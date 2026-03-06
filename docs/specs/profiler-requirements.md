# Profiler Configuration — EARS Requirements

**Prefix:** PROF
**Resource:** `mongodb_profiler`
**Status:** Draft

---

## Schema

- **PROF-001:** WHEN the `mongodb_profiler` resource schema is defined, THEN it SHALL include: `database` (Required, ForceNew, TypeString), `level` (Required, TypeInt), `slowms` (Optional, TypeInt, Default 100), `ratelimit` (Optional, TypeInt, Default 1).

## Validation

- **PROF-002:** WHEN `level` is set to a value outside `[0, 2]`, THEN the system SHALL reject the configuration with a validation error.
- **PROF-003:** WHEN `slowms` is set to a value less than 0, THEN the system SHALL reject the configuration with a validation error.

## Identity

- **PROF-004:** WHEN a `mongodb_profiler` resource is created, THEN the ID SHALL be `formatResourceId(database, "profiler")`.

## Create

- **PROF-005:** WHEN a `mongodb_profiler` resource is created, THEN the system SHALL run `{profile: <level>, slowms: <slowms>, ratelimit: <ratelimit>}` on the target database.

## Read

- **PROF-006:** WHEN a `mongodb_profiler` resource is read, THEN the system SHALL run `{profile: -1}` on the target database and read `was` (as level), `slowms`, and `ratelimit` from the response.

## Update

- **PROF-007:** WHEN a `mongodb_profiler` resource is updated, THEN the system SHALL run the profile command with the new values (idempotent with Create).

## Delete

- **PROF-008:** WHEN a `mongodb_profiler` resource is deleted, THEN the system SHALL run `{profile: 0}` on the target database to disable profiling.

## Error Handling

- **PROF-009:** WHEN a profile command fails, THEN the error message SHALL include the command name and the MongoDB error message.

## Force Replacement

- **PROF-010:** WHEN the `database` field changes, THEN the system SHALL force replacement of the resource.

## Maturity

- **PROF-011:** WHEN the resource is registered, THEN it SHALL be classified as `ResourceExperimental`.
