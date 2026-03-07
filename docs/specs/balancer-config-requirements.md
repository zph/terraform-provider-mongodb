# Balancer Configuration — EARS Requirements

**Prefix:** BAL
**Resource:** `mongodb_balancer_config`
**Status:** Draft

---

## Schema

- **BAL-001:** WHEN the `mongodb_balancer_config` resource schema is defined, THEN it SHALL include: `enabled` (Optional, TypeBool, Default true), `active_window_start` (Optional, TypeString), `active_window_stop` (Optional, TypeString), `chunk_size_mb` (Optional, TypeInt), `secondary_throttle` (Optional, TypeString), `wait_for_delete` (Optional, TypeBool).

## Defaults

- **BAL-002:** WHEN `enabled` is not specified, THEN it SHALL default to `true`.

## Create / Update — Enabled

- **BAL-003:** WHEN `enabled` is `true`, THEN the system SHALL run `balancerStart` on the admin database; WHEN `enabled` is `false`, THEN the system SHALL run `balancerStop` on the admin database.

## Create / Update — Active Window

- **BAL-004:** WHEN both `active_window_start` and `active_window_stop` are set, THEN the system SHALL upsert `config.settings` document `_id:"balancer"` with `activeWindow.start` and `activeWindow.stop`.

## Create / Update — Chunk Size

- **BAL-005:** WHEN `chunk_size_mb` is set, THEN the system SHALL upsert `config.settings` document `_id:"chunksize"` with `value` equal to the configured chunk size in megabytes.

## Create / Update — Secondary Throttle

- **BAL-006:** WHEN `secondary_throttle` is set, THEN the system SHALL write `_secondaryThrottle` to `config.settings` document `_id:"balancer"`.

## Create / Update — Wait For Delete

- **BAL-007:** WHEN `wait_for_delete` is set, THEN the system SHALL write `_waitForDelete` to `config.settings` document `_id:"balancer"`.

## Read

- **BAL-008:** WHEN the resource is read, THEN the system SHALL use `balancerStatus` to read the `enabled` state (mode "full" = true, otherwise false), and SHALL use `config.settings` FindOne to read `active_window_start`, `active_window_stop`, `chunk_size_mb`, `secondary_throttle`, and `wait_for_delete`.

## Delete

- **BAL-009:** WHEN the resource is deleted, THEN the system SHALL re-enable the balancer via `balancerStart`, `$unset` the `activeWindow`, `_secondaryThrottle`, and `_waitForDelete` fields from the balancer settings document, and delete the `_id:"chunksize"` document from `config.settings`.

## Validation — Active Window

- **BAL-010:** WHEN only one of `active_window_start` or `active_window_stop` is set, THEN the system SHALL reject the configuration with a validation error.
- **BAL-011:** WHEN `active_window_start` or `active_window_stop` is set, THEN the value SHALL be validated as `HH:MM` format (24-hour clock).

## Connection

- **BAL-012:** WHEN the resource is used, THEN it SHALL require a mongos connection; IF connected to a non-mongos topology, THEN the system SHALL return an error.

## Identity

- **BAL-013:** WHEN the resource is created, THEN the ID SHALL be the fixed string `"balancer"`.

## Maturity

- **BAL-014:** WHEN the resource is registered, THEN it SHALL be classified as `ResourceExperimental`.

## Validation — Chunk Size

- **BAL-015:** WHEN `chunk_size_mb` is set to a value less than 1 or greater than 1024, THEN the system SHALL reject the configuration with a validation error.
