# Shard Member Configuration Requirements

This document defines EARS requirements for per-member configuration of
replica set members via the `mongodb_shard_config` Terraform resource:
matching `member` blocks to live members, merging their fields, adding
members that are not in the set yet, and reading members back.

## Schema

**SHARD-001** (Ubiquitous): The `mongodb_shard_config` resource schema SHALL
define an Optional `member` block of TypeList containing sub-fields: `host`
(Required, TypeString), `tags` (Optional, TypeMap of TypeString), `priority`
(Optional, TypeFloat, Default 1), `votes` (Optional, TypeInt, Default 1),
`hidden` (Optional, TypeBool), `arbiter_only` (Optional, TypeBool), and
`build_indexes` (Optional, TypeBool, Default true).

## Backward Compatibility

**SHARD-002** (Event Driven): WHEN the `member` block is omitted from the HCL
configuration, the `mongodb_shard_config` resource SHALL preserve the existing
behavior and leave all replica set members untouched.

## Host-Based Member Lookup

**SHARD-003** (Event Driven): WHEN the Update method processes a `member` block,
the resource SHALL locate the matching `ConfigMember` in the fetched
`RSConfig.Members` by comparing the `host` field (case-sensitive exact string
match including port).

**SHARD-004** (Event Driven): WHEN a `member` block references a `host` that
does not match any member in the current `RSConfig.Members`, the Update method
SHALL treat the block as a member to add (SHARD-012 through SHARD-017 and
SHARD-022) rather
than return an error. Before 0.4.0 this was an error whose only remedy,
removing the member and adding a new one, the resource could not perform.

## Member Field Update Merging

**SHARD-005** (Event Driven): WHEN a `member` block matches an existing
`ConfigMember` by host, the Update method SHALL apply all fields from the
Terraform-declared member block onto the matched `ConfigMember`.

**SHARD-006** (Event Driven): WHEN the `member` block is present but does not
list all replica set members, the Update method SHALL leave unlisted members
completely unchanged in the `RSConfig.Members` array.

## Read-Back for Drift Detection

**SHARD-007** (Event Driven): WHEN the Read method fetches the `RSConfig`, it
SHALL populate the Terraform state with a `member` list derived from
`RSConfig.Members`, setting all fields (host, tags, priority, votes, hidden,
arbiter_only, build_indexes) for each managed member.

**SHARD-008** (Event Driven): WHEN reading member state back from MongoDB, the
Read method SHALL only populate members in the Terraform state that are
explicitly listed in the HCL configuration.

## Tags

**SHARD-009** (Event Driven): WHEN reading or writing `tags` for a member, the
resource SHALL treat tags as a `map[string]string` matching the `ReplsetTags`
type defined in `replica_set_types.go`.

## Read-Back Ordering

**SHARD-010** (Event Driven): WHEN reading members back into Terraform state,
the Read method SHALL return members in the same order as they appear in the
HCL `member` blocks to avoid spurious plan diffs.

## No-Auth Support

**SHARD-011** (Event Driven): WHEN the provider `username` is empty,
`MongoClient` SHALL skip setting authentication credentials, behaving
identically to `MongoClientNoAuth`.

## Adding Members

**SHARD-012** (Event Driven): WHEN the Update method processes `member`
blocks, it SHALL partition them by `host` into blocks that match a live
member, which are merged per SHARD-005 in the settings reconfig, and blocks
that do not, which are added per SHARD-013. Block order is preserved within
each group.

**SHARD-013** (Ubiquitous): The Update method SHALL add each missing member
with its own `replSetReconfig`, one member per reconfig, in block order, after
the reconfig that applies the settings and the merged members. MongoDB 4.4 and
later reject a non-force reconfig that adds more than one voting member; one
member per reconfig is correct on every supported version.

**SHARD-014** (Ubiquitous): Before each add the resource SHALL re-read the
configuration from the server and SHALL assign the new member the `_id` one
greater than the highest `_id` in that configuration (0 for an empty
configuration), never its position in the `member` list.

**SHARD-015** (Event Driven): WHEN building the new member, the resource SHALL
apply every per-member field from the block (hidden, arbiter_only,
build_indexes, tags), with votes and priority as SHARD-022 describes.

**SHARD-016** (Event Driven): WHEN a reconfig that adds a member succeeds, the
resource SHALL poll `replSetGetStatus`, for up to `init_timeout_secs`, until
the primary reports the member with `health: 1` and in the state its block
requires: `SECONDARY` (or `PRIMARY`) for a member that will receive votes or a
non-zero priority, `ARBITER` for an arbiter, and any state for a member that
stays a non-voter with priority 0. Only members that will vote are made to
wait for initial sync, because only they affect the majority.

**SHARD-017** (Unwanted Behaviour): IF the primary has reported the new
member at least once and never with `health: 1` within `init_timeout_secs`,
THEN the resource SHALL remove the member it just added with a further
`replSetReconfig` (best effort) and SHALL return an error naming the host and
the last observed status; if the removal also fails, the error SHALL report
both failures. IF `replSetGetStatus` could not be read at all during the wait,
THEN the resource SHALL leave the member in place and return an error saying
the wait was inconclusive: nothing is known about the member, and a removal
could take a healthy node out of the set. IF the member is reachable but has
not reached the required state in time, THEN the resource SHALL leave it in
the set as added (a non-voter with priority 0), SHALL continue with the
remaining blocks, and SHALL complete the apply with a warning naming the host,
the observed state, and the votes and priority a later apply will give it; the
state written per SHARD-018 shows the member at votes 0, so the next plan
shows the promotion as a pending change. Initial sync of a data-bearing set
routinely outlasts the timeout, so this is the expected outcome of adding a
voter to a shard with data, not a failure. IF the add reconfig itself fails,
THEN the resource SHALL check whether the member was nevertheless installed
and remove it if so.

