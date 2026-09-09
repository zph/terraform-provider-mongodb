# Integration Test Suite Requirements

## Overview

This document specifies requirements for integration tests in the terraform-provider-mongodb project. These tests use Testcontainers to spin up a real MongoDB replica set and verify CRUD operations, connection handling, and replica set configuration against a live database.

**System Name:** Integration Test Suite
**Version:** 1.0
**Last Updated:** 2026-03-02

## Prerequisites

- Docker (or Podman) running locally
- `//go:build integration` build tag
- Run via: `make test-integration`

## Requirements

### MongoDB Connection

**Test file:** `mongodb/integration_test.go`
**Source function:** `MongoClientInit`

**INTEG-001:** Event Driven

**Requirement:**
When `MongoClientInit` is called with a valid `MongoDatabaseConfiguration` pointing to a running MongoDB replica set, the Integration Test Suite SHALL return a connected `*mongo.Client` with a nil error, and the client SHALL respond to a Ping.

**Rationale:**
The provider's core connectivity MUST be verified against a real MongoDB instance to catch driver version incompatibilities and configuration issues that unit tests cannot surface.

**Verification:**
Start a MongoDB replica set via Testcontainers, configure `MongoDatabaseConfiguration` with the container's host/port and admin credentials, call `MongoClientInit`, assert client is non-nil, error is nil, and `client.Ping` succeeds.

---

**INTEG-002:** Unwanted Behaviour

**Requirement:**
If `MongoClientInit` is called with invalid credentials against a running MongoDB instance, then the Integration Test Suite SHALL verify that it returns a non-nil error.

**Rationale:**
Authentication failures MUST propagate as errors rather than returning a client that silently fails on subsequent operations.

**Verification:**
Start MongoDB via Testcontainers, configure `MongoDatabaseConfiguration` with wrong password, call `MongoClientInit`, assert error is non-nil.

---

### User CRUD

**Test file:** `mongodb/integration_test.go`
**Source functions:** `createUser`, `getUser`

**INTEG-003:** Event Driven

**Requirement:**
When `createUser` is called with a valid `DbUser` and roles against a live MongoDB, the Integration Test Suite SHALL verify that the user can subsequently be retrieved by `getUser` with matching username and database.

**Rationale:**
User creation is the primary write operation of the `mongodb_db_user` resource and MUST produce a retrievable user in MongoDB.

**Verification:**
Create user "testuser" in "admin" database with `readWrite` role, call `getUser`, assert returned user list has 1 entry with matching username.

---

**INTEG-004:** Event Driven

**Requirement:**
When `createUser` is called with an empty roles slice, the Integration Test Suite SHALL verify that the user is created successfully with no roles assigned.

**Rationale:**
The provider allows users with no roles, and this edge case MUST work against real MongoDB.

**Verification:**
Create user with empty roles slice, call `getUser`, assert user exists with 0 roles.

---

**INTEG-005:** Event Driven

**Requirement:**
When `getUser` is called for a non-existent username, the Integration Test Suite SHALL verify that the returned `SingleResultGetUser.Users` slice is empty.

**Rationale:**
Read operations for missing resources MUST return empty results so the provider can detect resource deletion.

**Verification:**
Call `getUser` with "nonexistent" username, assert `result.Users` has length 0.

---

### Role CRUD

**Test file:** `mongodb/integration_test.go`
**Source functions:** `createRole`, `getRole`

**INTEG-006:** Event Driven

**Requirement:**
When `createRole` is called with a role name, inherited roles, and privileges against a live MongoDB, the Integration Test Suite SHALL verify that the role can subsequently be retrieved by `getRole` with matching role name and database.

**Rationale:**
Role creation is the primary write operation of the `mongodb_db_role` resource and MUST produce a retrievable role in MongoDB.

**Verification:**
Create role "testrole" in "admin" database with a db-scoped privilege, call `getRole`, assert returned role list has 1 entry with matching role name.

---

**INTEG-007:** Event Driven

