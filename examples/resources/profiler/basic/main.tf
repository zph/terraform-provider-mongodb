terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# mongodb_profiler is experimental: enable it here or via
# TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_profiler.
provider "mongodb" {
  host             = "127.0.0.1"
  port             = "27017"
  username         = "root"
  password         = var.mongo_password
  auth_database    = "admin"
  features_enabled = ["mongodb_profiler"]
}

# Capture operations slower than 200ms on one database.
# level: 0 = off, 1 = slow operations only, 2 = all operations.
# database is immutable; destroy sets the level back to 0.
resource "mongodb_profiler" "app_db" {
  database = "app_db"
  level    = 1
  slowms   = 200
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
