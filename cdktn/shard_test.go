package cdktn

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func shardProps() *MongoShardProps {
	return &MongoShardProps{
		ReplicaSetName: "shard01",
		Members: []MemberConfig{
			{Host: "s1m1", Port: 27018},
			{Host: "s1m2", Port: 27019},
			{Host: "s1m3", Port: 27020},
		},
		Credentials: &DirectCredentials{Username: "admin", Password: "pass"},
		Roles: []RoleConfig{
			{
				Name:     "FailoverRole",
				Database: "admin",
				Privileges: []Privilege{
					{Cluster: true, Actions: []string{"replSetGetConfig", "replSetGetStatus"}},
				},
			},
		},
		Users: []UserConfig{
			{
				Username: "appuser",
				Password: "secret",
				Database: "admin",
				Roles:    []UserRoleRef{{Role: "readWrite", DB: "mydb"}},
			},
		},
	}
}

// CDKTN-001: MongoShard is an exported struct with New constructor
func TestNewMongoShard_ReturnsNonNil(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	shard, err := NewMongoShard(stack, "test-shard", shardProps())
	require.NoError(t, err)
	require.NotNil(t, shard)
}

// CDKTN-022: Fewer than 3 members returns error
func TestNewMongoShard_TooFewMembers(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	props := shardProps()
	props.Members = props.Members[:1]
	_, err := NewMongoShard(stack, "test-shard", props)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "minimum")
}

// CDKTN-004: Provider aliases match pattern
func TestNewMongoShard_ProviderAliases(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	shard, err := NewMongoShard(stack, "test-shard", shardProps())
	require.NoError(t, err)

	assert.Equal(t, []string{"shard_shard01_0", "shard_shard01_1", "shard_shard01_2"}, shard.Aliases)
}

// CDKTN-003: 3 members produce 3 provider alias blocks
func TestNewMongoShard_ThreeProviderBlocks(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	_, err := NewMongoShard(stack, "test-shard", shardProps())
	require.NoError(t, err)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	providers := m["provider"].(map[string]interface{})
	mongodbProviders := providers["mongodb"].([]interface{})
	assert.Len(t, mongodbProviders, 3)
}

// CDKTN-012: Role resources generated for each member
func TestNewMongoShard_RolesPerMember(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	_, err := NewMongoShard(stack, "test-shard", shardProps())
	require.NoError(t, err)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	roles := resources["mongodb_db_role"].(map[string]interface{})
	assert.Len(t, roles, 3, "1 role x 3 members = 3 role resources")
}

// CDKTN-011: User resources generated for each member
func TestNewMongoShard_UsersPerMember(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	_, err := NewMongoShard(stack, "test-shard", shardProps())
	require.NoError(t, err)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	users := resources["mongodb_db_user"].(map[string]interface{})
	assert.Len(t, users, 3, "1 user x 3 members = 3 user resources")
}

// CDKTN-013: User depends_on includes role resources
func TestNewMongoShard_UserDependsOnRoles(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	_, err := NewMongoShard(stack, "test-shard", shardProps())
	require.NoError(t, err)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	users := resources["mongodb_db_user"].(map[string]interface{})
	// Check first member's user
	u := users["shard_shard01_0_user_appuser"].(map[string]interface{})
	deps := u["depends_on"].([]interface{})
	assert.Contains(t, deps, "mongodb_db_role.shard_shard01_0_role_FailoverRole")
}

// CDKTN-015: One shard_config per replica set
func TestNewMongoShard_SingleShardConfig(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	_, err := NewMongoShard(stack, "test-shard", shardProps())
	require.NoError(t, err)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	configs := resources["mongodb_shard_config"].(map[string]interface{})
	assert.Len(t, configs, 1)
}

// CDKTN-016: Shard config defaults
func TestNewMongoShard_DefaultShardConfigSettings(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	props := shardProps()
	props.ShardConfig = nil // use defaults
	_, err := NewMongoShard(stack, "test-shard", props)
	require.NoError(t, err)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	configs := resources["mongodb_shard_config"].(map[string]interface{})
	cfg := configs["shard01_config"].(map[string]interface{})
	assert.Equal(t, true, cfg["chaining_allowed"])
	assert.Equal(t, float64(1000), cfg["heartbeat_interval_millis"])
	assert.Equal(t, float64(10), cfg["heartbeat_timeout_secs"])
	assert.Equal(t, float64(10000), cfg["election_timeout_millis"])
}

// CDKTN-035: Provider aliases have direct=true
func TestNewMongoShard_DirectTrue(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	_, err := NewMongoShard(stack, "test-shard", shardProps())
	require.NoError(t, err)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	providers := m["provider"].(map[string]interface{})
	for _, p := range providers["mongodb"].([]interface{}) {
		pMap := p.(map[string]interface{})
		assert.Equal(t, true, pMap["direct"])
	}
}

// CDKTN-025: Duplicate host:port error
func TestNewMongoShard_DuplicateHostPort(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	props := shardProps()
	props.Members[2] = props.Members[0] // duplicate
	_, err := NewMongoShard(stack, "test-shard", props)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

// Empty RS name error
func TestNewMongoShard_EmptyRSName(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	props := shardProps()
	props.ReplicaSetName = ""
	_, err := NewMongoShard(stack, "test-shard", props)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "replica set name")
}

// Golden file test // CDKTN-043
func TestNewMongoShard_GoldenFile(t *testing.T) {
	stack := NewTerraformStack(">= 1.7.5", "9.9.9")
	_, err := NewMongoShard(stack, "test-shard", shardProps())
	require.NoError(t, err)

	data, err := stack.Synth()
	require.NoError(t, err)

	// Verify output is valid JSON
	assert.True(t, json.Valid(data))

	// If golden file exists, compare. Otherwise write it.
	goldenCompare(t, "shard_basic.json", data)
}