**Requirement:**
When `createRole` is called with a `PrivilegeDto` that has both `Db` set and `Cluster=true`, the Integration Test Suite SHALL verify that it returns a non-nil error.

**Rationale:**
MongoDB does not allow privileges scoped to both a database and the cluster, and the provider MUST validate this before sending the command.

**Verification:**
Call `createRole` with a privilege having `Db: "test"` and `Cluster: true`, assert error is non-nil and contains expected message.

---

**INTEG-008:** Event Driven

**Requirement:**
When `getRole` is called for a non-existent role name, the Integration Test Suite SHALL verify that the returned `SingleResultGetRole.Roles` slice is empty.

**Rationale:**
Read operations for missing roles MUST return empty results for provider resource detection.

**Verification:**
Call `getRole` with "nonexistent" role name, assert `result.Roles` has length 0.

---

### Replica Set Configuration

**Test file:** `mongodb/integration_test.go`
**Source functions:** `GetReplSetConfig`, `SetReplSetConfig`, `GetReplSetStatus`

**INTEG-009:** Event Driven

**Requirement:**
When `GetReplSetConfig` is called against a running MongoDB replica set, the Integration Test Suite SHALL return an `*RSConfig` with a non-empty ID and at least one member.

**Rationale:**
Reading replica set configuration is required for the `mongodb_shard_config` resource's Read operation and MUST return valid structure from a real replica set.

**Verification:**
Call `GetReplSetConfig`, assert returned config has non-empty `ID`, non-nil `Members` with length >= 1.

---

**INTEG-010:** Event Driven

**Requirement:**
When `SetReplSetConfig` is called with a modified `RSConfig` (incremented version, changed `HeartbeatIntervalMillis`), the Integration Test Suite SHALL verify that a subsequent `GetReplSetConfig` returns the updated settings.

**Rationale:**
Writing replica set configuration is the core operation of `mongodb_shard_config` and MUST persist changes that are readable on the next fetch.

**Verification:**
Call `GetReplSetConfig`, modify `HeartbeatIntervalMillis` to 3000 and increment version, call `SetReplSetConfig`, call `GetReplSetConfig` again, assert `HeartbeatIntervalMillis` equals 3000.

---

**INTEG-011:** Event Driven

**Requirement:**
When `GetReplSetStatus` is called against a running MongoDB replica set, the Integration Test Suite SHALL return a `*ReplSetStatus` with set name matching the configured replica set, at least one member, and `MyState` equal to PRIMARY.

**Rationale:**
Replica set status is required for health checks and member discovery, and MUST return valid structure from a real replica set.

**Verification:**
Call `GetReplSetStatus`, assert `Set` equals "rs0", `Members` length >= 1, `MyState` equals `MemberStatePrimary`.

---

**INTEG-012:** Event Driven

**Requirement:**
When `GetReplSetStatus` is called and `GetSelf()` is invoked on the result, the Integration Test Suite SHALL verify that the returned member is non-nil, has `Self=true`, state PRIMARY, and health UP.

**Rationale:**
`GetSelf` is used to identify the current node in the replica set and MUST correctly locate the self member from a real status response.

**Verification:**
Call `GetReplSetStatus`, call `GetSelf()`, assert non-nil, `Self` is true, `State` equals `MemberStatePrimary`, `Health` equals `MemberHealthUp`.

---

**INTEG-013:** Event Driven

**Requirement:**
When `GetReplSetStatus` is called and `Primary()` is invoked on the result, the Integration Test Suite SHALL verify that the returned member is non-nil, has state PRIMARY, and matches the member returned by `GetSelf()`.

**Rationale:**
On a single-node replica set the primary and self MUST be the same member, validating both lookup methods against a real response.

**Verification:**
Call `GetReplSetStatus`, call `Primary()` and `GetSelf()`, assert both are non-nil and have the same `Name`.

---

**INTEG-014:** Event Driven

