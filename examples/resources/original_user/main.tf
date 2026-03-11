terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Bootstrap the original admin user on a fresh MongoDB instance
# running without authentication enabled.
#
# This resource connects WITHOUT auth to create the first user,
# then subsequent resources use the provider's auth credentials.

# The provider block is required but its credentials are unused during
# bootstrap — auth doesn't exist yet. After bootstrap, replace these
# with real credentials matching the user created below.
provider "mongodb" {
  host = "127.0.0.1"
  port = "27017"
}

variable "admin_password" {
  type      = string
  sensitive = true
}

# mongos
# resource "mongodb_original_user" "admin" {
#   host     = "127.0.0.1"
#   port     = "30109"
#   username = "admin"
#   password = var.admin_password
#
#   role {
#     role = "root"
#     db   = "admin"
#   }
# }

resource "mongodb_original_user" "shard_01_admin" {
  host     = "127.0.0.1"
  port     = "27017"
  username = "admin"
  password = var.admin_password

  role {
    role = "root"
    db   = "admin"
  }
}

# resource "mongodb_original_user" "shard_02_admin" {
#   host     = "127.0.0.1"
#   port     = "27018"
#   username = "admin"
#   password = var.admin_password
#
#   role {
#     role = "root"
#     db   = "admin"
#   }
# }
#
