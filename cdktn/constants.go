package cdktn

// MongoDB replica set limits — duplicated from mongodb/replica_set_types.go
// to avoid importing the parent module (separate Go modules). // CDKTN-026, CDKTN-027
const (
	MinVotingMembers     = 1
	MaxVotingMembers     = 7
	MaxMembers           = 50
	MinReplicaSetMembers = 3
	DefaultPriority      = 2
	DefaultVotes         = 1
)

// Shard config defaults — match provider schema (resource_shard_config.go:168-188). // CDKTN-016
const (
	DefaultChainingAllowed         = true
	DefaultHeartbeatIntervalMillis = 1000
	DefaultHeartbeatTimeoutSecs    = 10
	DefaultElectionTimeoutMillis   = 10000
)

// Provider metadata. // CDKTN-006
const (
	ProviderSource          = "registry.terraform.io/zph/mongodb"
	ProviderType            = "mongodb"
	DefaultAuthDatabase     = "admin"
	DefaultPort             = 27017
	DefaultTerraformVersion = ">= 1.7.5"
)

// ComponentType identifies the role of a MongoDB node in a sharded cluster.
type ComponentType int

const (
	ComponentTypeShard ComponentType = iota
	ComponentTypeConfigServer
	ComponentTypeMongos
)

var componentTypeStrings = map[ComponentType]string{
	ComponentTypeShard:        "shard",
	ComponentTypeConfigServer: "configsvr",
	ComponentTypeMongos:       "mongos",
}

func (c ComponentType) String() string {
	if s, ok := componentTypeStrings[c]; ok {
		return s
	}
	return "unknown"
}
