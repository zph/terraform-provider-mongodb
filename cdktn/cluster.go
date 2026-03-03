package cdktn

import "fmt"

// MongoShardedCluster is an L3 construct that composes MongoMongos, MongoConfigServer,
// and MongoShard L2 constructs into a complete sharded cluster topology.
// CDKTN-002
type MongoShardedCluster struct {
	Stack        *TerraformStack
	MongosL2s    []*MongoMongos
	ConfigServer *MongoConfigServer
	Shards       []*MongoShard
}

// NewMongoShardedCluster validates cluster-level invariants, then creates
// all L2 constructs on a shared stack. // CDKTN-002, CDKTN-009, CDKTN-023, CDKTN-024, CDKTN-028, CDKTN-037
func NewMongoShardedCluster(id string, props *MongoShardedClusterProps) (*MongoShardedCluster, error) {
	// Cluster-level validation
	if err := ValidateClusterMongos(props.Mongos); err != nil {
		return nil, err
	}
	if err := ValidateClusterShards(props.Shards); err != nil {
		return nil, err
	}

	// CDKTN-028: Collect all RS names (config server + shards) and check for duplicates
	rsNames := make([]string, 0, 1+len(props.Shards))
	rsNames = append(rsNames, props.ConfigServers.ReplicaSetName)
	for _, s := range props.Shards {
		rsNames = append(rsNames, s.ReplicaSetName)
	}
	if err := ValidateDuplicateRSNames(rsNames); err != nil {
		return nil, err
	}

	// Create the shared stack
	providerVersion := props.ProviderVersion
	if providerVersion == "" {
		providerVersion = "9.9.9"
	}
	stack := NewTerraformStack(DefaultTerraformVersion, providerVersion)

	cluster := &MongoShardedCluster{
		Stack: stack,
	}

	// Build config server L2 // CDKTN-009: cascade credentials, SSL, proxy
	cs, err := NewMongoConfigServer(stack, fmt.Sprintf("%s-configsvr", id), &ConfigServerProps{
		ReplicaSetName: props.ConfigServers.ReplicaSetName,
		Members:        props.ConfigServers.Members,
		Credentials:    props.Credentials,
		SSL:            props.SSL,
		Proxy:          props.Proxy,
		Users:          props.ConfigServers.Users,
		Roles:          mergeRoles(props.Roles, props.ConfigServers.Roles),
		ShardConfig:    props.ConfigServers.ShardConfig,
		OriginalUsers:  props.ConfigServers.OriginalUsers, // CDKTN-052
	})
	if err != nil {
		return nil, fmt.Errorf("config server: %w", err)
	}
	cluster.ConfigServer = cs

	// Build shard L2s
	for i, shardCfg := range props.Shards {
		shard, err := NewMongoShard(stack, fmt.Sprintf("%s-shard-%d", id, i), &MongoShardProps{
			ReplicaSetName: shardCfg.ReplicaSetName,
			Members:        shardCfg.Members,
			Credentials:    props.Credentials,
			SSL:            props.SSL,
			Proxy:          props.Proxy,
			Users:          shardCfg.Users,
			Roles:          mergeRoles(props.Roles, shardCfg.Roles),
			ShardConfig:    shardCfg.ShardConfig,
			OriginalUsers:  shardCfg.OriginalUsers, // CDKTN-052
		})
		if err != nil {
			return nil, fmt.Errorf("shard %q: %w", shardCfg.ReplicaSetName, err)
		}
		cluster.Shards = append(cluster.Shards, shard)
	}

	// Build mongos L2s — sequential alias numbering across groups
	mongosOffset := 0
	for i, mongosCfg := range props.Mongos {
		mongos, err := NewMongoMongosWithOffset(stack, fmt.Sprintf("%s-mongos-%d", id, i), &MongosProps{
			Members:       mongosCfg.Members,
			Credentials:   props.Credentials,
			SSL:           props.SSL,
			Proxy:         props.Proxy,
			Users:         mongosCfg.Users,
			Roles:         mergeRoles(props.Roles, mongosCfg.Roles),
			OriginalUsers: mongosCfg.OriginalUsers, // CDKTN-052
		}, mongosOffset)
		if err != nil {
			return nil, fmt.Errorf("mongos %d: %w", i, err)
		}
		cluster.MongosL2s = append(cluster.MongosL2s, mongos)
		mongosOffset += len(mongosCfg.Members)
	}

	// CDKTN-037: Propagate cluster-level users to mongos aliases + shard primary aliases
	if len(props.Users) > 0 {
		clusterUserAliases := collectClusterUserTargets(cluster)
		// Cluster-level roles are already merged into L2s; we need role deps for user depends_on
		roleDeps := BuildRoles(stack, clusterUserAliases, props.Roles)
		BuildUsers(stack, clusterUserAliases, props.Users, roleDeps)
	}

	// CDKTN-052: Propagate cluster-level original users to first alias of each component
	if len(props.OriginalUsers) > 0 {
		targets := collectClusterOriginalUserTargets(cluster)
		for _, alias := range targets {
			BuildOriginalUsers(stack, alias, props.OriginalUsers)
		}
	}

	return cluster, nil
}

// collectClusterOriginalUserTargets returns first alias of each component
// (mongos, config server, shards) for cluster-level original user propagation. // CDKTN-052
func collectClusterOriginalUserTargets(cluster *MongoShardedCluster) []string {
	var aliases []string
	for _, m := range cluster.MongosL2s {
		if len(m.Aliases) > 0 {
			aliases = append(aliases, m.Aliases[0])
		}
	}
	if len(cluster.ConfigServer.Aliases) > 0 {
		aliases = append(aliases, cluster.ConfigServer.Aliases[0])
	}
	for _, s := range cluster.Shards {
		if len(s.Aliases) > 0 {
			aliases = append(aliases, s.Aliases[0])
		}
	}
	return aliases
}

// collectClusterUserTargets returns aliases that receive cluster-level users:
// all mongos aliases + first alias of each shard (primary). // CDKTN-037
func collectClusterUserTargets(cluster *MongoShardedCluster) []string {
	var aliases []string
	for _, m := range cluster.MongosL2s {
		aliases = append(aliases, m.Aliases...)
	}
	for _, s := range cluster.Shards {
		if len(s.Aliases) > 0 {
			aliases = append(aliases, s.Aliases[0])
		}
	}
	return aliases
}

// mergeRoles combines cluster-level roles with component-level roles.
func mergeRoles(clusterRoles, componentRoles []RoleConfig) []RoleConfig {
	if len(clusterRoles) == 0 {
		return componentRoles
	}
	if len(componentRoles) == 0 {
		return clusterRoles
	}
	merged := make([]RoleConfig, 0, len(clusterRoles)+len(componentRoles))
	merged = append(merged, clusterRoles...)
	merged = append(merged, componentRoles...)
	return merged
}
