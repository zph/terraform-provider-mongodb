# Shard Member Configuration Requirements

This document defines EARS requirements for per-member configuration of
replica set members via the `mongodb_shard_config` Terraform resource.

## Schema

**SHARD-001** (Ubiquitous): The `mongodb_shard_config` resource schema SHALL
define an Optional `member` block of TypeList containing sub-fields: `host`
(Required, TypeString), `tags` (Optional, TypeMap of TypeString), `priority`
(Optional, TypeInt), `votes` (Optional, TypeInt), `hidden` (Optional, TypeBool),
`arbiter_only` (Optional, TypeBool), and `build_indexes` (Optional, TypeBool,
Default true).

## Backward Compatibility

**SHARD-002** (Event Driven): WHEN the `member` block is omitted from the HCL
configuration, the `mongodb_shard_config` resource SHALL preserve the existing
behavior and leave all replica set members untouched.

## Host-Based Member Lookup

**SHARD-003** (Event Driven): WHEN the Update method processes a `member` block,
the resource SHALL locate the matching `ConfigMember` in the fetched
`RSConfig.Members` by comparing the `host` field (case-sensitive exact string
match including port).

**SHARD-004** (Unwanted Behaviour): IF a `member` block references a `host`
that does not match any member in the current `RSConfig.Members`, THEN the
Update method SHALL return a diagnostic error identifying the unmatched host.

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
