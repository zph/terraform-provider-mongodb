# Oplog Configuration Requirements

This document defines EARS requirements for oplog size configuration via the
`mongodb_shard_config` Terraform resource. Oplog size is managed through the
`replSetResizeOplog` admin command, which is separate from `replSetReconfig`.

## Schema

**OPLOG-001** (Ubiquitous): The `mongodb_shard_config` resource schema SHALL
define an Optional `oplog_size_mb` field of TypeFloat with no default value,
representing the maximum oplog size in megabytes.

**OPLOG-002** (Ubiquitous): The `oplog_size_mb` field SHALL have a ValidateFunc
that rejects values less than or equal to zero.

## Write Path

**OPLOG-003** (Event Driven): WHEN the Update method processes a resource with
`oplog_size_mb` set, it SHALL execute `replSetResizeOplog` admin command with
the configured size AFTER the `replSetReconfig` call completes.

**OPLOG-004** (Event Driven): WHEN `oplog_size_mb` is NOT configured in the HCL,
the Update method SHALL NOT execute `replSetResizeOplog`.

## Read Path

**OPLOG-005** (Event Driven): WHEN the Read method runs and `oplog_size_mb`
exists in Terraform state, it SHALL read the current oplog size via `collStats`
on `local.oplog.rs` and convert the `maxSize` field from bytes to megabytes by
dividing by 1048576.

## Initialization

**OPLOG-006** (Event Driven): WHEN `initializeReplicaSet` completes and
`oplog_size_mb` is configured, it SHALL apply the oplog size via
`replSetResizeOplog` AFTER the PRIMARY is elected and the replica set is stable.

## Error Handling

**OPLOG-007** (Unwanted Behaviour): IF `replSetResizeOplog` returns an error,
THEN the resource SHALL return a diagnostic error including the command name and
error message.

**OPLOG-008** (Unwanted Behaviour): IF `collStats` on `local.oplog.rs` returns
an error during Read, THEN the resource SHALL return a diagnostic error.