**SHARD-018** (Event Driven): WHEN all reconfigs have completed, the resource
SHALL read the configuration from the server and derive the `member` state
from it rather than from the configuration it sent.

**SHARD-019** (Ubiquitous): The resource SHALL NOT remove a live member that
has no `member` block. The only removal it performs is the rollback in
SHARD-017, and only for a member it added in the same apply.

**SHARD-020** (Ubiquitous): The resource SHALL send `priority` and `votes` for
every managed member, including a `priority` of 0, so the documented defaults
of 1 are applied by the resource rather than by the server and hidden members,
which require priority 0, can be configured.

**SHARD-021** (Event Driven): WHEN the settings reconfig has succeeded, the
resource SHALL record the resource ID and the settings in state before
promoting or adding members, so a failure there cannot leave an applied
reconfig unrecorded. WHEN the member phase ends, with or without an error, the
resource SHALL read the configuration from the server and write the `member`
state from it before returning (SHARD-018). The SDK persists state on error,
and without this write the state would hold the planned blocks, such as votes
1 for a member the server has at votes 0, and a plan without refresh would
show nothing left to do. A following apply partitions against the live
configuration again and does only what is still missing.

## Staged Adds and Promotion

**SHARD-022** (Ubiquitous): The resource SHALL add a non-arbiter member with
`votes: 0` and `priority: 0` regardless of its block, and, once the member
reports `SECONDARY` (SHARD-016), SHALL apply the block's votes and priority in
a second `replSetReconfig` of its own. A block with votes 0 and priority 0
needs no second reconfig. Arbiters must vote and SHALL be added in their final
form. Before MongoDB 5.0 a newly added voter counts toward majority before it
has synced, so adding a voter directly can leave a set whose voters are online
but cannot elect a primary; staging avoids this on every version and matches
MongoDB's guidance for those versions.

**SHARD-023** (Event Driven): WHEN a `member` block gives a live member more
votes than it has, the Update method SHALL keep the member's live votes and
priority in the settings reconfig, SHALL wait for the member to report
`SECONDARY` (SHARD-016), and SHALL then apply the block's votes and priority
in one `replSetReconfig` per member, before any missing member is added. This
completes an add whose promotion did not fit in an earlier apply, and is the
path for any block that turns a non-voter into a voter. A member that is still
syncing when its wait ends is left as it is and reported as a warning per
SHARD-017. Lowering votes and every other field change go out in the settings
reconfig (SHARD-005).

## Pre-flight of Hosts to Add

**SHARD-027** (Event Driven): WHEN one or more `member` blocks name hosts
that are not in the replica set, the resource SHALL, before sending any
`replSetReconfig`, connect directly to each such host, run `replSetGetStatus`
against it with the provider's credentials and then without authentication,
and decide from the answer. A host that reports NotYetInitialized (code 94) or
InvalidReplicaSetConfig (code 93, a member removed earlier), or that reports
this set with itself in state REMOVED, SHALL be added. A host that reports
NoReplicationEnabled (code 76), a different set name, or this set in any other
state SHALL be refused with an error naming the host and, for a member of this
set, the name the set knows it by. A host that cannot be inspected, because it
is unreachable from the runner or accepts none of the credentials tried, SHALL
be logged and added, with the wait of SHARD-016 as the only check. The
refusal exists for a host that is already a member under another name, such
as an FQDN for a member configured by its short name: the reconfig would give
that node two entries, the node would remove itself until an acceptable
configuration arrives, and in a two-voter set the primary would lose its
majority with no rollback able to run. A fresh node has no users, so an
authenticated probe fails there and reads as uninspectable, while a live
member has replicated users and answers. The probe SHALL be skipped when
`host_override` is set, since the member hosts are then known to be
unreachable from the runner (DISC-008).

## Arbiters, State Order and Duplicate Hosts

**SHARD-024** (Ubiquitous): The resource SHALL send `priority: 0` for a
`member` block with `arbiter_only = true` regardless of the block's `priority`,
and SHALL suppress plan diffs on `priority` for such blocks. The server forces
an arbiter's priority to 0 (a value of 1 is reset to 0 on every supported
version), so the schema default of 1 would otherwise diff against the
read-back on every plan.

**SHARD-025** (Ubiquitous): The `member` state list SHALL follow the order of
the `member` blocks, not the order of the server's configuration, skipping
blocks whose host is not in the set. `member` is a TypeList and is compared
position by position, so a block for a new host placed before a block for an
existing host would otherwise diff on every plan after the add.

**SHARD-026** (Unwanted Behaviour): IF two `member` blocks name the same
host, THEN the resource SHALL return a diagnostic error naming both block
indexes before any command is sent. Otherwise the second block would merge
twice or, for a new host, be added a second time and rejected by the server
after the first add.

## Initialization

The initialization flow hands over to this reconciliation after
`replSetInitiate` of the first member (INIT-010); SHARD-012 through SHARD-027
apply unchanged.
