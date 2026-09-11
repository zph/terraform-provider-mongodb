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
  features_enabled = ["mongodb_server_parameter"]
}

# Some parameters accept a config string on write but return a different
# document on read; wiredTigerEngineRuntimeConfig is the usual case.
# ignore_read skips getParameter and trusts the configured value, so the
# resource never shows drift for them.
resource "mongodb_server_parameter" "wt_runtime" {
  parameter   = "wiredTigerEngineRuntimeConfig"
  value       = "cache_size=2G"
  ignore_read = true
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
