terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Direct connection mode bypasses replica set discovery.
# Useful when connecting to a specific mongod node (e.g. for
# shard configuration) rather than going through DNS seed list.
provider "mongodb" {
  host          = "shard01-primary.example.com"
  port          = "27018"
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"

  direct = true
}

variable "mongo_password" {
  description = "MongoDB root password"
  type        = string
  sensitive   = true
}
