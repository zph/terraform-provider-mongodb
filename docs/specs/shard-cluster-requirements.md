# Shard Cluster Management Requirements

## Overview

This document defines EARS requirements for the `mongodb_shard` Terraform
resource, which manages adding and removing shards from a MongoDB sharded
cluster via the mongos router's `addShard` and `removeShard` admin commands.

**System Name:** `mongodb_shard` resource
**Depends on:** DISC-001 through DISC-010

## Schema

**CLUS-001** (Ubiquitous): The resource SHALL have the following schema:
`shard_name` (Required, ForceNew — allowlisted exception per DANGER-017 and DANGER-018),
`hosts` (Required, TypeList of String, immutable via CustomizeDiff — DANGER-016),
`state` (Computed), `remove_timeout_secs` (Optional, Default 300).

## Create (addShard)

**CLUS-002** (Event Driven): WHEN Create is called, the resource SHALL run
`addShard` with a connection string in the format `"rsName/host1:port,host2:port"`.

**CLUS-003** (Event Driven): WHEN `addShard` succeeds, the resource SHALL
read back state via `listShards`.

## Read

**CLUS-004** (Event Driven): WHEN Read is called, the resource SHALL run
`listShards` and update the `state` attribute from the matching shard entry.

**CLUS-005** (Unwanted Behaviour): IF the shard is not found in `listShards`,
THEN the resource SHALL clear the resource ID to remove it from state.

## Delete (removeShard)

**CLUS-006** (Event Driven): WHEN Delete is called, the resource SHALL run
`removeShard` and poll until the state is `"completed"`.

## Identity Field Immutability

**CLUS-007** (Ubiquitous): Changes to `hosts` SHALL be blocked at plan time by
`CustomizeDiff` returning an error (DANGER-016). Changes to `shard_name` SHALL
be blocked at plan time by `CustomizeDiff` (DANGER-017) and additionally by
`ForceNew: true` as a defense-in-depth allowlisted exception (DANGER-017,
DANGER-018). The Update function SHALL handle `remove_timeout_secs` changes
gracefully as a client-side-only value requiring no MongoDB operation
(DANGER-019).

## Polling

**CLUS-008** (Event Driven): WHEN `removeShard` returns an ongoing state, the
resource SHALL poll at 5-second intervals until completed or timeout.

## Timeout

**CLUS-009** (Ubiquitous): The default remove timeout SHALL be 300 seconds.

**CLUS-010** (Optional Feature): WHERE `remove_timeout_secs` is set, the
resource SHALL use the specified value as the remove timeout in seconds.

## addShard Retry

**CLUS-011** (Event Driven): WHEN `addShard` fails with MongoDB error code
133 (FailedToSatisfyReadPreference), the resource SHALL retry at 2-second
intervals until `add_timeout_secs` is exceeded, because mongos may not have
discovered the newly initialized RS primary yet.

**CLUS-012** (Unwanted Behaviour): IF `addShard` retries do not succeed
before `add_timeout_secs`, the resource SHALL return the last error.

**CLUS-013** (Ubiquitous): The default add timeout SHALL be 60 seconds.

**CLUS-014** (Optional Feature): WHERE `add_timeout_secs` is set, the
resource SHALL use the specified value as the addShard retry timeout.
