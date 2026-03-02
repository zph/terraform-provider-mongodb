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

# Simple privilege-based role granting read access with collection stats.
resource "mongodb_db_role" "analyst" {
  name     = "analyst_role"
  database = "admin"

  privilege {
    db         = "analytics"
    collection = ""
    actions    = ["find", "collStats", "dbStats", "listCollections"]
  }
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
