terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# TLS-enabled connection with a custom CA certificate.
# The certificate value is the PEM-encoded CA content (not a file path).
provider "mongodb" {
  host          = "mongo.internal.example.com"
  port          = "27017"
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"

  ssl                  = true
  insecure_skip_verify = false
  certificate          = file(pathexpand(var.ca_cert_path))
}

variable "mongo_password" {
  description = "MongoDB root password"
  type        = string
  sensitive   = true
}

variable "ca_cert_path" {
  description = "Path to PEM-encoded CA certificate file"
  type        = string
  default     = "~/.mongodb/ca.pem"
}
