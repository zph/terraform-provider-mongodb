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
  features_enabled = ["mongodb_zone_key_range", "mongodb_shard_zone"]
}

resource "mongodb_shard_zone" "east" {
  shard_name = "shard01"
  zone       = "US-East"
}

resource "mongodb_shard_zone" "west" {
  shard_name = "shard02"
  zone       = "US-West"
}

# Cover the whole key space of a hashed shard key { _id: "hashed" } with two
# ranges that meet at 0: $minKey and $maxKey are the absolute bounds, and
# hashed values are signed 64-bit integers, so this splits the data in half.
resource "mongodb_zone_key_range" "orders_east" {
  depends_on = [mongodb_shard_zone.east]

  namespace = "app_db.orders"
  zone      = "US-East"
  min       = jsonencode({ _id = { "$minKey" = 1 } })
  max       = jsonencode({ _id = 0 })
}

resource "mongodb_zone_key_range" "orders_west" {
  depends_on = [mongodb_shard_zone.west]

  namespace = "app_db.orders"
  zone      = "US-West"
  min       = jsonencode({ _id = 0 })
  max       = jsonencode({ _id = { "$maxKey" = 1 } })
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
