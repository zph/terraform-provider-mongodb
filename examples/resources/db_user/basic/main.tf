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

# Single user with one built-in role.
resource "mongodb_db_user" "app_reader" {
  auth_database = "app_db"
  name          = "app_reader"
  password      = var.app_reader_password

  role {
    role = "read"
    db   = "app_db"
  }
}

variable "mongo_password" {
  type      = string
  sensitive = true
}

variable "app_reader_password" {
  type      = string
  sensitive = true
}
