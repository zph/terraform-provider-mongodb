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
  features_enabled = ["mongodb_shard_zone"]
}

# A shard may belong to several zones and a zone may span several shards.
locals {
  zone_assignments = [
    { shard = "shard01", zone = "US-East" },
    { shard = "shard01", zone = "Backup" },
    { shard = "shard02", zone = "US-West" },
    { shard = "shard02", zone = "Backup" },
  ]
}

# The for_each key doubles as the import ID (shard_name:zone).
resource "mongodb_shard_zone" "this" {
  for_each = { for a in local.zone_assignments : "${a.shard}:${a.zone}" => a }

  shard_name = each.value.shard
  zone       = each.value.zone
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
