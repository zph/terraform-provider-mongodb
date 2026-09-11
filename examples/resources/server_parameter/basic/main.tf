terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# mongodb_server_parameter is experimental: enable it here or via
# TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_server_parameter.
provider "mongodb" {
  host             = "127.0.0.1"
  port             = "27017"
  username         = "root"
  password         = var.mongo_password
  auth_database    = "admin"
  features_enabled = ["mongodb_server_parameter"]
}

# value is always a string in HCL. The provider coerces it before
# setParameter: "true"/"false" -> bool, then int, then float, else string.
#
# parameter is immutable. Destroy is state-only: setParameter cannot unset
# a value, so MongoDB keeps whatever was last applied.
resource "mongodb_server_parameter" "cursor_timeout" {
  parameter = "cursorTimeoutMillis"
  value     = "600000"
}

# Reject queries that would scan a whole collection; typical for test environments.
resource "mongodb_server_parameter" "notablescan" {
  parameter = "notablescan"
  value     = "true"
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
