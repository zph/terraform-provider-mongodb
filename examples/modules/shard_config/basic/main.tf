terraform {
  required_version = ">= 1.5.7"

  required_providers {
    mongodb = {
      source = "registry.terraform.io/Kaginari/mongodb"
      version = "9.9.9"
    }
  }
}

provider "mongodb" {
  username = "admin"
  password = "admin"

  host = "localhost"
  port = "27019"
}

resource "mongodb_shard_config" "shard01" {
  shard_name = "shard01"
  chaining_allowed = false
  election_timeout_millis = 222
}
