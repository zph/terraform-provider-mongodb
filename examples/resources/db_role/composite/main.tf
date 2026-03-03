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

# Leaf role: cluster-level monitoring.
resource "mongodb_db_role" "monitoring" {
  name     = "custom_monitoring"
  database = "admin"

  privilege {
    cluster = true
    actions = ["replSetGetStatus", "serverStatus"]
  }
}

# Leaf role: application data access.
resource "mongodb_db_role" "data_access" {
  name     = "custom_data_access"
  database = "admin"

  privilege {
    db         = "orders"
    collection = ""
    actions    = ["find", "insert", "update", "remove", "createIndex"]
  }

  privilege {
    db         = "orders"
    collection = "audit_log"
    actions    = ["find"]
  }
}

# Composite role: inherits both leaf roles and adds its own privilege.
# Demonstrates the full feature set: privileges + inherited_role + depends_on.
resource "mongodb_db_role" "admin_composite" {
  depends_on = [
    mongodb_db_role.monitoring,
    mongodb_db_role.data_access,
  ]
  name     = "admin_composite_role"
  database = "admin"

  privilege {
    db         = "admin"
    collection = ""
    actions    = ["collStats", "dbStats", "listCollections"]
  }

  inherited_role {
    role = mongodb_db_role.monitoring.name
    db   = "admin"
  }

  inherited_role {
    role = mongodb_db_role.data_access.name
    db   = "admin"
  }
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
