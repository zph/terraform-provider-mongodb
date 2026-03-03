# mongodb_original_user

`mongodb_original_user` bootstraps the initial admin user on a fresh MongoDB instance running without authentication. This resource carries its own connection parameters independent of the provider config, since the provider credentials do not yet exist during bootstrap.

~> **IMPORTANT:** This resource connects to MongoDB **without authentication** to create the first user. After bootstrap, configure the provider with the credentials created by this resource.

~> **IMPORTANT:** If the user already exists and the provided credentials authenticate successfully, the resource adopts the existing user into Terraform state rather than failing.

## Example Usage

### Bootstrap admin user on a standalone instance

```hcl
provider "mongodb" {
  host     = "127.0.0.1"
  port     = "27017"
  username = "placeholder"
  password = "placeholder"
}

variable "admin_password" {
  type      = string
  sensitive = true
}

resource "mongodb_original_user" "admin" {
  host     = "127.0.0.1"
  port     = "27017"
  username = "admin"
  password = var.admin_password

  role {
    role = "root"
    db   = "admin"
  }
}
```

### Bootstrap across a sharded cluster

```hcl
resource "mongodb_original_user" "mongos_admin" {
  host     = "127.0.0.1"
  port     = "30109"
  username = "admin"
  password = var.admin_password

  role {
    role = "root"
    db   = "admin"
  }
}

resource "mongodb_original_user" "shard01_admin" {
  host     = "127.0.0.1"
  port     = "30103"
  username = "admin"
  password = var.admin_password

  role {
    role = "root"
    db   = "admin"
  }
}

resource "mongodb_original_user" "shard02_admin" {
  host     = "127.0.0.1"
  port     = "30106"
  username = "admin"
  password = var.admin_password

  role {
    role = "root"
    db   = "admin"
  }
}
```

## Argument Reference

* `host` - (Required) MongoDB host to connect to without auth.
* `port` - (Required) MongoDB port.
* `username` - (Required) Admin username to create.
* `password` - (Required, Sensitive) Admin password to create.
* `auth_database` - (Optional) Database to create the user in. Default: `"admin"`.
* `ssl` - (Optional) Enable SSL. Default: `false`.
* `certificate` - (Optional, Sensitive) PEM-encoded certificate content for TLS.
* `insecure_skip_verify` - (Optional) Skip certificate verification. Default: `false`.
* `replica_set` - (Optional, Computed) Replica set name. Auto-discovered from the server via `isMaster` if not set. When present, the driver uses discovery mode to route writes to the primary.

### Role

Each role block assigns a role to the user. If no roles are specified, the user is granted the `root` role on `admin` by default.

* `role` - (Required) Name of the role to grant.
* `db` - (Optional) Database on which the role is granted.

## How It Works

### Create

1. Probes the server via `isMaster` to auto-discover the replica set name (if not specified).
2. Connects without authentication.
3. Runs `createUser` with the specified username, password, and roles.
4. If the user already exists, attempts authenticated connection with the provided credentials and adopts the user into state.

### Read

1. Connects with authentication and verifies the user exists.
2. Falls back to a no-auth connection if authentication fails (server may have been reset).
3. If the user is not found, removes the resource from state.

### Update

1. Connects with authentication.
2. Drops the existing user.
3. Recreates the user with updated credentials and roles.

### Delete

1. Connects with authentication.
2. Drops the user.
