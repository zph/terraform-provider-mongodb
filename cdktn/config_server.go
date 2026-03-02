package cdktn

import "log"

// MongoConfigServer is an L2 construct representing a config server replica set.
// Same pattern as MongoShard but with componentType=configsvr. // CDKTN-001
type MongoConfigServer struct {
	ReplicaSetName string
	Aliases        []string
}

// NewMongoConfigServer validates props and adds all provider/resource blocks to the stack.
// CDKTN-001, CDKTN-021, CDKTN-025, CDKTN-035
func NewMongoConfigServer(stack *TerraformStack, id string, props *ConfigServerProps) (*MongoConfigServer, error) {
	if err := ValidateReplicaSetName(props.ReplicaSetName); err != nil {
		return nil, err
	}
	if err := ValidateReplicaSetMembers(ComponentTypeConfigServer, props.Members); err != nil {
		return nil, err
	}
	if err := ValidateDuplicateHostPort(props.Members); err != nil {
		return nil, err
	}

	WarnVotingMemberCount(log.Default(), props.Members)

	aliases := BuildProviders(stack, ComponentTypeConfigServer, props.ReplicaSetName, props.Members, props.Credentials, props.SSL, props.Proxy)

	roleDeps := BuildRoles(stack, aliases, props.Roles)
	BuildUsers(stack, aliases, props.Users, roleDeps)
	BuildShardConfig(stack, props.ReplicaSetName, aliases[0], props.ShardConfig)

	return &MongoConfigServer{
		ReplicaSetName: props.ReplicaSetName,
		Aliases:        aliases,
	}, nil
}
