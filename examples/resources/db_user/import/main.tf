terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

provider "mongodb" {
  host          = "127.0.0.1"
  port          = "27017"
  username      = "root"
  password      = var.mongo_password
  auth_database = "admin"
}

# Importing an existing MongoDB user into Terraform state.
#
# The import ID is "{auth_database}.{username}" (plain text):
#
#   $ terraform import mongodb_db_user.existing admin.existing_user
#
# After import, run `terraform plan` to verify state matches config.
# Note: password cannot be read back from MongoDB, so set it to
# the current password to avoid an unnecessary update.
resource "mongodb_db_user" "existing" {
  auth_database = "admin"
  name          = "existing_user"
  password      = var.existing_password

  role {
    role = "readWriteAnyDatabase"
    db   = "admin"
  }
}

variable "mongo_password" {
  type      = string
  sensitive = true
}

variable "existing_password" {
  type      = string
  sensitive = true
}
