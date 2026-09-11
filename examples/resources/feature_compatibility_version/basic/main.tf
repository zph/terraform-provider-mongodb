terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# mongodb_feature_compatibility_version is experimental: enable it here or via
# TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_feature_compatibility_version.
provider "mongodb" {
  host             = "127.0.0.1"
  port             = "27017"
  username         = "root"
  password         = var.mongo_password
  auth_database    = "admin"
  features_enabled = ["mongodb_feature_compatibility_version"]
}

# Pin the cluster's featureCompatibilityVersion. Singleton (import ID "fcv").
#
# Creating always proceeds. Once the resource exists, any change to version
# is rejected at plan time until danger_mode = true (see the upgrade example),
# so a stray edit cannot move the cluster's FCV.
# Destroy is state-only: FCV always has a value and cannot be unset.
resource "mongodb_feature_compatibility_version" "this" {
  version = "7.0"

  lifecycle {
    prevent_destroy = true
  }
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
