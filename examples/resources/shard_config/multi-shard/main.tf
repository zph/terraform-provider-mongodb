terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Multiple shards require separate provider instances via aliases,
# since each shard's replSetReconfig must target that shard's primary.

provider "mongodb" {
  alias         = "shard01"
  host          = "shard01-primary.example.com"
  port          = "27018"
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"
  direct        = true
}

provider "mongodb" {
  alias         = "shard02"
  host          = "shard02-primary.example.com"
  port          = "27018"
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"
  direct        = true
}

resource "mongodb_shard_config" "shard01" {
  provider                  = mongodb.shard01
  shard_name                = "shard01"
  chaining_allowed          = false
  election_timeout_millis   = 5000
  heartbeat_interval_millis = 1000
  heartbeat_timeout_secs    = 10
}

resource "mongodb_shard_config" "shard02" {
  provider                  = mongodb.shard02
  shard_name                = "shard02"
  chaining_allowed          = false
  election_timeout_millis   = 5000
  heartbeat_interval_millis = 1000
  heartbeat_timeout_secs    = 10
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
