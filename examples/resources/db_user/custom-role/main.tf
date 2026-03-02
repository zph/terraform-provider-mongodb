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

# Create a custom role first, then assign it to a user.
# depends_on ensures the role exists before the user is created.
resource "mongodb_db_role" "app_readwrite" {
  name     = "app_readwrite_role"
  database = "admin"

  privilege {
    db         = "app_db"
    collection = ""
    actions    = ["find", "insert", "update", "remove"]
  }
}

resource "mongodb_db_user" "app_user" {
  depends_on    = [mongodb_db_role.app_readwrite]
  auth_database = "admin"
  name          = "app_user"
  password      = var.app_password

  role {
    role = mongodb_db_role.app_readwrite.name
    db   = "admin"
  }

  role {
    role = "read"
    db   = "config"
  }
}

variable "mongo_password" {
  type      = string
  sensitive = true
}

variable "app_password" {
  type      = string
  sensitive = true
}
