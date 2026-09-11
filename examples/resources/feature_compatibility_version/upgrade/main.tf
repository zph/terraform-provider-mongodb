terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

provider "mongodb" {
  host             = "127.0.0.1"
  port             = "27017"
  username         = "root"
  password         = var.mongo_password
  auth_database    = "admin"
  features_enabled = ["mongodb_feature_compatibility_version"]

  # Show the exact setFeatureCompatibilityVersion command in the plan.
  command_preview = true
}

# Move an existing cluster from FCV 7.0 to 8.0 after every binary has been
# upgraded. Changing FCV is cluster-wide and may be irreversible; danger_mode
# acknowledges that and lets the plan through. Apply warns on every change
# and again on a downgrade. Keep danger_mode false in day-to-day config so
# the plan-time guard stays in force.
resource "mongodb_feature_compatibility_version" "this" {
  version     = "8.0"
  danger_mode = true
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
