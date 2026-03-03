terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Minimal provider configuration connecting to a local MongoDB instance.
provider "mongodb" {
  host          = "127.0.0.1"
  port          = "27017"
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"
}

variable "mongo_password" {
  description = "MongoDB root password"
  type        = string
  sensitive   = true
}
