terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Provider must connect to a mongos router.
# mongodb_collection_balancing is experimental: enable it here or via
# TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_collection_balancing.
provider "mongodb" {
  host             = "127.0.0.1"
  port             = "30109"
  username         = "admin"
  password         = var.mongo_password
  features_enabled = ["mongodb_collection_balancing"]
}

# Pin one sharded collection in place while the balancer keeps running for
# the rest of the cluster. namespace is db.collection and is immutable.
# Destroy re-enables balancing for the collection.
resource "mongodb_collection_balancing" "users" {
  namespace = "app_db.users"
  enabled   = false
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
