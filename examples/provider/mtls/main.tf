terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Mutual TLS: the CA verifies the server, and the client certificate is
# presented to a mongod that was started with a CA file and the default
# allowConnectionsWithoutCertificates=false. Both values are PEM content,
# not file paths. The client bundle is the certificate followed by its
# unencrypted private key, the same layout as mongod's tlsCertificateKeyFile.
provider "mongodb" {
  host          = "mongo.internal.example.com"
  port          = "27017"
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"

  ssl                    = true
  certificate            = file(pathexpand(var.ca_cert_path))
  client_certificate_key = file(pathexpand(var.client_cert_key_path))
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

variable "client_cert_key_path" {
  description = "Path to a PEM file holding the client certificate followed by its private key"
  type        = string
  default     = "~/.mongodb/client.pem"
}
