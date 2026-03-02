terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "~> 0.3"
    }
  }
}

provider "mongodb" {
  host          = "127.0.0.1"
  port          = "27017"
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"
}

# Cluster-level privilege role for replica set failover operations.
# When cluster=true, db and collection are ignored by MongoDB.
resource "mongodb_db_role" "failover_operator" {
  name     = "failover_operator"
  database = "admin"

  privilege {
    cluster = true
    actions = ["replSetGetConfig", "replSetGetStatus", "replSetStateChange"]
  }
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
