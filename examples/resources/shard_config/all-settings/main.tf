terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Connect to mongos (or a single shard's primary). For direct connect to each shard,
# add provider aliases and set provider = mongodb[each.key] on the resource.
provider "mongodb" {
  host          = "localhost"
  port          = "30109"
  auth_database = "admin"
}

# Shard-level and member-level defaults; per-shard and per-member overrides merge in.
locals {
  shard_config_defaults = {
    chaining_allowed          = true
    heartbeat_interval_millis = 1000
    heartbeat_timeout_secs    = 10
    election_timeout_millis   = 10000
  }
  member_defaults = {
    priority      = 1
    votes         = 1
    hidden        = false
    arbiter_only  = false
    build_indexes = true
    tags          = {}
  }
  analytical_node_defaults = {
    priority = 0.1
    tags = {
      nodeType = "analytical"
    }
  }
  online_node_defaults = {
    priority = 1
    tags = {
      nodeType = "online"
    }
  }
  # One entry per shard: optional shard-level overrides + members list.
  shards = {
    shard1 = {
      members = [
        merge(local.member_defaults, local.online_node_defaults, { host = "localhost:30103" }),
        merge(local.member_defaults, local.analytical_node_defaults, { host = "localhost:30104" }),
        merge(local.member_defaults, local.online_node_defaults, { host = "localhost:30105" }),
      ]
    }
    shard2 = {
      members = [
        merge(local.member_defaults, local.online_node_defaults, { host = "localhost:30206" }),
        merge(local.member_defaults, local.analytical_node_defaults, { host = "localhost:30207" }),
        merge(local.member_defaults, local.analytical_node_defaults, { host = "localhost:30208" }),
      ]
      election_timeout_millis = 5000
    }
  }
  # Full config per shard: shard defaults + per-shard overrides (including members).
  shard_configs = { for name, shard in local.shards : name => merge(local.shard_config_defaults, shard) }
}

# All configurable shard settings explicitly set.
#
# Known limitations:
# - Delete is a no-op: destroying this resource removes it from state
#   but does not reset the MongoDB replica set configuration.
resource "mongodb_shard_config" "shards" {
  for_each = local.shard_configs

  shard_name                = each.key
  chaining_allowed          = each.value.chaining_allowed
  heartbeat_interval_millis = each.value.heartbeat_interval_millis
  heartbeat_timeout_secs    = each.value.heartbeat_timeout_secs
  election_timeout_millis   = each.value.election_timeout_millis

  dynamic "member" {
    for_each = each.value.members
    content {
      host          = member.value.host
      priority      = member.value.priority
      votes         = member.value.votes
      hidden        = member.value.hidden
      arbiter_only  = member.value.arbiter_only
      build_indexes = member.value.build_indexes
      tags          = member.value.tags
    }
  }
}
