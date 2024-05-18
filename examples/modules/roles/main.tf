terraform {
  required_version = ">= 0.13"

  required_providers {
    mongodb = {
      source = "registry.terraform.io/Kaginari/mongodb"
      version = "9.9.9"
    }
  }
}

resource "mongodb_db_role" "role" {
  name = "custom_role_test"
  privilege {
    db = "admin"
    collection = "*"
    actions = ["collStats"]
  }
  privilege {
    db = "ds"
    collection = "*"
    actions = ["collStats"]
  }
}

resource "mongodb_db_role" "role_2" {
  depends_on = [mongodb_db_role.role]
  database = "admin"
  name = "new_role3"
  inherited_role {
    role = mongodb_db_role.role.name
    db =   "admin"
  }
  privilege {
    db = "not_inhireted"
    collection = "*"
    actions = ["collStats"]
  }
}
resource "mongodb_db_role" "role4" {
  depends_on = [mongodb_db_role.role]
  database = "exemple"
  name = "new_role4"
}
