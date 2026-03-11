package cdktn

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configServerProps() *ConfigServerProps {
	return &ConfigServerProps{
		ReplicaSetName: "csrs",
		Members: []MemberConfig{
			{Host: "cfg1", Port: 27019},
			{Host: "cfg2", Port: 27020},
			{Host: "cfg3", Port: 27021},
		},
		Credentials: &DirectCredentials{Username: "admin", Password: "pass"},
		Roles: []RoleConfig{
			{Name: "ConfigRole", Database: "admin"},
		},
		Users: []UserConfig{
			{Username: "monitor", Password: "mon", Database: "admin"},
		},
	}
}

// CDKTN-001: MongoConfigServer is an exported struct
func TestNewMongoConfigServer_ReturnsNonNil(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	cs, err := NewMongoConfigServer(stack, "test-cs", configServerProps())
	require.NoError(t, err)
	require.NotNil(t, cs)
}

// CDKTN-021: Fewer than 3 members returns error
func TestNewMongoConfigServer_TooFewMembers(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	props := configServerProps()
	props.Members = props.Members[:2]
	_, err := NewMongoConfigServer(stack, "test-cs", props)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "minimum")
	assert.Contains(t, err.Error(), "3")
}

// Alias naming uses configsvr prefix
func TestNewMongoConfigServer_AliasNaming(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	cs, err := NewMongoConfigServer(stack, "test-cs", configServerProps())
	require.NoError(t, err)
	assert.Equal(t, []string{"configsvr_csrs_0", "configsvr_csrs_1", "configsvr_csrs_2"}, cs.Aliases)
}

// CDKTN-035: Config server providers have direct=true
func TestNewMongoConfigServer_DirectTrue(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	_, err := NewMongoConfigServer(stack, "test-cs", configServerProps())
	require.NoError(t, err)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	providers := m["provider"].(map[string]interface{})
	for _, p := range providers["mongodb"].([]interface{}) {
		pMap := p.(map[string]interface{})
		assert.Equal(t, true, pMap["direct"])
	}
}

// CDKTN-015: Shard config resource generated
func TestNewMongoConfigServer_HasShardConfig(t *testing.T) {
	stack := NewTerraformStack("", "1.0.0")
	_, err := NewMongoConfigServer(stack, "test-cs", configServerProps())
	require.NoError(t, err)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	configs := resources["mongodb_shard_config"].(map[string]interface{})
	assert.Len(t, configs, 1)
}

func TestNewMongoConfigServer_GoldenFile(t *testing.T) {
	stack := NewTerraformStack(">= 1.7.5", "9.9.9")
	_, err := NewMongoConfigServer(stack, "test-cs", configServerProps())
	require.NoError(t, err)

	data, err := stack.Synth()
	require.NoError(t, err)
	assert.True(t, json.Valid(data))
	goldenCompare(t, data)
}
