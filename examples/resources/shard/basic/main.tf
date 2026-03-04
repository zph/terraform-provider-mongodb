terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Provider must connect to a mongos router for shard operations.
provider "mongodb" {
  host     = "127.0.0.1"
  port     = "30109"
  username = "admin"
  password = var.mongo_password
}

resource "mongodb_shard" "shard01" {
  shard_name = "shard01"
  hosts      = ["mongo1:27017", "mongo2:27017", "mongo3:27017"]
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
