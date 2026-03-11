package cdktn

// MongoMongos is an L2 construct representing one or more mongos query routers.
// Unlike shard/configsvr, mongos has no replica set, no shard_config, and uses
// direct=false. // CDKTN-001, CDKTN-017, CDKTN-036
type MongoMongos struct {
	Aliases []string
}

// NewMongoMongos validates props and adds provider/resource blocks to the stack.
// CDKTN-001, CDKTN-017, CDKTN-025, CDKTN-036
func NewMongoMongos(stack *TerraformStack, id string, props *MongosProps) (*MongoMongos, error) {
	return NewMongoMongosWithOffset(stack, id, props, 0)
}

// NewMongoMongosWithOffset is like NewMongoMongos but starts alias numbering
// at offset. Used by MongoShardedCluster when composing multiple mongos groups.
func NewMongoMongosWithOffset(stack *TerraformStack, id string, props *MongosProps, offset int) (*MongoMongos, error) {
	if err := ValidateReplicaSetMembers(ComponentTypeMongos, props.Members); err != nil {
		return nil, err
	}
	if err := ValidateDuplicateHostPort(props.Members); err != nil {
		return nil, err
	}

	// CDKTN-036: mongos uses direct=false (handled by BuildProviders via ComponentTypeMongos)
	aliases := BuildProvidersWithOffset(stack, ComponentTypeMongos, "", props.Members, props.Credentials, props.SSL, props.Proxy, offset)

	roleDeps := BuildRoles(stack, aliases, props.Roles)
	BuildUsers(stack, aliases, props.Users, roleDeps)

	// CDKTN-017: No shard_config for mongos

	// CDKTN-052: Original users (bootstrap) targeting first member
	if len(props.OriginalUsers) > 0 {
		BuildOriginalUsers(stack, aliases[0], props.OriginalUsers)
	}

	if len(props.Profilers) > 0 {
		BuildProfilers(stack, aliases, props.Profilers)
	}
	if len(props.ServerParameters) > 0 {
		BuildServerParameters(stack, aliases, props.ServerParameters)
	}
	// Mongos does not support FCV (only mongod processes have FCV)

	return &MongoMongos{
		Aliases: aliases,
	}, nil
}
