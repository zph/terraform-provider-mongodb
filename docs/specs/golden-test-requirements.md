# Golden File Testing Engine — EARS Specification

**Prefix:** GOLDEN
**Status:** Implemented
**Last Updated:** 2026-03-03

---

## Command Recording

GOLDEN-001: WHEN a MongoDB command is started, the CommandRecorder SHALL capture the command name, database, and BSON body.

GOLDEN-002: WHEN a recorded command is one of the noise commands (hello, saslStart, saslContinue, ping, endSessions, isMaster, ismaster, buildInfo, getFreeMonitoringStatus, getLog), the CommandRecorder SHALL discard it.

GOLDEN-003: WHEN rendering the BSON body to JSON, the CommandRecorder SHALL strip driver-injected fields ($db, $readPreference, lsid, $clusterTime).

GOLDEN-004: WHEN the BSON body contains a "pwd" field, the CommandRecorder SHALL replace its value with "[REDACTED]".

GOLDEN-005: WHEN String() is called, the CommandRecorder SHALL produce output in the format "Source: \<source\>\nCommand: \<name\>\nDatabase: \<db\>\nBody:\n\<json\>" separated by blank lines. WHEN source is empty, the Source line SHALL be omitted.

## Golden File Comparison

GOLDEN-006: WHEN the UPDATE_GOLDEN environment variable is set, goldenCompare SHALL write the golden file instead of comparing.

GOLDEN-007: WHEN the golden file does not exist, goldenCompare SHALL create it on first run.

GOLDEN-008: WHEN the output differs from the golden file, goldenCompare SHALL fail the test with a diff message.

## Integration Tests

GOLDEN-009: WHEN a golden integration test runs, it SHALL capture all MongoDB commands for the resource lifecycle and compare against a golden file.

GOLDEN-010: WHEN the shard config golden test runs, it SHALL normalize dynamic values (ObjectIDs, host:port, version numbers) before comparison.

GOLDEN-011: WHEN TestGolden_DbUser_Basic runs, it SHALL capture createUser, usersInfo, dropUser+createUser (update), usersInfo, dropUser commands.

GOLDEN-012: WHEN TestGolden_Pattern_MonitoringUser runs, it SHALL capture the full lifecycle of a monitoring role and exporter user.

GOLDEN-013: WHEN TestGolden_Pattern_RoleHierarchy runs, it SHALL capture the full lifecycle of a 3-tier role hierarchy with 3 users.

## Cleanup and Isolation

GOLDEN-014: WHEN a golden test completes, t.Cleanup SHALL drop all created resources to avoid collisions with other tests.

GOLDEN-015: WHEN a golden test creates resources, it SHALL use the "golden_" prefix on all resource names to avoid collisions with existing integration tests.

## Shard Config Normalization

GOLDEN-016: WHEN normalizeReplSetBody processes output, it SHALL replace ObjectID hex strings with \<OBJECT_ID\>.

GOLDEN-017: WHEN normalizeReplSetBody processes output, it SHALL replace host:port patterns with \<HOST:PORT\>.

GOLDEN-018: WHEN normalizeReplSetBody processes output, it SHALL replace version and term numbers with \<VERSION\> and \<TERM\> placeholders.

## Sharded Golden Tests

GOLDEN-019: WHEN normalizeShardedBody processes output, it SHALL normalize shard host strings with \<SHARD_HOST\> and shard state values with \<SHARD_STATE\>, in addition to all replSetBody normalizations.

GOLDEN-020: WHEN TestGolden_ShardConfig_MongosDiscovery runs, it SHALL capture listShards via mongos, then replSetGetConfig, replSetReconfig, replSetGetConfig on a shard, and compare against a golden file with sharded normalization.

GOLDEN-021: WHEN TestGolden_ShardConfig_MultiShard runs, it SHALL capture listShards via mongos, then independent replSetGetConfig on both shard01 and shard02, and compare against a golden file with sharded normalization.

## Shard Resource Golden Tests

GOLDEN-022: WHEN TestGolden_Shard_AddRemove runs, it SHALL start a third shard container, capture addShard, listShards, removeShard (polling), and a final listShards verification, and compare against a golden file without normalization (shard commands use deterministic internal Docker hostnames).

GOLDEN-023: WHEN TestGolden_Shard_ListShards runs against the existing sharded cluster, it SHALL capture the listShards command and compare against a golden file without normalization.