**Requirement:**
When `GetReplSetStatus` is called on a single-node replica set and `GetMembersByState(MemberStateSecondary, 0)` is invoked, the Integration Test Suite SHALL verify that the returned slice is empty.

**Rationale:**
A single-node replica set has no secondaries, and the filter MUST correctly return an empty result rather than a false positive.

**Verification:**
Call `GetReplSetStatus`, call `GetMembersByState(MemberStateSecondary, 0)`, assert length is 0.

---

**INTEG-015:** Event Driven

**Requirement:**
When `createRole` is called with a `PrivilegeDto` that has `Cluster=true` and empty `Db`, the Integration Test Suite SHALL verify that the role is created successfully and `getRole` returns a privilege with `Resource.Cluster=true`.

**Rationale:**
Cluster-scoped privileges are a valid MongoDB pattern and the provider MUST support creating roles with them.

**Verification:**
Call `createRole` with `Cluster: true` and empty `Db`, call `getRole`, assert 1 privilege with `Resource.Cluster` equal to true.

---

**INTEG-016:** Event Driven

**Requirement:**
When `SetReplSetConfig` is called with multiple modified settings (`ChainingAllowed`, `HeartbeatIntervalMillis`, `ElectionTimeoutMillis`), the Integration Test Suite SHALL verify that a subsequent `GetReplSetConfig` returns all three updated values.

**Rationale:**
The `mongodb_shard_config` resource updates multiple settings atomically and all MUST persist in a single reconfig call.

**Verification:**
Call `GetReplSetConfig`, set `ChainingAllowed=false`, `HeartbeatIntervalMillis=3500`, `ElectionTimeoutMillis=15000`, increment version, call `SetReplSetConfig`, re-read and assert all three values match.

---

**INTEG-022:** Event Driven

**Requirement:**
When `updateWithClient` runs against a one-member replica set with three `member` blocks, the Integration Test Suite SHALL verify that the pre-flight reads a node not yet added as addable (SHARD-027), that the two missing members are added with `_id` 1 and 2, that the voting member ends with its configured votes and priority after being staged as a non-voter, that every per-member field (priority, votes, hidden, tags) is applied, that the seed member's fields are merged in place, that the config version advanced by at least four (settings, staged add, promotion, second add), that the `member` state lists all three hosts in block order, and that all three members become PRIMARY or SECONDARY.

**Rationale:**
This is the host-side-seed workflow: something else initiates the set with one member and the provider grows it. It exercises SHARD-012 through SHARD-018, SHARD-020 and SHARD-022 against a real server on every version in the matrix, including the priority-0 hidden member that the `omitempty` tag used to break.

**Verification:**
Start three mongods on one Docker network, `rs.initiate` the first alone, probe the second node through its mapped port with `memberProbeFor` and assert `CheckAddTarget` returns the addable verdict, build a `ResourceData` with three blocks via `schema.TestResourceDataRaw`, call `RShardConfig.updateWithClient`, and assert on `GetReplSetConfig`, the resource state and `replSetGetStatus`.

---

**INTEG-023:** Event Driven

**Requirement:**
When `updateWithClient` runs a second time against the grown set with the same blocks, the Integration Test Suite SHALL verify that no member is added and that every member keeps its `_id` and host.

**Rationale:**
The partition is computed against the live configuration, so a repeated apply must converge without changing membership (SHARD-012, SHARD-019).

**Verification:**
Call `updateWithClient` again with the same configuration and compare `GetReplSetConfig` before and after.

---

**INTEG-024:** Event Driven

**Requirement:**
When `updateWithClient` runs against the grown set with the hidden member's block changed from `votes = 0` to `votes = 1`, the Integration Test Suite SHALL verify that the member ends with votes 1, priority 0, hidden and the same `_id`, that the config version advanced by at least two (settings, promotion), and that the resource state shows the new votes.

**Rationale:**
Raising a live member's votes is the second half of a staged add and the path for turning a non-voter into a voter. It must wait for SECONDARY and go out in its own reconfig (SHARD-023).

