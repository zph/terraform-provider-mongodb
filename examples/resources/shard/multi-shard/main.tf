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

resource "mongodb_shard" "shard01" {
  shard_name = "shard01"
  hosts      = ["shard01-a:27018", "shard01-b:27018", "shard01-c:27018"]
}

resource "mongodb_shard" "shard02" {
  shard_name = "shard02"
  hosts      = ["shard02-a:27018", "shard02-b:27018", "shard02-c:27018"]
}

resource "mongodb_shard" "shard03" {
  shard_name          = "shard03"
  hosts               = ["shard03-a:27018", "shard03-b:27018", "shard03-c:27018"]
  remove_timeout_secs = 600
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
