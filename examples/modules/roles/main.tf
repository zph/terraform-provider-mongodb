terraform {
  required_version = ">= 1.5.7"

  required_providers {
    mongodb = {
      source = "registry.terraform.io/Kaginari/mongodb"
      version = "9.9.9"
    }
  }
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
