terraform {
  required_version = ">= 1.5.7"

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
  database = "example"
  name = "new_role4"
}


resource "mongodb_db_role" "failover_role" {
  name = "FailoversAndReplSetManagerRole"
  database = "admin"
  privilege {
    cluster = true
    actions = [
      "replSetGetConfig",
      "replSetGetStatus",
      "replSetStateChange",
    ]
  }
}

resource "mongodb_db_role" "staff_role" {
  database = "admin"
  name = "StaffRole"
  inherited_role {
    role = "clusterMonitor"
    db = "admin"
  }

  privilege {
    db = "*"
    collection = "*"
    actions = ["collStats"]
  }
}

locals {
  admin_roles = toset(["clusterAdmin" , "clusterManager" , "clusterMonitor"])
}
resource "mongodb_db_role" "staff_administrator_role" {
  depends_on = [mongodb_db_role.staff_role]
  database = "admin"
  name = "StaffAdministratorRole"
  inherited_role {
    role = "clusterAdmin"
    db = "admin"
  }
  inherited_role {
    role = "clusterManager"
    db = "admin"
  }

  inherited_role {
    role = "clusterMonitor"
    db = "admin"
  }
  inherited_role {
    role = mongodb_db_role.staff_role.name
    db = "admin"
  }
  inherited_role {
    role = "readWriteAnyDatabase"
    db = "admin"
  }
}
