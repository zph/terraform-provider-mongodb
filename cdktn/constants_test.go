package cdktn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// CDKTN-026: MaxMembers = 50
// CDKTN-027: MaxVotingMembers = 7
// CDKTN-016: Default shard config values
func TestConstants_MongoDBLimits(t *testing.T) {
	assert.Equal(t, 50, MaxMembers)
	assert.Equal(t, 7, MaxVotingMembers)
	assert.Equal(t, 1, MinVotingMembers)
	assert.Equal(t, 3, MinReplicaSetMembers)
}

func TestConstants_DefaultPriority(t *testing.T) {
	assert.Equal(t, 2, DefaultPriority)
	assert.Equal(t, 1, DefaultVotes)
}

func TestConstants_ShardConfigDefaults(t *testing.T) {
	assert.True(t, DefaultChainingAllowed)
	assert.Equal(t, 1000, DefaultHeartbeatIntervalMillis)
	assert.Equal(t, 10, DefaultHeartbeatTimeoutSecs)
	assert.Equal(t, 10000, DefaultElectionTimeoutMillis)
}

func TestConstants_ComponentTypes(t *testing.T) {
	assert.Equal(t, "shard", ComponentTypeShard.String())
	assert.Equal(t, "configsvr", ComponentTypeConfigServer.String())
	assert.Equal(t, "mongos", ComponentTypeMongos.String())
}

func TestConstants_ProviderSource(t *testing.T) {
	assert.Equal(t, "registry.terraform.io/zph/mongodb", ProviderSource)
}

func TestConstants_Defaults(t *testing.T) {
	assert.Equal(t, "admin", DefaultAuthDatabase)
	assert.Equal(t, 27017, DefaultPort)
	assert.Equal(t, ">= 1.7.5", DefaultTerraformVersion)
}
