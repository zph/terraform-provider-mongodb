terraform {
  required_version = ">= 1.7.5"

  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

provider "mongodb" {
  username = "admin"
  password = "admin"

  host = "localhost"
  port = "27019"

  # mongodb_shard_config is experimental; the env var
  # TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_shard_config works as well.
  features_enabled = ["mongodb_shard_config"]
}

resource "mongodb_shard_config" "shard01" {
  shard_name              = "shard01"
  chaining_allowed        = false
  election_timeout_millis = 222
}
