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

# Shard-level defaults; reference in each resource to avoid duplication.
locals {
  shard_config_defaults = {
    chaining_allowed          = false
    heartbeat_interval_millis = 1000
    heartbeat_timeout_secs    = 10
    election_timeout_millis   = 5000
  }
}

resource "mongodb_shard_config" "shard01" {
  provider                  = mongodb.shard01
  shard_name                = "shard01"
  chaining_allowed          = local.shard_config_defaults.chaining_allowed
  election_timeout_millis   = local.shard_config_defaults.election_timeout_millis
  heartbeat_interval_millis = local.shard_config_defaults.heartbeat_interval_millis
  heartbeat_timeout_secs    = local.shard_config_defaults.heartbeat_timeout_secs
}

resource "mongodb_shard_config" "shard02" {
  provider                  = mongodb.shard02
  shard_name                = "shard02"
  chaining_allowed          = local.shard_config_defaults.chaining_allowed
  election_timeout_millis   = local.shard_config_defaults.election_timeout_millis
  heartbeat_interval_millis = local.shard_config_defaults.heartbeat_interval_millis
  heartbeat_timeout_secs    = local.shard_config_defaults.heartbeat_timeout_secs
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
