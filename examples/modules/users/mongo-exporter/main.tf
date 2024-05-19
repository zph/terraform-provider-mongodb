terraform {
  required_version = ">= 0.13"

  required_providers {
    mongodb = {
      source = "registry.terraform.io/Kaginari/mongodb"
      version = "9.9.9"
    }
  }
}

variable "username" {
  description = "the user name"
  type = string
}
variable "password" {
  description = "the user password"
  type = string
}

resource "mongodb_db_user" "user" {
  auth_database = "admin"
  name = var.username
  password = var.password
  role {
   # role = mongodb_db_role.role.name
    role = "custom_role_test"
    db =   "admin"
  }
}
