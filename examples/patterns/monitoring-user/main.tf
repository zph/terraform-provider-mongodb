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

# Least-privilege role for a Prometheus/Datadog MongoDB exporter.
# Grants only the actions needed for metrics collection.
resource "mongodb_db_role" "exporter" {
  name     = "metrics_exporter"
  database = "admin"

  # Cluster-level metrics (serverStatus, replSetGetStatus, etc.)
  privilege {
    cluster = true
    actions = ["serverStatus", "replSetGetStatus"]
  }

  # Database-level stats across all databases.
  privilege {
    db         = ""
    collection = ""
    actions    = ["dbStats", "collStats", "indexStats"]
  }

  # Access to the oplog for replication lag monitoring.
  privilege {
    db         = "local"
    collection = "oplog.rs"
    actions    = ["find"]
  }
}

resource "mongodb_db_user" "exporter" {
  depends_on    = [mongodb_db_role.exporter]
  auth_database = "admin"
  name          = "mongodb_exporter"
  password      = var.exporter_password

  role {
    role = mongodb_db_role.exporter.name
    db   = "admin"
  }

  # clusterMonitor is a built-in role that supplements custom metrics.
  role {
    role = "clusterMonitor"
    db   = "admin"
  }
}

variable "mongo_password" {
  type      = string
  sensitive = true
}

variable "exporter_password" {
  description = "Password for the metrics exporter MongoDB user"
  type        = string
  sensitive   = true
}
