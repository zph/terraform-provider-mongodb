terraform {
  required_providers {
    mongodb = {
      source  = "zph/mongodb"
      version = "9.9.9"
    }
  }
}

# All connection details sourced from environment variables.
# Required env vars:
#   MONGO_HOST - server hostname
#   MONGO_PORT - server port
#   MONGO_USR  - username
#   MONGO_PWD  - password
#
# Optional env vars:
#   MONGODB_CERT - PEM-encoded CA certificate content
#   ALL_PROXY    - SOCKS5 proxy URL (e.g. socks5://proxy:1080)
provider "mongodb" {
  auth_database = "admin"
}
