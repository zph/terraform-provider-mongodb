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
  features_enabled = ["mongodb_balancer_config"]
}

# Balance only during a nightly window, with smaller chunks and conservative
# migrations.
resource "mongodb_balancer_config" "this" {
  enabled = true

  # HH:MM, 24-hour. Both bounds must be set together.
  active_window_start = "01:00"
  active_window_stop  = "05:00"

  # Default chunk size for the cluster, 1-1024 MB.
  chunk_size_mb = 64

  # _secondaryThrottle write concern for chunk migrations.
  secondary_throttle = "majority"

  # Wait for the source shard to delete migrated chunks before the next migration.
  wait_for_delete = true
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
