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
on `local.oplog.rs` on every data-bearing member and convert the `maxSize`
field from bytes to megabytes by dividing by 1048576.

## Initialization

**OPLOG-006** (Event Driven): WHEN `initializeReplicaSet` completes and
`oplog_size_mb` is configured, it SHALL apply the oplog size via
`replSetResizeOplog` AFTER the PRIMARY is elected and the replica set is stable.

## Error Handling

**OPLOG-007** (Unwanted Behaviour): IF `replSetResizeOplog` returns an error,
THEN the resource SHALL return a diagnostic error including the command name and
error message.

**OPLOG-008** (Unwanted Behaviour): IF a data-bearing member cannot be read
during Read, THEN it SHALL be skipped with a warning and the size computed
from the remaining members. IF no member can be read, THEN the value already
in state SHALL be kept (with a warning), so a down member does not block
refresh of the resource.

## Member Fan-Out

The oplog is not part of the replica set's shared configuration document:
`local.oplog.rs` is a capped collection each member owns independently, and
`replSetResizeOplog` only resizes the member it is issued against. A resize
must therefore reach every member individually.

**OPLOG-009** (Ubiquitous): WHEN `oplog_size_mb` is configured, the resize
SHALL be applied to every data-bearing member of the replica set via a direct
connection to each member, not only the member the provider is connected to.

**OPLOG-010** (Ubiquitous): The resize SHALL be applied to secondaries first
and the primary last, matching the documented procedure for changing the
oplog size of a replica set. IF no primary is identifiable, THEN members
SHALL be resized in configuration order.

**OPLOG-011** (Ubiquitous): Arbiter members SHALL be excluded from both the
resize and the read-back, as arbiters carry no oplog.

**OPLOG-012** (Unwanted Behaviour): IF the resize fails on any member, THEN
the resource SHALL return a diagnostic error identifying that member's host.
Members already resized keep their new size; `replSetResizeOplog` is
idempotent, so a subsequent apply converges the remaining members.

**OPLOG-013** (Event Driven): WHEN reading the oplog size back into state,
the resource SHALL store the common size when all readable data-bearing
members agree, and `-1` when they disagree, so that any divergent member
(undersized or oversized, e.g. after a partially failed shrink) surfaces as
a diff in the next plan. A non-positive reported size SHALL be treated as a
read failure so `0` never reaches state, where it would read as unset.

**OPLOG-014** (Event Driven): WHEN `oplog_size_mb` is set but unchanged and
the resource is not being created, the Update method SHALL NOT execute the
resize fan-out.

**OPLOG-015** (Ubiquitous): The `oplog_size_mb` field SHALL conflict with
`host_override` at plan time, because the per-member connections dial the
member hostnames from the replica set configuration and cannot honor an
override.

**OPLOG-016** (Unwanted Behaviour): IF a member is not in PRIMARY or
SECONDARY state when the resize runs, THEN that member SHALL be skipped with
a warning; its size surfaces as drift on the next plan. IF `replSetGetStatus`
fails or reports no primary, THEN the resize SHALL proceed in configuration
order with a warning rather than fail.

**OPLOG-017** (Ubiquitous): Per-member connections SHALL authenticate with
the provider's credentials. The no-auth fallback (INIT-018) SHALL apply only
on paths reached from the replica set initialization flow — including the
already-initialized delegation into the update logic (INIT-015, INIT-030),
where the replica set may exist before any users do. On steady-state Update
and Read paths, an authentication failure SHALL surface as an error.

**OPLOG-018** (Ubiquitous): The per-member read-back connections SHALL be
established concurrently, each bounded by a per-member timeout shorter than
the resize timeout, so refresh latency is bounded by the slowest member
rather than the sum across members. The resize fan-out remains sequential to
preserve the OPLOG-010 ordering.
