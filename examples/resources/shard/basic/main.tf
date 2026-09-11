terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Provider must connect to a mongos router for shard operations.
# mongodb_shard is experimental: enable it here or via
# TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_shard.
provider "mongodb" {
  host             = "127.0.0.1"
  port             = "30109"
  username         = "admin"
  password         = var.mongo_password
  features_enabled = ["mongodb_shard"]
}

# shard_name and hosts are immutable; the replica set must already be
# initialized (see the shard_config examples) before addShard runs.
resource "mongodb_shard" "shard01" {
  shard_name = "shard01"
  hosts      = ["mongo1:27017", "mongo2:27017", "mongo3:27017"]
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
