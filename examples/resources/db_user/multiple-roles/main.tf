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

# User with multiple built-in roles across different databases.
# The role set supports up to 25 entries.
resource "mongodb_db_user" "backend_service" {
  auth_database = "admin"
  name          = "backend_svc"
  password      = var.backend_password

  role {
    role = "readWrite"
    db   = "orders"
  }

  role {
    role = "readWrite"
    db   = "inventory"
  }

  role {
    role = "read"
    db   = "analytics"
  }

  role {
    role = "clusterMonitor"
    db   = "admin"
  }
}

variable "mongo_password" {
  type      = string
  sensitive = true
}

variable "backend_password" {
  type      = string
  sensitive = true
}
