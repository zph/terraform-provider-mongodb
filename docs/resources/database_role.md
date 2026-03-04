# mongodb_db_role

`mongodb_db_role` manages custom MongoDB roles. Use custom roles to specify custom sets of privileges and optionally inherit from other roles.

## Example Usage

### Basic role with collection privileges

```hcl
resource "mongodb_db_role" "example_role" {
  name     = "role_name"
  database = "my_database"

  privilege {
    db         = "my_database"
    collection = ""
    actions    = ["listCollections", "createCollection", "createIndex", "dropIndex", "insert", "remove", "renameCollectionSameDB", "update"]
  }

  privilege {
    db         = "admin"
    collection = "*"
    actions    = ["collStats"]
  }
}
```

### Cluster-level privilege

When `cluster = true`, the privilege applies to cluster-wide operations. The `db` and `collection` fields are ignored by MongoDB.

```hcl
resource "mongodb_db_role" "failover_operator" {
  name     = "failover_operator"
  database = "admin"

  privilege {
    cluster = true
    actions = ["replSetGetConfig", "replSetGetStatus", "replSetStateChange"]
  }
}
```

### Inherited roles

```hcl
resource "mongodb_db_role" "base_role" {
  database = "admin"
  name     = "base_role"

  privilege {
    db         = "admin"
    collection = ""
    actions    = ["collStats"]
  }
}

resource "mongodb_db_role" "derived_role" {
  depends_on = [mongodb_db_role.base_role]
  database   = "admin"
  name       = "derived_role"

  inherited_role {
    role = mongodb_db_role.base_role.name
    db   = "admin"
  }
}
```

### Composite role (privileges + inheritance)

```hcl
resource "mongodb_db_role" "monitoring" {
  name     = "custom_monitoring"
  database = "admin"

  privilege {
    cluster = true
    actions = ["replSetGetStatus", "serverStatus"]
  }
}

resource "mongodb_db_role" "data_access" {
  name     = "custom_data_access"
  database = "admin"

  privilege {
    db         = "orders"
    collection = ""
    actions    = ["find", "insert", "update", "remove", "createIndex"]
  }
}

resource "mongodb_db_role" "admin_composite" {
  depends_on = [
    mongodb_db_role.monitoring,
    mongodb_db_role.data_access,
  ]
  name     = "admin_composite_role"
  database = "admin"

  privilege {
    db         = "admin"
    collection = ""
    actions    = ["collStats", "dbStats", "listCollections"]
  }

  inherited_role {
    role = mongodb_db_role.monitoring.name
    db   = "admin"
  }

  inherited_role {
    role = mongodb_db_role.data_access.name
    db   = "admin"
  }
}
```

## Argument Reference

* `name` - (Required) Name of the custom role.
* `database` - (Optional) The database of the role. Default: `"admin"`.

~> **IMPORTANT:** If a role is created in a specific database, it can only be inherited by another role in the same database.

### Privilege

Each privilege block grants a set of actions. Up to 20 privilege blocks are supported.

* `actions` - (Required) List of privilege actions. See [Custom Role Actions](https://docs.mongodb.com/manual/reference/privilege-actions/).
* `db` - (Optional) Database on which the actions are granted.
* `collection` - (Optional) Collection on which the actions are granted. An empty string (`""`) grants actions on all collections in the database.
* `cluster` - (Optional) When `true`, the privilege applies to cluster-wide operations. `db` and `collection` are ignored by MongoDB when this is set.

### Inherited Role

Each inherited_role block grants all privileges of the referenced role. Up to 20 inherited_role blocks are supported.

* `role` - (Required) Name of the inherited role. Can be a custom role or a [built-in role](https://docs.mongodb.com/manual/reference/built-in-roles/).
* `db` - (Optional) Database on which the inherited role is granted. Should be `admin` for all roles except `read` and `readWrite`.

## Import

MongoDB roles can be imported using the `database.rolename` format:

```sh
$ terraform import mongodb_db_role.example_role admin.my_role
```
