terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "~> 0.3"
    }
  }
}

# Full sharded cluster management: mongos for users/roles, direct
# connections to each shard primary for replica set configuration.

# --- Provider: mongos router for user and role management ---
provider "mongodb" {
  host          = var.mongos_host
  port          = var.mongos_port
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"
  ssl           = true
  certificate   = file(pathexpand(var.ca_cert_path))
}

# --- Provider: shard01 primary (direct) ---
provider "mongodb" {
  alias         = "shard01"
  host          = var.shard01_host
  port          = var.shard_port
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"
  ssl           = true
  certificate   = file(pathexpand(var.ca_cert_path))
  direct        = true
}

# --- Provider: shard02 primary (direct) ---
provider "mongodb" {
  alias         = "shard02"
  host          = var.shard02_host
  port          = var.shard_port
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"
  ssl           = true
  certificate   = file(pathexpand(var.ca_cert_path))
  direct        = true
}

# --- Shard configurations ---

resource "mongodb_shard_config" "shard01" {
  provider                  = mongodb.shard01
  shard_name                = "shard01"
  chaining_allowed          = false
  heartbeat_interval_millis = 1000
  heartbeat_timeout_secs    = 10
  election_timeout_millis   = 5000
}

resource "mongodb_shard_config" "shard02" {
  provider                  = mongodb.shard02
  shard_name                = "shard02"
  chaining_allowed          = false
  heartbeat_interval_millis = 1000
  heartbeat_timeout_secs    = 10
  election_timeout_millis   = 5000
}

# --- Roles (created via mongos) ---

resource "mongodb_db_role" "app_readwrite" {
  name     = "app_readwrite"
  database = "admin"

  privilege {
    db         = "app_db"
    collection = ""
    actions    = ["find", "insert", "update", "remove"]
  }
}

resource "mongodb_db_role" "ops_monitoring" {
  name     = "ops_monitoring"
  database = "admin"

  privilege {
    cluster = true
    actions = ["replSetGetStatus", "serverStatus"]
  }

  privilege {
    db         = "admin"
    collection = ""
    actions    = ["collStats", "dbStats"]
  }
}

# --- Users (created via mongos) ---

resource "mongodb_db_user" "app_service" {
  depends_on    = [mongodb_db_role.app_readwrite]
  auth_database = "admin"
  name          = "app_service"
  password      = var.app_password

  role {
    role = mongodb_db_role.app_readwrite.name
    db   = "admin"
  }
}

resource "mongodb_db_user" "ops_monitor" {
  depends_on    = [mongodb_db_role.ops_monitoring]
  auth_database = "admin"
  name          = "ops_monitor"
  password      = var.ops_password

  role {
    role = mongodb_db_role.ops_monitoring.name
    db   = "admin"
  }

  role {
    role = "clusterMonitor"
    db   = "admin"
  }
}

# --- Variables ---

variable "mongos_host" {
  description = "Hostname of the mongos router"
  type        = string
}

variable "shard01_host" {
  description = "Hostname of shard01 primary"
  type        = string
}

variable "shard02_host" {
  description = "Hostname of shard02 primary"
  type        = string
}

variable "mongos_port" {
  description = "Port for mongos"
  type        = string
  default     = "27017"
}

variable "shard_port" {
  description = "Port for shard primaries"
  type        = string
  default     = "27018"
}

variable "mongo_password" {
  type      = string
  sensitive = true
}

variable "app_password" {
  type      = string
  sensitive = true
}

variable "ops_password" {
  type      = string
  sensitive = true
}

variable "ca_cert_path" {
  description = "Path to PEM-encoded CA certificate"
  type        = string
  default     = "~/.mongodb/ca.pem"
}
