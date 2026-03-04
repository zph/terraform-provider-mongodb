package cdktn

import "log"

// MongoShard is an L2 construct representing a MongoDB shard replica set.
// It generates provider aliases, roles, users, and shard_config resources
// for all members of a single shard. // CDKTN-001
type MongoShard struct {
	ReplicaSetName string
	Aliases        []string
}

// NewMongoShard validates props and adds all provider/resource blocks to the stack.
// Returns (*MongoShard, error) — Go-idiomatic error handling, not panics.
// CDKTN-001, CDKTN-004, CDKTN-011-016, CDKTN-022, CDKTN-025, CDKTN-035
func NewMongoShard(stack *TerraformStack, id string, props *MongoShardProps) (*MongoShard, error) {
	if err := ValidateReplicaSetName(props.ReplicaSetName); err != nil {
		return nil, err
	}
	if err := ValidateReplicaSetMembers(ComponentTypeShard, props.Members); err != nil {
		return nil, err
	}
	if err := ValidateDuplicateHostPort(props.Members); err != nil {
		return nil, err
	}

	WarnVotingMemberCount(log.Default(), props.Members)

	// CDKTN-004: Create provider aliases
	aliases := BuildProviders(stack, ComponentTypeShard, props.ReplicaSetName, props.Members, props.Credentials, props.SSL, props.Proxy)

	// CDKTN-012: Roles on each member
	roleDeps := BuildRoles(stack, aliases, props.Roles)

	// CDKTN-011, CDKTN-013: Users on each member with depends_on roles
	BuildUsers(stack, aliases, props.Users, roleDeps)

	// CDKTN-015: One shard_config targeting first member (primary)
	BuildShardConfig(stack, props.ReplicaSetName, aliases[0], props.ShardConfig)

	// CDKTN-052: Original users (bootstrap) targeting first member
	if len(props.OriginalUsers) > 0 {
		BuildOriginalUsers(stack, aliases[0], props.OriginalUsers)
	}

	return &MongoShard{
		ReplicaSetName: props.ReplicaSetName,
		Aliases:        aliases,
	}, nil
}
