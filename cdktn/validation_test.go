package cdktn

import (
	"bytes"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func threeMembers() []MemberConfig {
	return []MemberConfig{
		{Host: "h1", Port: 27018},
		{Host: "h2", Port: 27019},
		{Host: "h3", Port: 27020},
	}
}

// CDKTN-021: ConfigServer with fewer than 3 members returns error
func TestValidateConfigServer_TooFewMembers(t *testing.T) {
	err := ValidateReplicaSetMembers(ComponentTypeConfigServer, []MemberConfig{
		{Host: "h1", Port: 27018},
		{Host: "h2", Port: 27019},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "minimum")
	assert.Contains(t, err.Error(), "3")
}

// CDKTN-022: Shard with fewer than 3 members returns error
func TestValidateShard_TooFewMembers(t *testing.T) {
	err := ValidateReplicaSetMembers(ComponentTypeShard, []MemberConfig{
		{Host: "h1", Port: 27018},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "minimum")
}

func TestValidateReplicaSet_ExactlyThreeMembers_OK(t *testing.T) {
	err := ValidateReplicaSetMembers(ComponentTypeShard, threeMembers())
	assert.NoError(t, err)
}

// CDKTN-026: More than 50 members returns error
func TestValidateReplicaSet_TooManyMembers(t *testing.T) {
	members := make([]MemberConfig, 51)
	for i := range members {
		members[i] = MemberConfig{Host: "h", Port: 27018 + i}
	}
	err := ValidateReplicaSetMembers(ComponentTypeShard, members)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum")
	assert.Contains(t, err.Error(), "50")
}

// Mongos has no minimum member count
func TestValidateReplicaSet_MongosNoMinimum(t *testing.T) {
	err := ValidateReplicaSetMembers(ComponentTypeMongos, []MemberConfig{
		{Host: "h1", Port: 27017},
	})
	assert.NoError(t, err)
}

// CDKTN-025: Duplicate host:port returns error
func TestValidateDuplicateHostPort(t *testing.T) {
	members := []MemberConfig{
		{Host: "h1", Port: 27018},
		{Host: "h2", Port: 27019},
		{Host: "h1", Port: 27018},
	}
	err := ValidateDuplicateHostPort(members)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "h1:27018")
	assert.Contains(t, err.Error(), "duplicate")
}

func TestValidateDuplicateHostPort_NoDuplicates(t *testing.T) {
	err := ValidateDuplicateHostPort(threeMembers())
	assert.NoError(t, err)
}

// CDKTN-028: Duplicate replica set names returns error
func TestValidateDuplicateRSNames(t *testing.T) {
	names := []string{"shard01", "shard02", "shard01"}
	err := ValidateDuplicateRSNames(names)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shard01")
	assert.Contains(t, err.Error(), "duplicate")
}

func TestValidateDuplicateRSNames_NoDuplicates(t *testing.T) {
	names := []string{"shard01", "shard02", "csrs"}
	err := ValidateDuplicateRSNames(names)
	assert.NoError(t, err)
}

// CDKTN-027: Warning when more than 7 voting members
func TestValidateVotingMembers_WarnsOverSeven(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	members := make([]MemberConfig, 8)
	for i := range members {
		members[i] = MemberConfig{Host: "h", Port: 27018 + i}
	}
	WarnVotingMemberCount(logger, members)
	assert.Contains(t, buf.String(), "voting member limit")
	assert.Contains(t, buf.String(), "7")
}

func TestValidateVotingMembers_NoWarnAtSeven(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	members := make([]MemberConfig, 7)
	for i := range members {
		members[i] = MemberConfig{Host: "h", Port: 27018 + i}
	}
	WarnVotingMemberCount(logger, members)
	assert.Empty(t, buf.String())
}

// CDKTN-023: Empty mongos slice returns error
func TestValidateCluster_NoMongos(t *testing.T) {
	err := ValidateClusterMongos([]MongosConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mongos")
}

// CDKTN-024: Empty shards slice returns error
func TestValidateCluster_NoShards(t *testing.T) {
	err := ValidateClusterShards([]ShardConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shard")
}

func TestValidateCluster_ValidMongos(t *testing.T) {
	err := ValidateClusterMongos([]MongosConfig{
		{Members: []MemberConfig{{Host: "m1", Port: 27017}}},
	})
	assert.NoError(t, err)
}

func TestValidateCluster_ValidShards(t *testing.T) {
	err := ValidateClusterShards([]ShardConfig{
		{ReplicaSetName: "s1", Members: threeMembers()},
	})
	assert.NoError(t, err)
}

func TestValidateReplicaSetName_Empty(t *testing.T) {
	err := ValidateReplicaSetName("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "replica set name")
}

func TestValidateReplicaSetName_NonEmpty(t *testing.T) {
	err := ValidateReplicaSetName("shard01")
	assert.NoError(t, err)
}
