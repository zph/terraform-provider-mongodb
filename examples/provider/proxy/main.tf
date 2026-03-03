terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Connect through a SOCKS5 proxy (e.g. SSH tunnel or bastion).
# Proxy URL must use socks5:// or socks5h:// scheme.
# Can also be set via ALL_PROXY or all_proxy env var.
provider "mongodb" {
  host          = "mongo.private.example.com"
  port          = "27017"
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"

  proxy = "socks5://localhost:1080"
}

variable "mongo_password" {
  description = "MongoDB root password"
  type        = string
  sensitive   = true
}
