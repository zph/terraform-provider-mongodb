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
  features_enabled = ["mongodb_collection_balancing"]
}

# Per-collection chunk size override. Honoured on MongoDB 6.0+ through
# configureCollectionBalancing; older versions ignore it with a warning
# and only apply enabled. Destroy resets it to the cluster default.
resource "mongodb_collection_balancing" "logs" {
  namespace     = "app_db.logs"
  enabled       = true
  chunk_size_mb = 256
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
