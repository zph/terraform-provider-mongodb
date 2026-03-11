terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Add a new replica set (shard03) to an existing sharded cluster.
#
# Use case: day-2 horizontal scale-out — the cluster already has mongos,
# config servers, and one or more shards running with auth enabled.
# This configuration bootstraps a brand-new RS from bare mongod processes
# and registers it as a shard.
#
# Topology (new):
#   - 1 new shard RS (shard03) — 3 members, initialized from scratch
#
# Topology (existing):
#   - 1 mongos router (already running, admin user exists)
#   - 1+ existing shards (already registered)
#
# Execution order (enforced by depends_on):
#   1. Initialize RS with all members (no auth — localhost exception)
#   2. Bootstrap admin user on the new RS via localhost exception
#   3. Register the new shard with mongos (authenticated)
#   4. Assign the new shard to a zone and define key ranges
#
# Prerequisites:
#   - New mongod processes started with --shardsvr, --replSet, --auth,
#     and --keyFile (same keyFile as the rest of the cluster)
#   - No users exist on the new RS yet (localhost exception allows bootstrap)
#   - Mongos is reachable and an admin user already exists on it

# ──────────────────────────────────────────────────────────────────────
# Variables
# ──────────────────────────────────────────────────────────────────────

# variable "admin_password" {
#   description = "Password for the admin user (must match existing cluster admin)"
#   type        = string
#   sensitive   = true
# }

variable "admin_username" {
  description = "Username for the admin user"
  type        = string
  default     = "user"
}

variable "admin_password" {
  description = "Password for the admin user"
  type        = string
  default     = "password"
}

variable "mongos_host" {
  description = "Hostname of the existing mongos router"
  type        = string
  default     = "localhost"
}

variable "mongos_port" {
  description = "Port of the existing mongos router"
  type        = string
  default     = "27017"
}

variable "new_shard_name" {
  description = "Replica set name for the new shard"
  type        = string
  default     = "shard01"
}

variable "new_shard_zone" {
  description = "Zone to assign the new shard to (e.g. US-East, Hot)"
  type        = string
  default     = "US-East"
}

variable "new_shard_members" {
  description = "New shard RS members (first entry is the primary candidate)"
  type = list(object({
    host = string
    port = string
  }))
  default = [
    { host = "localhost", port = "37017" },
    { host = "localhost", port = "37018" },
    { host = "localhost", port = "37019" },
  ]
}

# ──────────────────────────────────────────────────────────────────────
# Providers
# ──────────────────────────────────────────────────────────────────────

# Enable experimental resources before running:
#   export TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_shard_config,mongodb_shard

# Direct provider for Phase 1: connects to the new shard primary.
# On first run (no users yet), Create falls back to no-auth via localhost
# exception (INIT-029). On subsequent runs, auth works normally.
provider "mongodb" {
  alias         = "new_shard"
  host          = var.new_shard_members[0].host
  port          = var.new_shard_members[0].port
  username      = var.admin_username
  password      = var.admin_password
  auth_database = "admin"
  direct        = true
  features_enabled = [
    "mongodb_shard_config",
  ]
}

# Authenticated provider for Phase 3+4: addShard and zone config via mongos.
provider "mongodb" {
  host          = var.mongos_host
  port          = var.mongos_port
  username      = var.admin_username
  password      = var.admin_password
  auth_database = "admin"
  features_enabled = [
    "mongodb_shard",
    "mongodb_shard_zone",
    "mongodb_zone_key_range",
  ]
}

# ──────────────────────────────────────────────────────────────────────
# Phase 1: Initialize and configure the new replica set
# ──────────────────────────────────────────────────────────────────────
# On first run, shard_config detects auth failure (no users yet), falls back
# to the init flow which uses ConnectForInit with no-auth (INIT-029). It runs
# replSetInitiate, waits for PRIMARY, then adds remaining members.
# On subsequent runs, auth succeeds and Read/Update work normally.

resource "mongodb_shard_config" "new_shard" {
  provider = mongodb.new_shard

  shard_name                = var.new_shard_name
  chaining_allowed          = true
  heartbeat_interval_millis = 1000
  heartbeat_timeout_secs    = 10
  election_timeout_millis   = 10000

  member {
    host     = "${var.new_shard_members[0].host}:${var.new_shard_members[0].port}"
    priority = 10
    votes    = 1
    tags = {
      role = "leader"
    }
  }

  member {
    host     = "${var.new_shard_members[1].host}:${var.new_shard_members[1].port}"
    priority = 5
    votes    = 1
    tags = {
      role = "follower"
    }
  }

  member {
    host     = "${var.new_shard_members[2].host}:${var.new_shard_members[2].port}"
    priority = 1
    votes    = 1
    tags = {
      role = "analytics"
    }
  }
}

# ──────────────────────────────────────────────────────────────────────
# Phase 2: Bootstrap admin user via localhost exception
# ──────────────────────────────────────────────────────────────────────
# The RS is now initialized and has a PRIMARY. No users exist yet, so the
# localhost exception still allows createUser without auth. original_user
# connects without auth and creates the initial admin.

resource "mongodb_original_user" "new_shard_admin" {
  depends_on = [mongodb_shard_config.new_shard]

  host     = var.new_shard_members[0].host
  port     = var.new_shard_members[0].port
  username = var.admin_username
  password = var.admin_password

  role {
    role = "root"
    db   = "admin"
  }
}

# ──────────────────────────────────────────────────────────────────────
# Phase 3: Register the new shard with the existing cluster
# ──────────────────────────────────────────────────────────────────────
# Runs addShard via mongos. The RS must be fully initialized and the
# admin user must exist (auth is required on the mongos connection).
# After this, the balancer will begin migrating chunks to the new shard.

resource "mongodb_shard" "shard01" {
  depends_on = [mongodb_shard_config.new_shard, mongodb_original_user.new_shard_admin]

  shard_name = var.new_shard_name
  hosts = [
    for m in var.new_shard_members : "${m.host}:${m.port}"
  ]
}

# ──────────────────────────────────────────────────────────────────────
# Phase 4: Assign the new shard to a zone and define key ranges
# ──────────────────────────────────────────────────────────────────────
# Zone tagging controls which shards store which data. The shard must be
# registered (Phase 3) before it can be assigned to a zone. Key ranges
# route documents to the zone based on shard key values.

resource "mongodb_shard_zone" "shard01_zone" {
  depends_on = [mongodb_shard.shard01]

  shard_name = var.new_shard_name
  zone       = var.new_shard_zone
}

# Route all documents in app_db.orders with region keys to the new zone.
# Adjust namespace, min, and max to match your shard key and data layout.
resource "mongodb_zone_key_range" "shard01_zone_key_range" {
  depends_on = [mongodb_shard_zone.shard01_zone]

  namespace = "app_db.orders"
  zone      = var.new_shard_zone
  min       = jsonencode({ "region" = { "$minKey" = 1 } })
  max       = jsonencode({ "region" = { "$maxKey" = 1 } })
}
