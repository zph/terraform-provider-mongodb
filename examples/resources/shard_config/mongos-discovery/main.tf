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

resource "mongodb_shard_config" "shard01" {
  shard_name                = "shard01"
  chaining_allowed          = true
  heartbeat_interval_millis = 2000
  heartbeat_timeout_secs    = 10
  election_timeout_millis   = 10000
}

resource "mongodb_shard_config" "shard02" {
  shard_name              = "shard02"
  election_timeout_millis = 5000
}

# When internal shard hostnames are unreachable, use host_override
resource "mongodb_shard_config" "shard03" {
  shard_name    = "shard03"
  host_override = "shard03-external.example.com:27018"
}
