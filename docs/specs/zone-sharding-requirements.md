# Zone Sharding — EARS Requirements

**Prefix:** ZONE
**Resources:** `mongodb_shard_zone`, `mongodb_zone_key_range`
**Status:** Draft

---

## mongodb_shard_zone

### Schema

- **ZONE-001:** WHEN the `mongodb_shard_zone` resource schema is defined, THEN it SHALL include: `shard_name` (Required, TypeString, immutable via CustomizeDiff), `zone` (Required, TypeString, immutable via CustomizeDiff), `planned_commands` (Computed, TypeString).

### Create

- **ZONE-002:** WHEN Create is called, the resource SHALL run `addShardToZone` on the admin database with the configured `shard_name` and `zone` values.

- **ZONE-003:** WHEN `addShardToZone` succeeds, the resource SHALL set the ID to `shard_name:zone` and read back state.

### Read

- **ZONE-004:** WHEN Read is called, the resource SHALL query the `config.shards` collection for the document matching the shard name and check that the `tags` array contains the configured zone name.

- **ZONE-005:** IF the shard document is not found or the `tags` array does not contain the configured zone, THEN the resource SHALL clear the resource ID to remove it from state.

### Update

- **ZONE-006:** WHEN an update is attempted, the resource SHALL be a no-op because all fields are identity fields blocked by CustomizeDiff.

### Delete

- **ZONE-007:** WHEN Delete is called, the resource SHALL run `removeShardFromZone` on the admin database with the configured `shard_name` and `zone` values.

### Connection

- **ZONE-008:** WHEN the resource is used, THEN it SHALL require a mongos connection; IF connected to a non-mongos topology, THEN the system SHALL return an error.

### Identity

- **ZONE-009:** WHEN the resource is created, THEN the ID SHALL be formatted as `shard_name:zone` (colon-separated).

### Identity Field Immutability

- **ZONE-010:** WHEN the `shard_name` field changes on an existing resource, THEN the `CustomizeDiff` SHALL return an error blocking the change at plan time.

- **ZONE-011:** WHEN the `zone` field changes on an existing resource, THEN the `CustomizeDiff` SHALL return an error blocking the change at plan time.

### Maturity

- **ZONE-012:** WHEN the resource is registered, THEN it SHALL be classified as `ResourceExperimental`.

### Command Preview

- **ZONE-013:** WHEN command preview is enabled and the resource is being created, THEN `planned_commands` SHALL show the `addShardToZone` command with shard name and zone name.

---

## mongodb_zone_key_range

### Schema

- **ZONE-014:** WHEN the `mongodb_zone_key_range` resource schema is defined, THEN it SHALL include: `namespace` (Required, TypeString, immutable via CustomizeDiff), `zone` (Required, TypeString, immutable via CustomizeDiff), `min` (Required, TypeString, immutable via CustomizeDiff — JSON document), `max` (Required, TypeString, immutable via CustomizeDiff — JSON document), `planned_commands` (Computed, TypeString).

### Namespace Validation

- **ZONE-015:** WHEN `namespace` does not contain exactly one dot separator in `db.collection` format, THEN the system SHALL reject the configuration with a validation error.

### Min/Max Validation

- **ZONE-016:** WHEN `min` or `max` is not valid JSON, THEN the system SHALL reject the configuration with a validation error.

### Create

- **ZONE-017:** WHEN Create is called, the resource SHALL parse the `min` and `max` fields from JSON strings to BSON documents and run `updateZoneKeyRange` on the admin database with the configured `namespace`, `min`, `max`, and `zone` values.

- **ZONE-018:** WHEN `updateZoneKeyRange` succeeds, the resource SHALL set the ID and read back state.

### Read

- **ZONE-019:** WHEN Read is called, the resource SHALL query the `config.tags` collection for a document matching the `namespace` (`_id.ns` or `ns` field) with matching `min` and `max` bounds, and verify the `tag` field equals the configured zone.

- **ZONE-020:** IF no matching document is found in `config.tags`, THEN the resource SHALL clear the resource ID to remove it from state.

### Update

- **ZONE-021:** WHEN an update is attempted, the resource SHALL be a no-op because all fields are identity fields blocked by CustomizeDiff.

### Delete

- **ZONE-022:** WHEN Delete is called, the resource SHALL parse the `min` and `max` fields from JSON strings to BSON documents and run `updateZoneKeyRange` on the admin database with the configured `namespace`, `min`, `max`, and `zone` set to `null`.

### Connection

- **ZONE-023:** WHEN the resource is used, THEN it SHALL require a mongos connection; IF connected to a non-mongos topology, THEN the system SHALL return an error.

### Identity

- **ZONE-024:** WHEN the resource is created, THEN the ID SHALL encode the `namespace`, `min`, and `max` fields using base64-encoded JSON separated by `::` delimiters (format: `namespace::base64(min)::base64(max)`).

### Identity Field Immutability

- **ZONE-025:** WHEN the `namespace` field changes on an existing resource, THEN the `CustomizeDiff` SHALL return an error blocking the change at plan time.

- **ZONE-026:** WHEN the `zone` field changes on an existing resource, THEN the `CustomizeDiff` SHALL return an error blocking the change at plan time.

- **ZONE-027:** WHEN the `min` field changes on an existing resource, THEN the `CustomizeDiff` SHALL return an error blocking the change at plan time.

- **ZONE-028:** WHEN the `max` field changes on an existing resource, THEN the `CustomizeDiff` SHALL return an error blocking the change at plan time.

### Maturity

- **ZONE-029:** WHEN the resource is registered, THEN it SHALL be classified as `ResourceExperimental`.

### Command Preview

- **ZONE-030:** WHEN command preview is enabled and the resource is being created, THEN `planned_commands` SHALL show the `updateZoneKeyRange` command with namespace, min, max, and zone.
