# Shard Auto-Discovery Requirements

This document defines EARS requirements for auto-discovering shard topology
from a mongos connection and creating temporary direct connections to
individual shard replica sets.

## Topology Detection

**DISC-001** (Event Driven): WHEN connected to a mongos instance
(isMaster response `msg` equals `"isdbgrid"`), the `mongodb_shard_config`
resource SHALL run `listShards` to discover all shard connection strings.

**DISC-002** (Event Driven): WHEN `listShards` returns a shard whose `_id`
matches the resource's `shard_name`, the resource SHALL parse the shard's
`host` field (format `"rsName/host1:port,host2:port"`) to extract the
replica set name and member addresses.

## Temporary Connection

**DISC-003** (Ubiquitous): The resource SHALL create a temporary direct
connection to the first host in the parsed shard host string, inheriting the
provider's authentication credentials and TLS configuration.

**DISC-004** (Event Driven): WHEN a temporary shard connection is
established, the resource SHALL execute `replSetGetConfig` and
`replSetReconfig` against the temporary connection instead of the provider's
mongos connection.

## Error Handling

**DISC-005** (Unwanted Behaviour): IF the `shard_name` is not found in the
`listShards` response, the resource SHALL return a diagnostic error listing
all available shard names.

## Direct RS Connection

**DISC-006** (Event Driven): WHEN connected directly to a replica set member
(isMaster response contains a non-empty `setName`), the resource SHALL use
the provider connection directly without running `listShards`.

## Cleanup

**DISC-007** (Ubiquitous): The resource SHALL disconnect any temporary client
created for shard access after the CRUD operation completes.

## Host Override

**DISC-008** (Event Driven): WHERE the `host_override` attribute is set on
the resource, the resource SHALL use the `host_override` value instead of
the first host parsed from `listShards`.

**DISC-009** (Unwanted Behaviour): IF `host_override` is set but cannot be
parsed as a valid `host:port`, the resource SHALL return a diagnostic error.

## Security

**DISC-010** (Ubiquitous): The resource SHALL reuse the provider's TLS
configuration and proxy settings for all temporary shard connections.
