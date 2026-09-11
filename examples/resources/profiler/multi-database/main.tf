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
  port             = "27017"
  username         = "root"
  password         = var.mongo_password
  auth_database    = "admin"
  features_enabled = ["mongodb_profiler"]
}

# One profiler setting per database. Profiling is per-database in MongoDB,
# so a database that is not listed here is left as it is.
locals {
  profiled_databases = {
    app_db    = { level = 1, slowms = 200 }
    analytics = { level = 1, slowms = 1000 }
    # Everything, for a short debugging window; level 2 is expensive.
    staging = { level = 2, slowms = 0 }
  }
}

resource "mongodb_profiler" "this" {
  for_each = local.profiled_databases

  database = each.key
  level    = each.value.level
  slowms   = each.value.slowms

  # ratelimit (default 1) samples 1/N of slow operations and is honoured by
  # Percona Server for MongoDB only; stock MongoDB ignores it.
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
