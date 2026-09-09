# Shard Replica Set Initialization Requirements

## Overview

This document defines EARS requirements for initializing a fresh MongoDB
replica set via the `mongodb_shard_config` Terraform resource. Initialization
covers two phases: initiating the replica set on the first member using
`replSetInitiate`, then handing over to the same reconciliation Update
performs on any initialized set, which applies the settings and adds the
remaining members one `replSetReconfig` at a time (SHARD-012 through
SHARD-021).

**System Name:** `mongodb_shard_config` resource
**Depends on:** SHARD-001 through SHARD-021, DISC-001 through DISC-010

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

**INIT-005** (Ubiquitous): The resource SHALL give the first `member` block
`_id: 0` in `replSetInitiate`. Members added afterwards receive `_id` values
per SHARD-014 (one greater than the highest in use), never their position in
the `member` list.

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

## Phase 2: Hand Over to Reconciliation

**INIT-010** (Event Driven): WHEN the first member reaches PRIMARY state,
the resource SHALL run the Update reconciliation against it over the
initialization connection: one `replSetReconfig` applies the settings and the
first member's fields, then each remaining `member` block is added per
SHARD-013 through SHARD-017, and state is derived from the final configuration
per SHARD-018. No initialization-specific member logic exists beyond
`replSetInitiate`.

**INIT-011** (Event Driven): WHEN adding remaining members, the resource
SHALL apply all per-member fields (priority, votes, hidden, arbiter_only,
build_indexes, tags) from the corresponding `member` blocks (SHARD-015).

**INIT-012** (Event Driven): WHEN handing over to reconciliation, the resource
SHALL apply the replica set settings (chaining_allowed,
heartbeat_interval_millis, heartbeat_timeout_secs, election_timeout_millis,
catch_up_timeout_millis) from the HCL configuration in the first
`replSetReconfig`, before any member is added.

## Health Check

**INIT-013** (Event Driven): WHEN a member has been added, the resource SHALL
wait until the primary reports it with `health: 1` in any state, or until the
initialization timeout is reached (SHARD-016). Initial sync is not waited
for. This supersedes the earlier majority-healthy wait, which never observed
the added member once the set had two or more members and, for a one-member
set, blocked on initial sync.

**INIT-014** (Unwanted Behaviour): IF the added member does not become
reachable within the initialization timeout, THEN the resource SHALL remove
it again and return a diagnostic error naming the host and the last observed
status (SHARD-017).

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

## Post-Election Write Readiness

**INIT-025** (Event Driven): WHEN `replSetReconfig` fails with MongoDB error
code 17405 (Location17405, "logOp() but can't accept write to collection"),
the resource SHALL retry the reconfig with `initPollInterval` backoff until
the `init_timeout_secs` deadline is exceeded, because a freshly elected
PRIMARY cannot accept writes until it completes its initial no-op oplog entry.

**INIT-026** (Unwanted Behaviour): IF retries of `replSetReconfig` do not
succeed before `init_timeout_secs`, the resource SHALL return an error
indicating the transient error was not resolved within the timeout.

**INIT-027** (Event Driven): WHEN `replSetReconfig` fails with MongoDB error
code 103 (NewReplicaSetConfigurationIncompatible) due to a stale config
version, the resource SHALL re-read the current config version from the
server, increment it, and retry the reconfig, because internal
auto-reconfigurations (e.g. term updates after election) can bump the version
between `replSetGetConfig` and `replSetReconfig`.

**INIT-028** (Unwanted Behaviour): IF the config version re-read fails during
a version-conflict retry, the resource SHALL return an error immediately
rather than retrying with stale data.

## Auth Fallback on Create

**INIT-029** (Event Driven): WHEN `getShardClient` fails with an
authentication or authorization error (codes 13 or 18, or a SCRAM handshake
failure) during Create, the resource SHALL fall through to the initialization
flow via `initializeReplicaSet`, because on a fresh instance the admin user
may not exist yet. The initialization flow uses `ConnectForInit` which has its
own no-auth fallback via the localhost exception.

## Scope

**INIT-024** (Event Driven): WHEN the provider is connected to a mongos
router (detected per DISC-001), the resource SHALL NOT enter the
initialization flow, because shards listed via `listShards` are already
initialized.
