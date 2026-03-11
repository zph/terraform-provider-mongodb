terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Full sharded cluster setup from scratch: bootstrap admin users on each
# component, configure replica sets with full membership, add shards to
# the cluster, enable the balancer, and create application roles and users.
#
# Topology:
#   - 1 config server RS (configRS) — 3 members
#   - 2 shard RSs (shard01, shard02) — 3 members each
#   - 1 mongos router
#
# Execution order (enforced by depends_on):
#   1. Bootstrap admin users on every component (localhost exception)
#   2. Configure replica set membership and settings on each shard
#   3. Add shards to the cluster via mongos
#   4. Configure the balancer
#   5. Create application roles and users
#
# Prerequisites:
#   - All mongod/mongos processes started with --auth and --keyFile
#   - Config server RS and shard RSs already initialized (replSetInitiate)
#   - Mongos started with --configdb pointing to the config server RS
#   - No users exist yet (localhost exception allows bootstrap)

# ──────────────────────────────────────────────────────────────────────
# Variables
# ──────────────────────────────────────────────────────────────────────

variable "admin_password" {
  description = "Password for the admin user bootstrapped on every component"
  type        = string
  sensitive   = true
}

variable "app_password" {
  description = "Password for the application service user"
  type        = string
  sensitive   = true
}

variable "mongos_host" {
  description = "Hostname of the mongos router"
  type        = string
  default     = "127.0.0.1"
}

variable "mongos_port" {
  description = "Port of the mongos router"
  type        = string
  default     = "27017"
}

# Config server members
variable "configsvr_members" {
  description = "Config server RS members (first entry is the primary candidate)"
  type = list(object({
    host = string
    port = string
  }))
  default = [
    { host = "configsvr0.example.com", port = "27019" },
    { host = "configsvr1.example.com", port = "27019" },
    { host = "configsvr2.example.com", port = "27019" },
  ]
}

# Shard01 members
variable "shard01_members" {
  description = "Shard01 RS members (first entry is the primary candidate)"
  type = list(object({
    host = string
    port = string
  }))
  default = [
    { host = "shard01a.example.com", port = "27018" },
    { host = "shard01b.example.com", port = "27018" },
    { host = "shard01c.example.com", port = "27018" },
  ]
}

# Shard02 members
variable "shard02_members" {
  description = "Shard02 RS members (first entry is the primary candidate)"
  type = list(object({
    host = string
    port = string
  }))
  default = [
    { host = "shard02a.example.com", port = "27018" },
    { host = "shard02b.example.com", port = "27018" },
    { host = "shard02c.example.com", port = "27018" },
  ]
}

# ──────────────────────────────────────────────────────────────────────
# Phase 1: Bootstrap admin users via localhost exception
# ──────────────────────────────────────────────────────────────────────
# Each component needs its own admin user created before auth is usable.
# original_user connects without auth via the localhost exception.
# Bootstrap targets the first member (primary candidate) of each RS.

resource "mongodb_original_user" "configsvr_admin" {
  host     = var.configsvr_members[0].host
  port     = var.configsvr_members[0].port
  username = "admin"
  password = var.admin_password

  role {
    role = "root"
    db   = "admin"
  }
}

resource "mongodb_original_user" "shard01_admin" {
  host     = var.shard01_members[0].host
  port     = var.shard01_members[0].port
  username = "admin"
  password = var.admin_password

  role {
    role = "root"
    db   = "admin"
  }
}

resource "mongodb_original_user" "shard02_admin" {
  host     = var.shard02_members[0].host
  port     = var.shard02_members[0].port
  username = "admin"
  password = var.admin_password

  role {
    role = "root"
    db   = "admin"
  }
}

resource "mongodb_original_user" "mongos_admin" {
  host     = var.mongos_host
  port     = var.mongos_port
  username = "admin"
  password = var.admin_password

  role {
    role = "root"
    db   = "admin"
  }
}

# ──────────────────────────────────────────────────────────────────────
# Providers (authenticated — used after bootstrap)
# ──────────────────────────────────────────────────────────────────────
# Default provider targets mongos for user/role/shard management.
# Aliased providers connect directly to each shard primary for RS config.

provider "mongodb" {
  host          = var.mongos_host
  port          = var.mongos_port
  username      = "admin"
  password      = var.admin_password
  auth_database = "admin"

  features_enabled = [
    "mongodb_shard_config",
    "mongodb_shard",
    "mongodb_balancer_config",
    "mongodb_shard_zone",
    "mongodb_zone_key_range",
  ]
}

provider "mongodb" {
  alias         = "shard01"
  host          = var.shard01_members[0].host
  port          = var.shard01_members[0].port
  username      = "admin"
  password      = var.admin_password
  auth_database = "admin"
  direct        = true

  features_enabled = [
    "mongodb_shard_config",
  ]
}

provider "mongodb" {
  alias         = "shard02"
  host          = var.shard02_members[0].host
  port          = var.shard02_members[0].port
  username      = "admin"
  password      = var.admin_password
  auth_database = "admin"
  direct        = true

  features_enabled = [
    "mongodb_shard_config",
  ]
}

# ──────────────────────────────────────────────────────────────────────
# Phase 2: Configure shard replica sets with full membership
# ──────────────────────────────────────────────────────────────────────
# Each shard_config declares all RS members with priority/votes/tags.
# The first member gets highest priority to remain the preferred primary.
# depends_on ensures the admin user exists before we authenticate.

