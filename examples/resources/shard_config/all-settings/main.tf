terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Connect directly to the shard's primary for replSetReconfig.
# use no-auth option by omitting username and password
provider "mongodb" {
  host          = "localhost"
  port          = "30103"
  auth_database = "admin"
}

# All configurable shard settings explicitly set.
#
# Known limitations:
# - Delete is a no-op: destroying this resource removes it from state
#   but does not reset the MongoDB replica set configuration.
resource "mongodb_shard_config" "shard1" {
  shard_name                = "shard1"
  chaining_allowed          = true
  heartbeat_interval_millis = 1000
  heartbeat_timeout_secs    = 10
  election_timeout_millis   = 10000

  # Per-member configuration: identified by host:port
  member {
    host          = "localhost:30103"
    priority      = 2
    votes         = 1
    hidden        = false
    arbiter_only  = false
    build_indexes = true
    tags = {
      dc   = "us-east"
      rack = "r1"
      zone = "primary"
    }
  }

  member {
    host     = "localhost:30104"
    priority = 1
    votes    = 1
    tags = {
      dc   = "us-east"
      rack = "r2"
      zone = "secondary"
    }
  }

  member {
    host     = "localhost:30105"
    priority = 2
    votes    = 1
    tags = {
      dc   = "us-TEST"
      rack = "r3"
      zone = "secondary"
    }
  }
}
