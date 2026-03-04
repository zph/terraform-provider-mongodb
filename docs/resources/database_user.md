# mongodb_db_user

`mongodb_db_user` manages MongoDB database users. Each user has a set of roles that provide access to databases and collections.

~> **IMPORTANT:** The password is marked as sensitive and will not appear in plan/apply output, but it is stored in Terraform state as plain-text. [Read more about sensitive data in state.](https://www.terraform.io/docs/state/sensitive-data.html)

## Example Usage

### Basic user with a predefined role

```hcl
resource "mongodb_db_user" "user" {
  auth_database = "my_database"
  name          = "example"
  password      = "example"

  role {
    role = "readAnyDatabase"
    db   = "my_database"
  }
}
```

### User with a custom role

```hcl
resource "mongodb_db_user" "user_with_custom_role" {
  depends_on    = [mongodb_db_role.example_role]
  auth_database = "my_database"
  name          = var.username
  password      = var.password

  role {
    role = mongodb_db_role.example_role.name
    db   = "my_database"
  }

  role {
    role = "readAnyDatabase"
    db   = "admin"
  }
}
```

### User with multiple roles

```hcl
resource "mongodb_db_user" "multi_role_user" {
  auth_database = "admin"
  name          = "app_user"
  password      = var.password

  role {
    role = "readWrite"
    db   = "app_db"
  }

  role {
    role = "read"
    db   = "reporting_db"
  }

  role {
    role = "clusterMonitor"
    db   = "admin"
  }
}
```

## Argument Reference

* `auth_database` - (Required) Database against which MongoDB authenticates the user.
* `name` - (Required) Username for authenticating to MongoDB.
* `password` - (Required, Sensitive) User's password. Masked in plan/apply output but stored in state as plain-text.

### Role

Up to 25 role blocks are supported. A role allows the user to perform particular actions on the specified database.

* `role` - (Required) Name of the role to grant. See [built-in roles](https://docs.mongodb.com/manual/reference/built-in-roles/) or use a custom role name.
* `db` - (Optional) Database on which the user has the specified role. A role on the `admin` database can include privileges that apply to other databases.

## Import

MongoDB users can be imported using the `database.username` format:

```sh
$ terraform import mongodb_db_user.example_user admin.my_user
```
