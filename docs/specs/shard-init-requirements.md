# Shard Replica Set Initialization Requirements

## Overview

This document defines EARS requirements for initializing a fresh MongoDB
replica set via the `mongodb_shard_config` Terraform resource. Initialization
covers two phases: initiating the replica set on the first member using
`replSetInitiate`, then adding remaining members via `replSetReconfig`.

**System Name:** `mongodb_shard_config` resource
**Depends on:** SHARD-001 through SHARD-011, DISC-001 through DISC-010

## Detection

**INIT-001** (Event Driven): WHEN the Create method calls `replSetGetConfig`
and receives MongoDB error code 94 (`NotYetInitialized`), the resource SHALL
enter the initialization flow instead of the reconfiguration flow.

**INIT-002** (Event Driven): WHEN the Create method calls `replSetGetConfig`
and receives a valid `RSConfig`, the resource SHALL skip initialization and
proceed with the existing reconfiguration flow (Update behavior).

## Member Block Semantics

**INIT-003** (Unwanted Behaviour): IF the resource enters the initialization
flow and no `member` blocks are declared in the HCL configuration, THEN the
resource SHALL return a diagnostic error stating that `member` blocks are
required for replica set initialization.

**INIT-004** (Event Driven): WHEN the resource enters the initialization
flow, the resource SHALL treat each `member` block as a declarative member
of the new replica set, using the `host` field as the member address and all
other fields (priority, votes, hidden, arbiter_only, build_indexes, tags)
as the initial configuration for that member.

**INIT-005** (Ubiquitous): The resource SHALL assign `_id` values to members
sequentially starting from 0, following the order of `member` blocks in the
HCL configuration.

## Phase 1: Initiate First Member

**INIT-006** (Event Driven): WHEN the resource enters the initialization
flow, the resource SHALL connect in direct mode to the host declared in the
first `member` block.

**INIT-007** (Event Driven): WHEN connected to the first member in direct
mode, the resource SHALL run `replSetInitiate` with a config document
containing `_id` set to the `shard_name` attribute, `version` set to 1,
and a single-element `members` array with `_id: 0` and `host` set to the
first member's `host` value.

**INIT-008** (Event Driven): WHEN `replSetInitiate` succeeds, the resource
SHALL poll `replSetGetStatus` until `myState` equals 1 (PRIMARY) or until
the initialization timeout is reached.

**INIT-009** (Unwanted Behaviour): IF the `replSetGetStatus` poll does not
observe PRIMARY state within the initialization timeout, THEN the resource
SHALL return a diagnostic error indicating the replica set did not elect a
primary within the timeout period.

## Phase 2: Add Remaining Members

**INIT-010** (Event Driven): WHEN the first member reaches PRIMARY state
and additional `member` blocks exist, the resource SHALL retrieve the
current config via `replSetGetConfig`, append the remaining members to the
`members` array with sequentially assigned `_id` values starting from 1,
increment the config `version`, and call `replSetReconfig`.

**INIT-011** (Event Driven): WHEN adding remaining members via
`replSetReconfig`, the resource SHALL apply all per-member fields (priority,
votes, hidden, arbiter_only, build_indexes, tags) from the corresponding
`member` blocks.

**INIT-012** (Event Driven): WHEN adding remaining members via
`replSetReconfig`, the resource SHALL also apply replica set settings
(chaining_allowed, heartbeat_interval_millis, heartbeat_timeout_secs,
election_timeout_millis) from the HCL configuration.

## Health Check

**INIT-013** (Event Driven): WHEN all members have been added via
`replSetReconfig`, the resource SHALL poll `replSetGetStatus` until a
majority of members report a healthy state (PRIMARY or SECONDARY) or until
the initialization timeout is reached.

**INIT-014** (Unwanted Behaviour): IF a majority of members do not reach a
healthy state within the initialization timeout, THEN the resource SHALL
return a diagnostic error listing the unhealthy members and their states.

## Idempotency

**INIT-015** (Unwanted Behaviour): IF `replSetInitiate` returns MongoDB
error code 23 (`AlreadyInitialized`), THEN the resource SHALL treat the
replica set as already initialized and fall through to the reconfiguration
flow.

**INIT-016** (Event Driven): WHEN the resource enters the reconfiguration
flow after an `AlreadyInitialized` response, the resource SHALL use
`replSetGetConfig` to fetch the current config and proceed with standard
Update logic (member override merging per SHARD-003 through SHARD-006).

## Authentication

**INIT-017** (Event Driven): WHEN connecting to the first member for
initialization, the resource SHALL first attempt connection using the
provider's configured credentials.

**INIT-018** (Unwanted Behaviour): IF authentication fails during the
initialization connection, THEN the resource SHALL retry the connection
without authentication credentials to support fresh MongoDB instances
that have no users configured.

**INIT-019** (Ubiquitous): The resource SHALL reuse the provider's TLS
configuration and proxy settings for all initialization connections,
consistent with DISC-010.

## Timeout

**INIT-020** (Ubiquitous): The resource SHALL default the initialization
timeout to 60 seconds.

**INIT-021** (Optional Feature): WHERE the `init_timeout_secs` attribute
is set on the resource, the resource SHALL use the specified value as the
initialization timeout in seconds instead of the default.

## Connection

**INIT-022** (Ubiquitous): The resource SHALL connect in direct mode
(`Direct: true`) for all initialization operations, because the MongoDB Go
driver cannot perform replica set topology discovery against uninitialized
members.

**INIT-023** (Ubiquitous): The resource SHALL disconnect any temporary
client created during initialization after the operation completes,
consistent with DISC-007.

## Scope

**INIT-024** (Event Driven): WHEN the provider is connected to a mongos
router (detected per DISC-001), the resource SHALL NOT enter the
initialization flow, because shards listed via `listShards` are already
initialized.