**Verification:**
After INTEG-022 and INTEG-023, wait for all members to be PRIMARY or SECONDARY, call `updateWithClient` with the third block's votes set to 1, and assert on `GetReplSetConfig` and the resource state.

---

**INTEG-025:** Unwanted Behaviour

**Requirement:**
When `updateWithClient` runs against the grown set with a fourth block naming a host that does not resolve and `init_timeout_secs` of 15, the Integration Test Suite SHALL verify that the call returns an error naming the host and saying it was removed again, that `GetReplSetConfig` still lists exactly the three live members with unchanged `_id` values, and that the resource's `member` state lists only those three hosts.

**Rationale:**
A member the primary reports down for the whole wait is rolled back (SHARD-017), and state must describe the members the server has even when the apply fails, because the SDK persists state on error (SHARD-021). The pre-flight of SHARD-027 cannot reach the container aliases from the test process, so this test also exercises its pass-through path.

**Verification:**
After INTEG-022, wait for all members to be PRIMARY or SECONDARY, call `updateWithClient` with the three-member configuration plus `rsgrow-nope:27017`, and assert on the diagnostics, `GetReplSetConfig` and `data.Get("member")`.

---

**INTEG-026:** Unwanted Behaviour

**Requirement:**
When `updateWithClient` runs against the grown set with a fourth block whose host is the address at which the test process reaches the first node (its mapped port), the Integration Test Suite SHALL verify that the call returns an error naming that address and the name the set knows the node by, that no `replSetReconfig` was sent (the config version is unchanged), and that the resource recorded no ID. It SHALL also verify that probing each node through its mapped port yields a refusal naming that node's network alias.

**Rationale:**
The mapped address is an alias of the network name the set knows the node by, which is exactly the FQDN-for-short-name case the pre-flight of SHARD-027 exists for. This exercises the refusal path, and the `isMaster` field names it depends on, against a real server on every version in the matrix.

**Verification:**
After INTEG-022, wait for all members to be PRIMARY or SECONDARY, run `memberProbeFor` and `CheckAddTarget` against each node's mapped address, then call `updateWithClient` with the three-member configuration plus the first node's mapped address and assert on the diagnostics, `GetReplSetConfig` and `data.Id()`.

---

**INTEG-027:** Unwanted Behaviour

**Requirement:**
When `updateWithClient` runs against the grown set with a fourth block that has `arbiter_only = true` and a host that does not resolve, the Integration Test Suite SHALL verify that the call returns an error naming the arbiter host and saying it could not be inspected and can be added by hand, and that no `replSetReconfig` was sent.

**Rationale:**
An arbiter is added with its vote and cannot be staged, so an arbiter host the pre-flight cannot inspect is refused before anything is sent (SHARD-028). A data-bearing block with the same host is added and rolled back instead (INTEG-025); the two tests together pin the difference.

**Verification:**
After INTEG-022, call `updateWithClient` with the three-member configuration plus `rsgrow-nope:27017` as an arbiter and assert on the diagnostics and `GetReplSetConfig`.

---

## Test File Summary

| Test File | Requirements | Source File |
|---|---|---|
| `mongodb/integration_test.go` | INTEG-001 through INTEG-016 | `mongodb/config.go`, `mongodb/replica_set_types.go` |
| `mongodb/member_add_integration_test.go` | INTEG-022 through INTEG-027 | `mongodb/shard_members.go`, `mongodb/resource_shard_config.go` |

## Testcontainer Configuration

The integration tests use a single shared MongoDB replica set container for all tests:

- Image: `mongo:7` (or configurable via env var)
- Replica set name: `rs0`
- Authentication: enabled with admin user
- Container lifecycle: started once per test suite via `TestMain`, torn down after all tests complete

The member-add tests (INTEG-022 through INTEG-027) start their own three-node replica set on a dedicated Docker network the first time one of them runs, without authentication, and skip if it cannot start. `TestMain` tears it down with the rest.
