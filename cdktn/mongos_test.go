package cdktn

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mongosProps() *MongosProps {
	return &MongosProps{
		Members: []MemberConfig{
			{Host: "mongos1", Port: 27017},
			{Host: "mongos2", Port: 27017},
		},
		Credentials: &DirectCredentials{Username: "admin", Password: "pass"},
		Roles: []RoleConfig{
			{Name: "AppRole", Database: "admin"},
		},
		Users: []UserConfig{
			{Username: "appuser", Password: "secret", Database: "admin"},
		},
	}
}

// CDKTN-001: MongoMongos is an exported struct
func TestNewMongoMongos_ReturnsNonNil(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	mongos, err := NewMongoMongos(stack, "test-mongos", mongosProps())
	require.NoError(t, err)
	require.NotNil(t, mongos)
}

// Mongos has no minimum member count — single member is valid
func TestNewMongoMongos_SingleMember(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	props := mongosProps()
	props.Members = props.Members[:1]
	_, err := NewMongoMongos(stack, "test-mongos", props)
	require.NoError(t, err)
}

// CDKTN-036: Mongos provider aliases have direct=false
func TestNewMongoMongos_DirectFalse(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	_, err := NewMongoMongos(stack, "test-mongos", mongosProps())
	require.NoError(t, err)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	providers := m["provider"].(map[string]interface{})
	for _, p := range providers["mongodb"].([]interface{}) {
		pMap := p.(map[string]interface{})
		assert.Equal(t, false, pMap["direct"])
	}
}

// CDKTN-017: No shard_config for mongos
func TestNewMongoMongos_NoShardConfig(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	_, err := NewMongoMongos(stack, "test-mongos", mongosProps())
	require.NoError(t, err)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	_, hasShardConfig := resources["mongodb_shard_config"]
	assert.False(t, hasShardConfig, "mongos MUST NOT have shard_config resources")
}

// Mongos alias naming: mongos_<index>
func TestNewMongoMongos_AliasNaming(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	mongos, err := NewMongoMongos(stack, "test-mongos", mongosProps())
	require.NoError(t, err)
	assert.Equal(t, []string{"mongos_0", "mongos_1"}, mongos.Aliases)
}

// Roles and users are generated per member
func TestNewMongoMongos_RolesAndUsers(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	_, err := NewMongoMongos(stack, "test-mongos", mongosProps())
	require.NoError(t, err)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	roles := resources["mongodb_db_role"].(map[string]interface{})
	users := resources["mongodb_db_user"].(map[string]interface{})
	assert.Len(t, roles, 2, "1 role x 2 members")
	assert.Len(t, users, 2, "1 user x 2 members")
}

func TestNewMongoMongos_GoldenFile(t *testing.T) {
	stack := NewTerraformStack(">= 1.7.5", "9.9.9")
	_, err := NewMongoMongos(stack, "test-mongos", mongosProps())
	require.NoError(t, err)

	data, err := stack.Synth()
	require.NoError(t, err)
	assert.True(t, json.Valid(data))
	goldenCompare(t, "mongos_basic.json", data)
}

// CDKTN-025: Duplicate host:port
func TestNewMongoMongos_DuplicateHostPort(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	props := mongosProps()
	props.Members[1] = props.Members[0]
	_, err := NewMongoMongos(stack, "test-mongos", props)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}
