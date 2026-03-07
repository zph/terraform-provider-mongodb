# CatchUp Timeout Configuration — EARS Requirements

**Prefix:** CATCHUP
**Resource:** `mongodb_shard_config`
**Status:** Draft

---

## Schema

- **CATCHUP-001:** WHEN the `mongodb_shard_config` resource schema is defined, THEN it SHALL include an Optional `catch_up_timeout_millis` field of TypeInt with a Default of `-1`.

## Create

- **CATCHUP-002:** WHEN a `mongodb_shard_config` resource is created with `catch_up_timeout_millis` set, THEN the system SHALL include the value in the `replSetReconfig` settings document as `CatchUpTimeoutMillis`.

## Update

- **CATCHUP-003:** WHEN a `mongodb_shard_config` resource is updated, THEN the system SHALL write the `catch_up_timeout_millis` value to the config Settings before calling `replSetReconfig`.

## Read

- **CATCHUP-004:** WHEN a `mongodb_shard_config` resource is read, THEN the system SHALL populate state from `config.Settings.CatchUpTimeoutMillis`.

## Model

- **CATCHUP-005:** WHEN the `SettingsModel` struct is defined, THEN it SHALL include a `CatchUpTimeoutMillis int64` field.
