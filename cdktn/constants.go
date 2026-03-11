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

// Resource type constants for all provider resources.
const (
	ResourceTypeOriginalUser    = "mongodb_original_user"                 // CDKTN-052
	ResourceTypeShard           = "mongodb_shard"                         // CLUS-001
	ResourceTypeBalancerConfig  = "mongodb_balancer_config"               // BAL-001
	ResourceTypeShardZone       = "mongodb_shard_zone"                    // ZONE-001
	ResourceTypeZoneKeyRange    = "mongodb_zone_key_range"                // ZONE-014
	ResourceTypeCollBalancing   = "mongodb_collection_balancing"          // CBAL-001
	ResourceTypeProfiler        = "mongodb_profiler"                      // PROF-001
	ResourceTypeServerParameter = "mongodb_server_parameter"              // PARAM-001
	ResourceTypeFCV             = "mongodb_feature_compatibility_version" // FCV-001
)

// Default remove timeout for shard removal (matches provider default).
const DefaultRemoveTimeoutSecs = 300

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
