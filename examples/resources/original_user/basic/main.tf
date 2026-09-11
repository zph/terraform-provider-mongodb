terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Bootstrap the original admin user on a fresh MongoDB instance that has no
# users yet. The resource connects WITHOUT auth through MongoDB's localhost
# exception, so it carries its own host/port instead of using the provider
# connection. After this apply, point the provider at the credentials
# created here.
#
# Updates are refused, and destroy only removes the resource from state:
# the user is never dropped, since that would lock out the cluster.
# If the user already exists and the credentials authenticate, the resource
# adopts it instead of failing.

# The provider block is required but no credentials exist yet, so none are given.
provider "mongodb" {
  host = "127.0.0.1"
  port = "27017"
}

variable "admin_password" {
  type      = string
  sensitive = true
}

# Every replica set keeps its own users: in a sharded cluster declare one
# resource for the mongos (config servers) and one per shard primary.
resource "mongodb_original_user" "shard01_admin" {
  host     = "127.0.0.1"
  port     = "27017"
  username = "admin"

  # Optional. Leave it out and export MONGODB_ORIGINAL_USER_PASSWORD instead
  # to keep the password out of Terraform state.
  password = var.admin_password

  # replica_set is auto-discovered; set it only if discovery is not wanted.

  # Defaults to root on admin when no role block is given.
  role {
    role = "root"
    db   = "admin"
  }
}
