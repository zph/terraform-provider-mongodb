terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Provider must connect to a mongos router.
# mongodb_zone_key_range is experimental: enable it here or via
# TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_zone_key_range.
provider "mongodb" {
  host             = "127.0.0.1"
  port             = "30109"
  username         = "admin"
  password         = var.mongo_password
  features_enabled = ["mongodb_zone_key_range", "mongodb_shard_zone"]
}

# A zone has to be assigned to at least one shard before a key range can
# point at it, so the ranges depend on the mongodb_shard_zone resources.
resource "mongodb_shard_zone" "east" {
  shard_name = "shard01"
  zone       = "US-East"
}

resource "mongodb_shard_zone" "west" {
  shard_name = "shard02"
  zone       = "US-West"
}

# Shard key { region: 1, user_id: 1 }: each zone owns every document of one
# region. min is inclusive, max exclusive, both as JSON with the same fields
# as the shard key. All four fields are immutable and ranges may not overlap.
#
# Field order in min/max must follow the shard key. jsonencode sorts keys
# alphabetically, so write the JSON as a literal string when that order differs.
resource "mongodb_zone_key_range" "orders_east" {
  depends_on = [mongodb_shard_zone.east]

  namespace = "app_db.orders"
  zone      = "US-East"
  min       = jsonencode({ region = "us-east", user_id = { "$minKey" = 1 } })
  max       = jsonencode({ region = "us-east", user_id = { "$maxKey" = 1 } })
}

resource "mongodb_zone_key_range" "orders_west" {
  depends_on = [mongodb_shard_zone.west]

  namespace = "app_db.orders"
  zone      = "US-West"
  min       = jsonencode({ region = "us-west", user_id = { "$minKey" = 1 } })
  max       = jsonencode({ region = "us-west", user_id = { "$maxKey" = 1 } })
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