resource "mongodb_shard_config" "shard01" {
  provider   = mongodb.shard01
  depends_on = [mongodb_original_user.shard01_admin]

  shard_name                = "shard01"
  chaining_allowed          = true
  heartbeat_interval_millis = 1000
  heartbeat_timeout_secs    = 10
  election_timeout_millis   = 10000

  member {
    host     = "${var.shard01_members[0].host}:${var.shard01_members[0].port}"
    priority = 10
    votes    = 1
    tags = {
      role = "primary-preferred"
      zone = "us-east-1a"
    }
  }

  member {
    host     = "${var.shard01_members[1].host}:${var.shard01_members[1].port}"
    priority = 5
    votes    = 1
    tags = {
      role = "secondary"
      zone = "us-east-1b"
    }
  }

  member {
    host     = "${var.shard01_members[2].host}:${var.shard01_members[2].port}"
    priority = 5
    votes    = 1
    tags = {
      role = "secondary"
      zone = "us-east-1c"
    }
  }
}

resource "mongodb_shard_config" "shard02" {
  provider   = mongodb.shard02
  depends_on = [mongodb_original_user.shard02_admin]

  shard_name                = "shard02"
  chaining_allowed          = true
  heartbeat_interval_millis = 1000
  heartbeat_timeout_secs    = 10
  election_timeout_millis   = 10000

  member {
    host     = "${var.shard02_members[0].host}:${var.shard02_members[0].port}"
    priority = 10
    votes    = 1
    tags = {
      role = "primary-preferred"
      zone = "us-west-2a"
    }
  }

  member {
    host     = "${var.shard02_members[1].host}:${var.shard02_members[1].port}"
    priority = 5
    votes    = 1
    tags = {
      role = "secondary"
      zone = "us-west-2b"
    }
  }

  member {
    host     = "${var.shard02_members[2].host}:${var.shard02_members[2].port}"
    priority = 5
    votes    = 1
    tags = {
      role = "secondary"
      zone = "us-west-2c"
    }
  }
}

# ──────────────────────────────────────────────────────────────────────
# Phase 3: Add shards to the cluster via mongos
# ──────────────────────────────────────────────────────────────────────
# Shards are added after RS config is applied so members are known.
# hosts lists all RS members — mongos uses this for shard discovery.

resource "mongodb_shard" "shard01" {
  depends_on = [
    mongodb_original_user.mongos_admin,
    mongodb_shard_config.shard01,
  ]

  shard_name = "shard01"
  hosts = [
    for m in var.shard01_members : "${m.host}:${m.port}"
  ]
}

resource "mongodb_shard" "shard02" {
  depends_on = [
    mongodb_original_user.mongos_admin,
    mongodb_shard_config.shard02,
  ]

  shard_name = "shard02"
  hosts = [
    for m in var.shard02_members : "${m.host}:${m.port}"
  ]
}

# ──────────────────────────────────────────────────────────────────────
# Phase 4: Balancer configuration
# ──────────────────────────────────────────────────────────────────────
# Configured after shards are added so balancer has something to balance.

resource "mongodb_balancer_config" "this" {
  depends_on = [
    mongodb_shard.shard01,
    mongodb_shard.shard02,
  ]

  enabled             = true
  active_window_start = "02:00"
  active_window_stop  = "06:00"
}

# ──────────────────────────────────────────────────────────────────────
# Phase 5: Zone sharding — assign shards to zones and define key ranges
# ──────────────────────────────────────────────────────────────────────
# Zones control which shards store which data. First map shards to zones,
# then define key ranges that route documents to the correct zone.

resource "mongodb_shard_zone" "shard01_east" {
  depends_on = [mongodb_shard.shard01]

  shard_name = "shard01"
  zone       = "US-East"
}

resource "mongodb_shard_zone" "shard02_west" {
  depends_on = [mongodb_shard.shard02]

  shard_name = "shard02"
  zone       = "US-West"
}

# Route lower half of hashed _id space to US-East, upper half to US-West.
# Uses $minKey/$maxKey to cover the full key space for hashed shard keys.
resource "mongodb_zone_key_range" "orders_east" {
  depends_on = [mongodb_shard_zone.shard01_east]

  namespace = "app_db.orders"
  zone      = "US-East"
  min       = jsonencode({ "_id" = { "$minKey" = 1 } })
  max       = jsonencode({ "_id" = 0 })
}

resource "mongodb_zone_key_range" "orders_west" {
  depends_on = [mongodb_shard_zone.shard02_west]

  namespace = "app_db.orders"
  zone      = "US-West"
  min       = jsonencode({ "_id" = 0 })
  max       = jsonencode({ "_id" = { "$maxKey" = 1 } })
}

# ──────────────────────────────────────────────────────────────────────
# Phase 6: Application roles and users (via mongos)
# ──────────────────────────────────────────────────────────────────────

resource "mongodb_db_role" "app_readwrite" {
  depends_on = [mongodb_original_user.mongos_admin]

  name     = "app_readwrite"
  database = "admin"

  privilege {
    db         = "app_db"
    collection = ""
    actions    = ["find", "insert", "update", "remove"]
  }

  privilege {
    db         = "app_db"
    collection = ""
    actions    = ["createIndex", "dropIndex"]
  }
}

resource "mongodb_db_user" "app_service" {
  depends_on = [mongodb_db_role.app_readwrite]

  auth_database = "admin"
  name          = "app_service"
  password      = var.app_password

  role {
    role = mongodb_db_role.app_readwrite.name
    db   = "admin"
  }
}
