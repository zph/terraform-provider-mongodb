# Mongos auto-discovery example
#
# Connect to a mongos router with a single provider block.
# Each mongodb_shard_config resource auto-discovers its shard via listShards
# and creates a temporary direct connection for replSetReconfig.
#
# No provider aliases needed — one provider manages all shards.

terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = ">= 9.9.9"
    }
  }
}

provider "mongodb" {
  host     = "mongos.example.com"
  port     = "27017"
  username = "admin"
  password = "secret"
}

# Shard-level defaults; per-shard overrides merge in.
locals {
  shard_config_defaults = {
    chaining_allowed          = true
    heartbeat_interval_millis = 1000
    heartbeat_timeout_secs    = 10
    election_timeout_millis   = 10000
  }
  shard_overrides = {
    shard01 = {
      heartbeat_interval_millis = 2000
    }
    shard02 = {
      election_timeout_millis = 5000
    }
    shard03 = {
      # When internal shard hostnames are unreachable, use host_override on the resource
      election_timeout_millis = 5000
    }
  }
  shard_configs = { for k, overrides in local.shard_overrides : k => merge(local.shard_config_defaults, overrides) }
}

resource "mongodb_shard_config" "shards" {
  for_each = local.shard_configs

  shard_name                = each.key
  chaining_allowed          = each.value.chaining_allowed
  heartbeat_interval_millis = each.value.heartbeat_interval_millis
  heartbeat_timeout_secs    = each.value.heartbeat_timeout_secs
  election_timeout_millis   = each.value.election_timeout_millis

  # Optional: when internal shard hostnames are unreachable from this host
  # host_override = each.key == "shard03" ? "shard03-external.example.com:27018" : null
}

# User created via mongos — valid for all connections through this mongos (all shards).
resource "mongodb_db_user" "app_service" {
  auth_database = "admin"
  name          = "app_service"
  password      = var.app_password

  role {
    role = "readWrite"
    db   = "app_db"
  }
}

variable "app_password" {
  type      = string
  sensitive = true
}
