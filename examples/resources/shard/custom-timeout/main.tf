terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

provider "mongodb" {
  host     = "127.0.0.1"
  port     = "30109"
  username = "admin"
  password = var.mongo_password
}

# Large shard with extended drain timeout for removal.
resource "mongodb_shard" "shard01" {
  shard_name          = "shard01"
  hosts               = ["mongo1:27017", "mongo2:27017", "mongo3:27017"]
  remove_timeout_secs = 1800
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
