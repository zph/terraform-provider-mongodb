terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# Bootstrap the first admin user on a replica set that only accepts TLS.
# The resource dials MongoDB with its own TLS settings; the provider block's
# connection is not used until a user exists.
provider "mongodb" {
  host        = "localhost"
  port        = "27017"
  ssl         = true
  certificate = file(pathexpand(var.ca_cert_path))
}

variable "ca_cert_path" {
  description = "Path to the PEM-encoded CA that signed the server certificate"
  type        = string
  default     = "~/.mongodb/ca.pem"
}

resource "mongodb_original_user" "admin" {
  # The localhost exception only applies to loopback connections, so the
  # server certificate has to cover the loopback name used here.
  host = "localhost"
  port = "27017"

  ssl         = true
  certificate = file(pathexpand(var.ca_cert_path))
  # Only set true when the certificate does not cover the loopback name.
  insecure_skip_verify = false

  # Auto-discovered when omitted. Setting it skips the probe and connects in
  # discovery mode, so the createUser write is routed to the primary.
  replica_set = "rs0"

  # Database the user is created in (default admin).
  auth_database = "admin"

  username = "admin"
  # No password attribute: supplied through MONGODB_ORIGINAL_USER_PASSWORD
  # at apply time, so it is never written to Terraform state.

  role {
    role = "root"
    db   = "admin"
  }
}
