terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Provider must connect to a mongos router.
# mongodb_shard_zone is experimental: enable it here or via
# TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_shard_zone.
provider "mongodb" {
  host             = "127.0.0.1"
  port             = "30109"
  username         = "admin"
  password         = var.mongo_password
  features_enabled = ["mongodb_shard_zone"]
}

# One resource per shard/zone pair (addShardToZone). Both fields are
# immutable. Destroy runs removeShardFromZone and leaves any key ranges
# that point at the zone in place; see the zone_key_range examples.
resource "mongodb_shard_zone" "shard01_east" {
  shard_name = "shard01"
  zone       = "US-East"
}

resource "mongodb_shard_zone" "shard02_west" {
  shard_name = "shard02"
  zone       = "US-West"
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
