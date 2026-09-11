terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

provider "mongodb" {
  host             = "127.0.0.1"
  port             = "30109"
  username         = "admin"
  password         = var.mongo_password
  features_enabled = ["mongodb_shard"]
}

# Both timeouts are client-side only and can be changed without any
# MongoDB operation.
resource "mongodb_shard" "shard01" {
  shard_name = "shard01"
  hosts      = ["mongo1:27017", "mongo2:27017", "mongo3:27017"]

  # addShard is retried until the mongos has discovered the replica set
  # primary, which can lag right after replSetInitiate (default 60).
  add_timeout_secs = 180

  # removeShard drains every chunk off the shard before it completes;
  # large shards need well over the default 300.
  remove_timeout_secs = 1800
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
