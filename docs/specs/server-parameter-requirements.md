# Server Parameter Configuration — EARS Requirements

**Prefix:** PARAM
**Resource:** `mongodb_server_parameter`
**Status:** Draft

---

## Schema

- **PARAM-001:** WHEN the `mongodb_server_parameter` resource schema is defined, THEN it SHALL include: `parameter` (Required, ForceNew, TypeString), `value` (Required, TypeString), `ignore_read` (Optional, TypeBool, Default false).

## Create

- **PARAM-002:** WHEN a `mongodb_server_parameter` resource is created, THEN the system SHALL run `{setParameter: 1, <parameter>: <coerced_value>}` on the admin database.

## Read (Normal)

- **PARAM-003:** WHEN a `mongodb_server_parameter` resource is read with `ignore_read` set to false, THEN the system SHALL run `{getParameter: 1, <parameter>: 1}` and store `fmt.Sprintf("%v", val)` as the value.

## Read (Ignore)

- **PARAM-004:** WHEN a `mongodb_server_parameter` resource is read with `ignore_read` set to true, THEN the system SHALL retain the configured value without making a MongoDB call.

## Update

- **PARAM-005:** WHEN a `mongodb_server_parameter` resource is updated, THEN the system SHALL run `setParameter` with the new coerced value.

## Delete

- **PARAM-006:** WHEN a `mongodb_server_parameter` resource is deleted, THEN the system SHALL perform a no-op (server parameters cannot be unset) and remove the resource from state.

## Identity

- **PARAM-007:** WHEN a `mongodb_server_parameter` resource is created, THEN the ID SHALL be `formatResourceId("admin", parameter)`.

## Value Coercion

- **PARAM-008:** WHEN a value is sent to MongoDB, THEN the system SHALL coerce the string value in this order: boolean ("true"/"false") → integer → float → string (kept as-is).

## Error Handling

- **PARAM-009:** WHEN a `setParameter` command fails, THEN the error message SHALL include the parameter name.
- **PARAM-010:** WHEN a `getParameter` command fails (and `ignore_read` is false), THEN the error message SHALL include the parameter name.

## Maturity

- **PARAM-011:** WHEN the resource is registered, THEN it SHALL be classified as `ResourceExperimental`.

## Read-Back

- **PARAM-012:** WHEN reading a parameter value from MongoDB, THEN the system SHALL convert the value to a string via `fmt.Sprintf("%v", val)`.
