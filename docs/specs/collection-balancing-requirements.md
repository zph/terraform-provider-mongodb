# Collection Balancing — EARS Requirements

**Prefix:** CBAL
**Resource:** `mongodb_collection_balancing`
**Status:** Draft

---

## Schema

- **CBAL-001:** WHEN the `mongodb_collection_balancing` resource schema is defined, THEN it SHALL include: `namespace` (Required, TypeString, immutable via CustomizeDiff — DANGER-014), `enabled` (Optional, TypeBool, Default true), `chunk_size_mb` (Optional, TypeInt).

## Namespace Validation

- **CBAL-002:** WHEN `namespace` does not contain exactly one dot separator, THEN the system SHALL reject the configuration with a validation error.

## Create / Update — MongoDB 6.0+

- **CBAL-003:** WHEN the featureCompatibilityVersion is 6.0 or higher, THEN the system SHALL use the `configureCollectionBalancing` admin command to set balancing and chunk size.

## Create / Update — MongoDB < 6.0

- **CBAL-004:** WHEN the featureCompatibilityVersion is below 6.0, THEN the system SHALL write the `noBalance` field to the `config.collections` document matching the namespace.

## Read

- **CBAL-005:** WHEN the resource is read, THEN the system SHALL use `config.collections` FindOne by `_id` equal to the namespace to read balancing state.
- **CBAL-006:** WHEN the `noBalance` field is `true`, THEN `enabled` SHALL be `false`; WHEN `noBalance` is absent or `false`, THEN `enabled` SHALL be `true`.

## Chunk Size Version Gate

- **CBAL-007:** WHEN `chunk_size_mb` is set and the featureCompatibilityVersion is below 6.0, THEN the system SHALL ignore the value and emit a warning diagnostic.

## Delete

- **CBAL-008:** WHEN the resource is deleted, THEN the system SHALL re-enable balancing for the collection (path depends on featureCompatibilityVersion).

## Identity

- **CBAL-009:** WHEN the resource is created, THEN the ID SHALL be `formatResourceId(namespace, "balancing")`.

## Connection

- **CBAL-010:** WHEN the resource is used, THEN it SHALL require a mongos connection; IF connected to a non-mongos topology, THEN the system SHALL return an error.

## Maturity

- **CBAL-011:** WHEN the resource is registered, THEN it SHALL be classified as `ResourceExperimental`.

## Identity Field Immutability

- **CBAL-012:** WHEN the `namespace` field changes on an existing resource, THEN the `CustomizeDiff` SHALL return an error blocking the change at plan time (DANGER-014). The field does not use `ForceNew`.
