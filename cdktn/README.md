# CDKTN Sharded MongoDB Cluster Construct Library

Go construct library that generates deterministic Terraform JSON for sharded MongoDB clusters
using `terraform-provider-mongodb` (`registry.terraform.io/zph/mongodb`).

Replaces the Jinja2 template approach with typed, composable Go constructs that validate
MongoDB topology constraints at construction time.

## Quick Start

```go
package main

import (
	"fmt"
	"os"

	cdktn "github.com/zph/terraform-provider-mongodb/cdktn"
)

func main() {
	cluster, err := cdktn.NewMongoShardedCluster("prod", &cdktn.MongoShardedClusterProps{
		ProviderVersion: ">= 1.0.0",
		Credentials:     &cdktn.DirectCredentials{Username: "admin", Password: "s3cret"},
		SSL:             &cdktn.SSLConfig{Enabled: true},
		ConfigServers: cdktn.ConfigServerConfig{
			ReplicaSetName: "csrs",
			Members: []cdktn.MemberConfig{
				{Host: "cfg1.example.com", Port: 27019},
				{Host: "cfg2.example.com", Port: 27019},
				{Host: "cfg3.example.com", Port: 27019},
			},
		},
		Shards: []cdktn.ShardConfig{
			{
				ReplicaSetName: "shard01",
				Members: []cdktn.MemberConfig{
					{Host: "s1m1.example.com", Port: 27018},
					{Host: "s1m2.example.com", Port: 27018},
					{Host: "s1m3.example.com", Port: 27018},
				},
			},
		},
		Mongos: []cdktn.MongosConfig{
			{Members: []cdktn.MemberConfig{{Host: "mongos1.example.com", Port: 27017}}},
		},
		Roles: []cdktn.RoleConfig{
			{
				Name:     "AppRole",
				Database: "admin",
				InheritedRoles: []cdktn.InheritedRole{
					{Role: "readWriteAnyDatabase", DB: "admin"},
				},
			},
		},
		Users: []cdktn.UserConfig{
			{
				Username: "appuser",
				Password: "app-secret",
				Database: "admin",
				Roles:    []cdktn.UserRoleRef{{Role: "AppRole", DB: "admin"}},
			},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	data, err := cluster.Stack.Synth()
	if err != nil {
		fmt.Fprintf(os.Stderr, "synth error: %v\n", err)
		os.Exit(1)
	}

	// Write to stdout or a file — feed to `terraform init && terraform apply`
	os.Stdout.Write(data)
}
```

Run it:

```bash
go run main.go > main.tf.json
terraform init
terraform plan
```

## Usage Examples

### Minimal Cluster (1 shard, 1 mongos)

```go
cluster, err := cdktn.NewMongoShardedCluster("minimal", &cdktn.MongoShardedClusterProps{
    ProviderVersion: "9.9.9",
    Credentials:     &cdktn.DirectCredentials{Username: "admin", Password: "pass"},
    ConfigServers: cdktn.ConfigServerConfig{
        ReplicaSetName: "csrs",
        Members: []cdktn.MemberConfig{
            {Host: "cfg1", Port: 27019},
            {Host: "cfg2", Port: 27020},
            {Host: "cfg3", Port: 27021},
        },
    },
    Shards: []cdktn.ShardConfig{{
        ReplicaSetName: "shard01",
        Members: []cdktn.MemberConfig{
            {Host: "s1m1", Port: 27018},
            {Host: "s1m2", Port: 27019},
            {Host: "s1m3", Port: 27020},
        },
    }},
    Mongos: []cdktn.MongosConfig{{
        Members: []cdktn.MemberConfig{{Host: "mongos1", Port: 27017}},
    }},
})
```

### Environment Variable Credentials

Use `EnvCredentials` so the generated Terraform defers to provider env var defaults
(`MONGO_USR` / `MONGO_PWD`):

```go
cluster, err := cdktn.NewMongoShardedCluster("env", &cdktn.MongoShardedClusterProps{
    Credentials: &cdktn.EnvCredentials{
        UsernameEnvVar: "MONGO_USR",
        PasswordEnvVar: "MONGO_PWD",
    },
    // ... rest of config
})
```

### AWS Parameter Store Credentials

Store MongoDB credentials in AWS Systems Manager Parameter Store and reference them
at `terraform plan`/`apply` time. Create a companion `data.tf` alongside the generated
`.tf.json` to define the SSM data sources:

