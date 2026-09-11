terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Provider must connect to a mongos router.
# mongodb_balancer_config is experimental: enable it here or via
# TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_balancer_config.
provider "mongodb" {
  host             = "127.0.0.1"
  port             = "30109"
  username         = "admin"
  password         = var.mongo_password
  features_enabled = ["mongodb_balancer_config"]
}

# Singleton: one resource per cluster (import ID "balancer").
# enabled = false runs balancerStop. Destroy re-enables the balancer and
# clears the window, throttle, wait_for_delete and chunk size.
resource "mongodb_balancer_config" "this" {
  enabled = false
}

variable "mongo_password" {
  type      = string
  sensitive = true
}
