terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "~> 0.3"
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
# - Read only retrieves shard_name: settings drift is not detected.
resource "mongodb_shard_config" "shard01" {
  shard_name                = "shard01"
  chaining_allowed          = true
  heartbeat_interval_millis = 2000
  heartbeat_timeout_secs    = 10
  election_timeout_millis   = 10000
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
