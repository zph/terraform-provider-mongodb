# Terraform Provider MongoDB

> Fork of [Kaginari/terraform-provider-mongodb](https://github.com/Kaginari/terraform-provider-mongodb).
> Forked to make larger changes than could be contributed via pull request to the upstream project and to iterate quickly for my own use cases. The changes are intended for production maturity but at this point the project is
largely unvalidated beyond the tests seen here.

This repository is a Terraform MongoDB provider for [Terraform](https://www.terraform.io).

### Why no MongoDB Atlas support?

This provider targets self-hosted MongoDB. We don't support MongoDB Atlas because we don't believe in fear-based extortion as a software engineering business model. If you need Atlas support, MongoDB has their own provider — best of luck with that. If you're paying them hundreds of thousands and want relief, ping me.

### Why no Amazon DocumentDB support?

DocumentDB shipped with a single-writer architecture for its first years of existence. We judge that decision harshly and don't support it here.

## Resources

| Resource | Maturity | Description |
|----------|----------|-------------|
| [`mongodb_db_user`](docs/resources/database_user.md) | Stable | Manage MongoDB database users |
| [`mongodb_db_role`](docs/resources/database_role.md) | Stable | Manage custom MongoDB roles with privileges and inheritance |
| [`mongodb_original_user`](docs/resources/original_user.md) | Stable | Bootstrap the initial admin user on a no-auth instance |
| [`mongodb_shard_config`](docs/resources/shard_config.md) | Experimental | Configure replica set settings, initialize replica sets, manage members |
| [`mongodb_shard`](docs/resources/shard.md) | Experimental | Add/remove shards from a mongos router |
| [`mongodb_profiler`](docs/resources/profiler.md) | Experimental | Manage per-database profiler configuration |
| [`mongodb_server_parameter`](docs/resources/server_parameter.md) | Experimental | Set/get MongoDB server parameters via setParameter |
| [`mongodb_balancer_config`](docs/resources/balancer_config.md) | Experimental | Manage global balancer settings |
| [`mongodb_collection_balancing`](docs/resources/collection_balancing.md) | Experimental | Manage per-collection balancer enable/disable and chunk size |
| [`mongodb_feature_compatibility_version`](docs/resources/feature_compatibility_version.md) | Experimental | Manage featureCompatibilityVersion with safety gate |

Experimental resources require opt-in via `features_enabled` in the provider block or the `TERRAFORM_PROVIDER_MONGODB_ENABLE` environment variable. Both sources are merged.

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

See [`examples/`](examples/README.md) for runnable configurations for each resource.

## Provider Configuration

```hcl
provider "mongodb" {
  host               = "127.0.0.1"     # MONGO_HOST
  port               = "27017"         # MONGO_PORT
  username           = "admin"         # MONGO_USR
  password           = var.password    # MONGO_PWD
  auth_database      = "admin"
  ssl                = false
  certificate        = ""              # MONGODB_CERT
  insecure_skip_verify = false
  replica_set        = ""
  direct             = false
  retrywrites        = true
  proxy              = ""              # ALL_PROXY / all_proxy (socks5)
  command_preview    = false           # TERRAFORM_PROVIDER_MONGODB_COMMAND_PREVIEW
  features_enabled   = []             # TERRAFORM_PROVIDER_MONGODB_ENABLE
}
```

### Command Preview

Set `command_preview = true` (or `TERRAFORM_PROVIDER_MONGODB_COMMAND_PREVIEW=true`) to see the exact MongoDB commands that will execute during `terraform plan`:

```
# mongodb_profiler.mydb will be created
  + resource "mongodb_profiler" "mydb" {
      + database         = "mydb"
      + level            = 1
      + planned_commands = "db.getSiblingDB(\"mydb\").runCommand({profile: 1, slowms: 100, ratelimit: 1})"
      + slowms           = 100
    }
```

Commands are computed from the Terraform config diff — no MongoDB connection is made during plan. Passwords are always shown as `[REDACTED]`.

## CDKTN Construct Library

The [`cdktn/`](cdktn/) directory contains a Go construct library for generating Terraform JSON
configs for sharded MongoDB clusters. Instead of hand-writing provider aliases and resources for
every node, define your cluster topology in Go and synthesize deterministic Terraform JSON.

See [`cdktn/README.md`](cdktn/README.md) for usage examples and API documentation.

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.7.5
- [Go](https://golang.org/doc/install) >= 1.17

## Installation

```bash
git clone https://github.com/zph/terraform-provider-mongodb
cd terraform-provider-mongodb
make install
```

## Local Development

Use [MUP](https://github.com/zph/mup) to spin up local MongoDB clusters for development and testing:

```bash
# Start a local MongoDB cluster (playground mode)
mup playground start

# Start with a specific version
mup playground start --version 8.0

# Check cluster status
mup playground status

# Connect to the cluster
mup playground connect

# Stop the cluster
mup playground stop

# Destroy the cluster
mup playground destroy
```

MUP supports standalone, replica set, and sharded cluster topologies. See the [MUP README](https://github.com/zph/mup) for full documentation.

### Running Tests

```bash
# Unit tests
make test

# Golden file tests (requires Docker)
make test-golden

# Sharded integration tests (requires Docker)
make test-sharded-integration

# All available targets
make help
```

### Releasing

```bash
make release
```

This strips `-dev` from `VERSION`, commits, tags `v<version>`, pushes both, then bumps `VERSION` to the next patch `-dev`. For example, `0.2.0-dev` becomes release `v0.2.0`, then `VERSION` is set to `0.2.1-dev`.

Pushing a `v*` tag triggers the GitHub Actions release workflow, which builds binaries via GoReleaser and publishes to the Terraform Registry.

To tag manually without the auto-bump: `make tag && git push origin v$(cat VERSION)`.