```hcl
# data.tf — lives next to the generated .tf.json
data "aws_ssm_parameter" "mongo_username" {
  name = "/prod/mongodb/admin-username"
}

data "aws_ssm_parameter" "mongo_password" {
  name            = "/prod/mongodb/admin-password"
  with_decryption = true
}
```

Then pass Terraform expression references through `DirectCredentials`:

```go
cluster, err := cdktn.NewMongoShardedCluster("prod", &cdktn.MongoShardedClusterProps{
    ProviderVersion: ">= 1.0.0",
    Credentials: &cdktn.DirectCredentials{
        Username: "${data.aws_ssm_parameter.mongo_username.value}",
        Password: "${data.aws_ssm_parameter.mongo_password.value}",
    },
    // ... rest of config
})
```

Terraform resolves the `${...}` expressions at plan time, so no secrets appear in the
generated JSON. This pattern works with any Terraform data source — `aws_secretsmanager_secret_version`,
`vault_generic_secret`, etc.

### Multi-Shard Cluster with SSL and Proxy

```go
cluster, err := cdktn.NewMongoShardedCluster("multi", &cdktn.MongoShardedClusterProps{
    ProviderVersion: ">= 1.0.0",
    Credentials:     &cdktn.DirectCredentials{Username: "admin", Password: "pass"},
    SSL: &cdktn.SSLConfig{
        Enabled:            true,
        Certificate:        pemCertString,
        InsecureSkipVerify: false,
    },
    Proxy: "socks5://bastion:1080",
    ConfigServers: cdktn.ConfigServerConfig{
        ReplicaSetName: "csrs",
        Members: []cdktn.MemberConfig{
            {Host: "cfg1", Port: 27019},
            {Host: "cfg2", Port: 27019},
            {Host: "cfg3", Port: 27019},
        },
    },
    Shards: []cdktn.ShardConfig{
        {
            ReplicaSetName: "shard01",
            Members: []cdktn.MemberConfig{
                {Host: "s1m1", Port: 27018},
                {Host: "s1m2", Port: 27018},
                {Host: "s1m3", Port: 27018},
            },
        },
        {
            ReplicaSetName: "shard02",
            Members: []cdktn.MemberConfig{
                {Host: "s2m1", Port: 27018},
                {Host: "s2m2", Port: 27018},
                {Host: "s2m3", Port: 27018},
            },
        },
    },
    Mongos: []cdktn.MongosConfig{
        {Members: []cdktn.MemberConfig{{Host: "mongos1", Port: 27017}}},
        {Members: []cdktn.MemberConfig{{Host: "mongos2", Port: 27017}}},
    },
})
```

### Shard-Local Users (Per-Component)

Users defined on a `ShardConfig` are scoped to that shard's members only:

```go
Shards: []cdktn.ShardConfig{{
    ReplicaSetName: "shard01",
    Members: []cdktn.MemberConfig{
        {Host: "s1m1", Port: 27018},
        {Host: "s1m2", Port: 27019},
        {Host: "s1m3", Port: 27020},
    },
    Users: []cdktn.UserConfig{{
        Username: "shard-monitor",
        Password: "mon-pass",
        Database: "admin",
        Roles:    []cdktn.UserRoleRef{{Role: "clusterMonitor", DB: "admin"}},
    }},
}},
```

Cluster-level `Users` propagate to all mongos instances and the primary of each shard.

### Per-Member Credential Override

Individual members can use different credentials than the cluster default:

```go
Shards: []cdktn.ShardConfig{{
    ReplicaSetName: "shard01",
    Members: []cdktn.MemberConfig{
        {Host: "s1m1", Port: 27018}, // uses cluster credentials
        {Host: "s1m2", Port: 27019}, // uses cluster credentials
        {Host: "s1m3", Port: 27020, Credentials: &cdktn.DirectCredentials{
            Username: "shard01-admin",
            Password: "different-pass",
        }}, // uses its own credentials
    },
}},
```

### Custom Shard Config Settings

Override replica set election/heartbeat tuning:

```go
Shards: []cdktn.ShardConfig{{
    ReplicaSetName: "shard01",
    Members:        members,
    ShardConfig: &cdktn.ShardConfigSettings{
        ChainingAllowed:         false,
        HeartbeatIntervalMillis: 2000,
        HeartbeatTimeoutSecs:    15,
        ElectionTimeoutMillis:   15000,
    },
}},
```

