terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Connect directly to the shard's primary for replSetReconfig.
provider "mongodb" {
  host          = "shard01-primary.example.com"
  port          = "27018"
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"
  direct        = true
}

# All configurable shard settings explicitly set.
#
# Known limitations:
# - Delete is a no-op: destroying this resource removes it from state
#   but does not reset the MongoDB replica set configuration.
resource "mongodb_shard_config" "shard01" {
  shard_name                = "shard01"
  chaining_allowed          = true
  heartbeat_interval_millis = 2000
  heartbeat_timeout_secs    = 10
  election_timeout_millis   = 10000

  # Per-member configuration: identified by host:port
  member {
    host          = "shard01-primary.example.com:27018"
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
    host     = "shard01-secondary1.example.com:27018"
    priority = 1
    votes    = 1
    tags = {
      dc   = "us-east"
      rack = "r2"
      zone = "secondary"
    }
  }

  member {
    host     = "shard01-secondary2.example.com:27018"
    priority = 1
    votes    = 1
    tags = {
      dc   = "us-west"
      rack = "r3"
      zone = "secondary"
    }
  }
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
