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

# Layered role hierarchy: base -> mid-tier -> admin.
# Each layer adds privileges on top of what it inherits.

# Layer 1: base read-only access to application data.
resource "mongodb_db_role" "viewer" {
  name     = "app_viewer"
  database = "admin"

  privilege {
    db         = "app_db"
    collection = ""
    actions    = ["find", "listCollections"]
  }

  privilege {
    db         = "app_db"
    collection = ""
    actions    = ["collStats", "dbStats"]
  }
}

# Layer 2: editor inherits viewer, adds write operations.
resource "mongodb_db_role" "editor" {
  depends_on = [mongodb_db_role.viewer]
  name       = "app_editor"
  database   = "admin"

  inherited_role {
    role = mongodb_db_role.viewer.name
    db   = "admin"
  }

  privilege {
    db         = "app_db"
    collection = ""
    actions    = ["insert", "update", "remove"]
  }
}

# Layer 3: admin inherits editor, adds index and collection management.
resource "mongodb_db_role" "admin" {
  depends_on = [mongodb_db_role.editor]
  name       = "app_admin"
  database   = "admin"

  inherited_role {
    role = mongodb_db_role.editor.name
    db   = "admin"
  }

  privilege {
    db         = "app_db"
    collection = ""
    actions    = ["createIndex", "dropIndex", "createCollection", "dropCollection"]
  }
}

# Assign one user at each tier to demonstrate the hierarchy.
resource "mongodb_db_user" "viewer_user" {
  depends_on    = [mongodb_db_role.viewer]
  auth_database = "admin"
  name          = "viewer_user"
  password      = var.viewer_password

  role {
    role = mongodb_db_role.viewer.name
    db   = "admin"
  }
}

resource "mongodb_db_user" "editor_user" {
  depends_on    = [mongodb_db_role.editor]
  auth_database = "admin"
  name          = "editor_user"
  password      = var.editor_password

  role {
    role = mongodb_db_role.editor.name
    db   = "admin"
  }
}

resource "mongodb_db_user" "admin_user" {
  depends_on    = [mongodb_db_role.admin]
  auth_database = "admin"
  name          = "admin_user"
  password      = var.admin_password

  role {
    role = mongodb_db_role.admin.name
    db   = "admin"
  }
}

variable "mongo_password" {
  type      = string
  sensitive = true
}

variable "viewer_password" {
  type      = string
  sensitive = true
}

variable "editor_password" {
  type      = string
  sensitive = true
}

variable "admin_password" {
  type      = string
  sensitive = true
}
