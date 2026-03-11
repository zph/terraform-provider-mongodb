# MongoDB Provider

The MongoDB provider manages resources on self-hosted MongoDB deployments. Configure the provider with connection credentials, then use the resources to manage users, roles, replica set configuration, and shard membership.

You may want to consider pinning the [provider version](https://www.terraform.io/docs/configuration/providers.html#provider-versions) to ensure you have a chance to review and prepare for changes.

## Example Usage

```hcl
provider "mongodb" {
  host          = "127.0.0.1"
  port          = "27017"
  username      = "admin"
  password      = var.mongo_password
  auth_database = "admin"
}
```

### With TLS

```hcl
provider "mongodb" {
  host                 = "mongo.example.com"
  port                 = "27017"
  username             = "admin"
  password             = var.mongo_password
  auth_database        = "admin"
  ssl                  = true
  certificate          = file(pathexpand("~/.mongodb/ca.pem"))
  insecure_skip_verify = false
}
```

### Environment Variables

Credentials can be provided via environment variables instead of the provider block:

```shell
export MONGO_HOST="127.0.0.1"
export MONGO_PORT="27017"
export MONGO_USR="admin"
export MONGO_PWD="secret"
```

```hcl
provider "mongodb" {
  auth_database = "admin"
}
```

## Argument Reference

* `host` - (Required) MongoDB server address. Can be sourced from `MONGO_HOST`. Default: `"127.0.0.1"`.
* `port` - (Required) MongoDB server port. Can be sourced from `MONGO_PORT`. Default: `"27017"`.
* `username` - (Optional) Username for authentication. Can be sourced from `MONGO_USR`.
* `password` - (Optional) Password for authentication. Can be sourced from `MONGO_PWD`.
* `auth_database` - (Optional) Authentication database. Default: `"admin"`.
* `ssl` - (Optional) Enable TLS/SSL. Default: `false`.
* `certificate` - (Optional) PEM-encoded CA certificate content for TLS. Can be sourced from `MONGODB_CERT`.
* `insecure_skip_verify` - (Optional) Skip TLS certificate hostname verification. Default: `false`.
* `replica_set` - (Optional) Replica set name. When set, the driver uses discovery mode.
* `direct` - (Optional) Force a direct connection (bypass replica set discovery). Default: `false`.
* `retrywrites` - (Optional) Enable retryable writes. Default: `true`.
* `proxy` - (Optional) SOCKS5 proxy URL (e.g., `socks5://myproxy:8080`). Can be sourced from `ALL_PROXY` or `all_proxy`.
* `command_preview` - (Optional) When `true`, each resource populates a `planned_commands` attribute during `terraform plan` showing the exact MongoDB commands that will execute on apply. No MongoDB connection is made during plan. Passwords are shown as `[REDACTED]`. Can be sourced from `TERRAFORM_PROVIDER_MONGODB_COMMAND_PREVIEW`. Default: `false`.
* `features_enabled` - (Optional) Set of experimental resource names to enable (e.g., `["mongodb_shard_config", "mongodb_shard"]`). Merged with the `TERRAFORM_PROVIDER_MONGODB_ENABLE` environment variable. Only recognized experimental resource names are accepted.

## Command Preview

Enable `command_preview` to see MongoDB commands in plan output:

```hcl
provider "mongodb" {
  host            = "127.0.0.1"
  port            = "27017"
  username        = "admin"
  password        = var.mongo_password
  auth_database   = "admin"
  command_preview = true
}
```

Example plan output:

```
# mongodb_server_parameter.notablescan will be created
  + resource "mongodb_server_parameter" "notablescan" {
      + parameter        = "notablescan"
      + planned_commands = "db.adminCommand({setParameter: 1, \"notablescan\": \"true\"})"
      + value            = "true"
    }
```

## Experimental Resources

Several resources are classified as experimental and require opt-in via `features_enabled` in the provider block or the `TERRAFORM_PROVIDER_MONGODB_ENABLE` environment variable. Both sources are merged.

```hcl
provider "mongodb" {
  # ...
  features_enabled = [
    "mongodb_shard_config",
    "mongodb_shard",
  ]
}
```

Or via environment variable:

```bash
export TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_shard_config,mongodb_shard
```