Omitting `ShardConfig` uses defaults: `chaining_allowed: true`, `heartbeat_interval_millis: 1000`,
`heartbeat_timeout_secs: 10`, `election_timeout_millis: 10000`.

### Using L2 Constructs Independently

Each L2 construct can be used standalone on its own stack:

```go
stack := cdktn.NewTerraformStack(">= 1.7.5", "9.9.9")

shard, err := cdktn.NewMongoShard(stack, "my-shard", &cdktn.MongoShardProps{
    ReplicaSetName: "shard01",
    Members: []cdktn.MemberConfig{
        {Host: "h1", Port: 27018},
        {Host: "h2", Port: 27019},
        {Host: "h3", Port: 27020},
    },
    Credentials: &cdktn.DirectCredentials{Username: "admin", Password: "pass"},
    Roles: []cdktn.RoleConfig{{
        Name:     "FailoverRole",
        Database: "admin",
        Privileges: []cdktn.Privilege{{
            Cluster: true,
            Actions: []string{"replSetGetConfig", "replSetGetStatus", "replSetStateChange"},
        }},
    }},
    Users: []cdktn.UserConfig{{
        Username: "failover-agent",
        Password: "agent-pass",
        Database: "admin",
        Roles:    []cdktn.UserRoleRef{{Role: "FailoverRole", DB: "admin"}},
    }},
})
if err != nil {
    log.Fatal(err)
}

data, _ := stack.Synth()
os.WriteFile("shard.tf.json", data, 0o644)
```

Mongos and config server work the same way:

```go
stack := cdktn.NewTerraformStack(">= 1.7.5", "9.9.9")

// Config server — requires >= 3 members, generates shard_config, direct=true
cs, err := cdktn.NewMongoConfigServer(stack, "my-csrs", &cdktn.ConfigServerProps{
    ReplicaSetName: "csrs",
    Members: []cdktn.MemberConfig{
        {Host: "cfg1", Port: 27019},
        {Host: "cfg2", Port: 27019},
        {Host: "cfg3", Port: 27019},
    },
    Credentials: &cdktn.DirectCredentials{Username: "admin", Password: "pass"},
})

// Mongos — no minimum member count, no shard_config, direct=false
mongos, err := cdktn.NewMongoMongos(stack, "my-mongos", &cdktn.MongosProps{
    Members: []cdktn.MemberConfig{
        {Host: "mongos1", Port: 27017},
    },
    Credentials: &cdktn.DirectCredentials{Username: "admin", Password: "pass"},
    Users: []cdktn.UserConfig{{
        Username: "app",
        Password: "app-pass",
        Database: "admin",
        Roles:    []cdktn.UserRoleRef{{Role: "readWrite", DB: "mydb"}},
    }},
})
```

## What Gets Generated

For a minimal cluster (3 config servers, 3 shard members, 1 mongos), the library generates:

- **7 provider aliases** (3 configsvr + 3 shard + 1 mongos), each with host, port, credentials, SSL, direct mode
- **1 `mongodb_shard_config`** per replica set (config server + each shard)
- **`mongodb_db_role`** resources per role per member
- **`mongodb_db_user`** resources per user per member, with `depends_on` referencing roles

See `testdata/cluster_minimal.json` for a complete example of synthesized output.

## Validation

Constructors return errors for invalid configurations:

| Rule | Error |
|------|-------|
| Config server or shard < 3 members | `"configsvr requires minimum 3 members, got 2"` |
| Any component > 50 members | `"shard exceeds maximum 50 members, got 51"` |
| Duplicate host:port in same component | `"duplicate member host:port h1:27018"` |
| Duplicate replica set names | `"duplicate replica set name \"shard01\""` |
| No mongos in cluster | `"at least one mongos instance is required"` |
| No shards in cluster | `"at least one shard is required"` |
| Empty replica set name | `"replica set name MUST not be empty"` |
| > 7 members (warning, not error) | Logged via `log.Printf` |

## Build

```bash
make cdktn-build    # compile
make cdktn-test     # run 106 unit tests
make cdktn-test-golden  # regenerate golden files after intentional changes
```

Terraform is managed via [Hermit](https://github.com/cashapp/hermit) (`.hermit/`).

## Architecture

See [docs/IMPLEMENTATION.md](../docs/IMPLEMENTATION.md) for design decisions, EARS requirement
traceability, and future work.
