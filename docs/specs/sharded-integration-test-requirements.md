# Sharded Integration Test Requirements

**Spec ID Prefix:** SINTEG
**Status:** Draft
**Last Updated:** 2026-03-03

---

## Cluster Setup Requirements

SINTEG-001: WHEN the sharded integration tests run, the system SHALL start a
Docker network shared by all cluster containers.

SINTEG-002: WHEN the sharded integration tests run, the system SHALL start a
config server replica set (`configRS`) with one member (`configsvr0`) on port
27019.

SINTEG-003: WHEN the sharded integration tests run, the system SHALL start two
shard replica sets (`shard01` on `shard01svr0:27018` and `shard02` on
`shard02svr0:27018`), each with one member.

SINTEG-004: WHEN the sharded integration tests run, the system SHALL start a
mongos router (`mongos0`) on port 27017 connected to the config server replica
set, with both shards registered via `sh.addShard`, and an admin user created
with root privileges.

## Detection Tests

SINTEG-005: WHEN `DetectConnectionType` is called against a mongos router, the
system SHALL return `ConnTypeMongos`.

SINTEG-006: WHEN `ListShards` is called against a mongos router with two
registered shards, the system SHALL return a `ShardList` containing exactly 2
shards with IDs `shard01` and `shard02`.

## Shard Lookup Tests

SINTEG-007: WHEN `FindShardByName` is called with a valid shard name from the
`ShardList`, the system SHALL return the host string for that shard.

SINTEG-008: WHEN `FindShardByName` is called with a non-existent shard name,
the system SHALL return an error listing the available shard names.

## Shard Client Resolution Tests

SINTEG-009: WHEN `ResolveShardClient` is called against a mongos with a valid
shard name and a `host_override` pointing to the shard's mapped port, the
system SHALL return a working client that can ping the shard.

SINTEG-010: WHEN `GetReplSetConfig` is called via a shard client resolved
through `ResolveShardClient`, the system SHALL return a config whose `_id`
matches the shard's replica set name.

SINTEG-011: WHEN `SetReplSetConfig` is called via a shard client resolved
through `ResolveShardClient` with a modified setting, THEN a subsequent
`GetReplSetConfig` SHALL reflect the updated value.

## Multi-Shard Tests

SINTEG-012: WHEN both shards are resolved independently via
`ResolveShardClient`, the system SHALL return clients with different replica
set names (`shard01` and `shard02`).

## Direct RS Passthrough Tests

SINTEG-013: WHEN `DetectConnectionType` is called against a direct shard
replica set member, the system SHALL return `ConnTypeReplicaSet`.

SINTEG-014: WHEN `ResolveShardClient` is called against a client already
connected directly to a replica set (not mongos), the system SHALL return the
same provider client with a no-op cleanup function.
