terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
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

# Base role with specific privileges.
resource "mongodb_db_role" "base_role" {
  name     = "base_operations"
  database = "admin"

  privilege {
    db         = "app_db"
    collection = ""
    actions    = ["find", "insert", "update"]
  }
}

# Derived role that inherits from the base role.
# inherited_role references must be in the same database for custom roles.
resource "mongodb_db_role" "extended_role" {
  depends_on = [mongodb_db_role.base_role]
  name       = "extended_operations"
  database   = "admin"

  inherited_role {
    role = mongodb_db_role.base_role.name
    db   = "admin"
  }
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
