terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "~> 0.3"
    }
  }
}

# Connect to a named replica set. The driver discovers all members
# via the seed node and routes operations to the primary.
provider "mongodb" {
  host          = "mongo-seed.example.com"
  port          = "27017"
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"

  replica_set = "rs0"
  retrywrites = true
}

variable "mongo_password" {
  description = "MongoDB root password"
  type        = string
  sensitive   = true
}
