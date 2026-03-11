# cdktn Usage Patterns

Go-based equivalents of the Terraform HCL patterns in [`examples/patterns/`](../../examples/patterns/). These demonstrate how to use the `cdktn` construct library to generate Terraform JSON for MongoDB sharded clusters.

## Patterns

| Pattern | Go Source | HCL Equivalent | Description |
|---------|-----------|----------------|-------------|
| [Full Cluster Setup](patterns/full-cluster-setup/main.go) | L3 `MongoShardedCluster` | [`examples/patterns/full-cluster-setup/`](../../examples/patterns/full-cluster-setup/main.tf) | End-to-end: bootstrap, RS config, shard registration, balancer, zones, roles, users |
| [Sharded Cluster](patterns/sharded-cluster/main.go) | L3 `MongoShardedCluster` | [`examples/patterns/sharded-cluster/`](../../examples/patterns/sharded-cluster/main.tf) | SSL/TLS + custom roles + users (assumes credentials exist) |
| [Monitoring User](patterns/monitoring-user/main.go) | L2 `MongoMongos` | [`examples/patterns/monitoring-user/`](../../examples/patterns/monitoring-user/main.tf) | Least-privilege monitoring role + exporter user |
| [Role Hierarchy](patterns/role-hierarchy/main.go) | L2 `MongoMongos` | [`examples/patterns/role-hierarchy/`](../../examples/patterns/role-hierarchy/main.tf) | 3-tier role inheritance (viewer → editor → admin) |
| [Add Replicaset](patterns/add-replicaset-to-cluster/main.go) | L2 `MongoShard` | [`examples/patterns/add-replicaset-to-cluster/`](../../examples/patterns/add-replicaset-to-cluster/main.tf) | Day-2 scale-out: bootstrap new RS and configure membership |
| [Zone Sharding](patterns/zone-sharding/main.go) | L3 `MongoShardedCluster` | — | Shard-to-zone mapping, key range routing, multi-zone shards, compound keys |
| [Collection Balancing](patterns/collection-balancing/main.go) | L3 `MongoShardedCluster` | — | Per-collection balancing enable/disable, chunk size overrides |

## Construct Hierarchy

```
L3: MongoShardedCluster        — Full cluster (composes all L2s on a shared stack)
    ├── L2: MongoConfigServer   — Config server replica set
    ├── L2: MongoShard          — Shard replica set (one per shard)
    └── L2: MongoMongos         — Mongos query routers
```

**L3** (`MongoShardedCluster`) is the high-level construct for managing an entire sharded topology. It handles credential cascading, role merging, user propagation, shard registration, balancer config, and zone sharding.

**L2** constructs (`MongoShard`, `MongoConfigServer`, `MongoMongos`) can be used standalone for single-component management — useful for day-2 operations like adding a shard or managing users on an existing mongos.

## Quick Start

```go
package main

import (
    "fmt"
    "log"

    "github.com/zph/terraform-provider-mongodb/cdktn"
)

func main() {
    cluster, err := cdktn.NewMongoShardedCluster("mycluster", &cdktn.MongoShardedClusterProps{
        Credentials: &cdktn.DirectCredentials{
            Username: "admin",
            Password: "changeme",
        },
        ConfigServers: cdktn.ConfigServerConfig{
            ReplicaSetName: "configRS",
            Members: []cdktn.MemberConfig{
                {Host: "cfg0.example.com", Port: 27019},
                {Host: "cfg1.example.com", Port: 27019},
                {Host: "cfg2.example.com", Port: 27019},
            },
        },
        Shards: []cdktn.ShardConfig{
            {
                ReplicaSetName: "shard01",
                Members: []cdktn.MemberConfig{
                    {Host: "sh1a.example.com", Port: 27018},
                    {Host: "sh1b.example.com", Port: 27018},
                    {Host: "sh1c.example.com", Port: 27018},
                },
            },
        },
        Mongos: []cdktn.MongosConfig{
            {
                Members: []cdktn.MemberConfig{
                    {Host: "mongos0.example.com", Port: 27017},
                },
            },
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    data, _ := cluster.Stack.Synth()
    fmt.Println(string(data))
}
```

## Running Examples

Each pattern is a standalone `main.go` that prints Terraform JSON to stdout. Set the required environment variables and run:

```bash
cd cdktn/examples/patterns/monitoring-user
MONGO_PASSWORD=secret EXPORTER_PASSWORD=metrics go run main.go
```

Pipe to a file for use with Terraform:

```bash
MONGO_PASSWORD=secret EXPORTER_PASSWORD=metrics go run main.go > main.tf.json
terraform init && terraform plan
```

## Key Concepts

### Credentials

Two credential strategies are available:

```go
// Literal values — emitted directly into provider config
creds := &cdktn.DirectCredentials{Username: "admin", Password: "secret"}

// Environment variable fallthrough — provider uses its EnvDefaultFunc
creds := &cdktn.EnvCredentials{}
```

Per-member overrides are supported via `MemberConfig.Credentials`:

```go
Members: []cdktn.MemberConfig{
    {Host: "h1", Port: 27018},
    {Host: "h2", Port: 27018, Credentials: &cdktn.DirectCredentials{
        Username: "special",
        Password: "different",
    }},
}
```

### SSL/TLS

Set at cluster level to propagate to all components:

```go
SSL: &cdktn.SSLConfig{
    Enabled:            true,
    Certificate:        pemCertString,
    InsecureSkipVerify: false,
}
```

### Shard Config Defaults

When `ShardConfig` is nil, `DefaultShardConfigSettings()` is used:

| Setting | Default |
|---------|---------|
| `ChainingAllowed` | `true` |
| `HeartbeatIntervalMillis` | `1000` |
| `HeartbeatTimeoutSecs` | `10` |
| `ElectionTimeoutMillis` | `10000` |

### Provider Alias Naming

Aliases are generated deterministically:

| Component | Pattern | Example |
|-----------|---------|---------|
| Shard | `shard_<rsname>_<idx>` | `shard_shard01_0` |
| Config Server | `configsvr_<rsname>_<idx>` | `configsvr_configRS_0` |
| Mongos | `mongos_<idx>` | `mongos_0` |

### Cluster-Level User Propagation

Users defined in `MongoShardedClusterProps.Users` are created on:
- All mongos aliases (for client access)
- First alias of each shard (primary, for direct shard operations)

Component-level users (defined in `ShardConfig.Users`, `MongosConfig.Users`, etc.) are scoped to that component's aliases only.
