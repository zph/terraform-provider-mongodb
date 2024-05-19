terraform {
  required_version = ">= 1.5.7"

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
    role = "StaffRole"
    db =   "admin"
  }
}
