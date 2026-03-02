package cdktn

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CDKTN-029: Deterministic output — two synths produce identical bytes
func TestStack_DeterministicOutput(t *testing.T) {
	synth := func() []byte {
		s := NewTerraformStack(">= 1.7.5", ">= 1.0.0")
		s.AddProvider("shard_s1_0", map[string]interface{}{"host": "h1", "port": "27018"})
		s.AddProvider("shard_s1_1", map[string]interface{}{"host": "h2", "port": "27019"})
		s.AddResource("mongodb_db_user", "s1_0_user_app", map[string]interface{}{
			"name": "app", "password": "secret",
		}, "mongodb.shard_s1_0", nil)
		data, err := s.Synth()
		require.NoError(t, err)
		return data
	}

	a := synth()
	b := synth()
	assert.Equal(t, string(a), string(b))
}

// CDKTN-030: Single required_providers entry
func TestStack_SingleRequiredProviders(t *testing.T) {
	s := NewTerraformStack(">= 1.7.5", ">= 1.0.0")
	s.AddProvider("alias1", map[string]interface{}{"host": "h1"})
	s.AddProvider("alias2", map[string]interface{}{"host": "h2"})

	m, err := s.SynthToMap()
	require.NoError(t, err)

	tf := m["terraform"].(map[string]interface{})
	rp := tf["required_providers"].(map[string]interface{})
	assert.Len(t, rp, 1, "MUST have exactly one required_providers entry")
	assert.Contains(t, rp, "mongodb")
}

// CDKTN-031: Terraform version constraint
func TestStack_TerraformVersionConstraint(t *testing.T) {
	s := NewTerraformStack(">= 1.7.5", ">= 1.0.0")
	m, err := s.SynthToMap()
	require.NoError(t, err)

	tf := m["terraform"].(map[string]interface{})
	assert.Equal(t, ">= 1.7.5", tf["required_version"])
}

// CDKTN-006: Provider source
func TestStack_ProviderSource(t *testing.T) {
	s := NewTerraformStack("", "1.0.0")
	m, err := s.SynthToMap()
	require.NoError(t, err)

	tf := m["terraform"].(map[string]interface{})
	rp := tf["required_providers"].(map[string]interface{})
	mongodb := rp["mongodb"].(map[string]interface{})
	assert.Equal(t, ProviderSource, mongodb["source"])
}

// CDKTN-042: Provider version constraint
func TestStack_ProviderVersion(t *testing.T) {
	s := NewTerraformStack("", ">= 2.0.0")
	m, err := s.SynthToMap()
	require.NoError(t, err)

	tf := m["terraform"].(map[string]interface{})
	rp := tf["required_providers"].(map[string]interface{})
	mongodb := rp["mongodb"].(map[string]interface{})
	assert.Equal(t, ">= 2.0.0", mongodb["version"])
}

func TestStack_ProviderAliasesInOutput(t *testing.T) {
	s := NewTerraformStack("", "1.0.0")
	s.AddProvider("shard_s1_0", map[string]interface{}{"host": "h1", "port": "27018"})
	s.AddProvider("mongos_m1_0", map[string]interface{}{"host": "m1", "port": "27017"})

	m, err := s.SynthToMap()
	require.NoError(t, err)

	providers := m["provider"].(map[string]interface{})
	mongodbProviders := providers["mongodb"].([]interface{})
	assert.Len(t, mongodbProviders, 2)

	// Sorted by alias
	first := mongodbProviders[0].(map[string]interface{})
	second := mongodbProviders[1].(map[string]interface{})
	assert.Equal(t, "mongos_m1_0", first["alias"])
	assert.Equal(t, "shard_s1_0", second["alias"])
}

func TestStack_ResourcesGroupedByType(t *testing.T) {
	s := NewTerraformStack("", "1.0.0")
	s.AddResource("mongodb_db_role", "r1", map[string]interface{}{"name": "MyRole"}, "mongodb.a1", nil)
	s.AddResource("mongodb_db_user", "u1", map[string]interface{}{"name": "app"}, "mongodb.a1",
		[]string{"mongodb_db_role.r1"})

	m, err := s.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	assert.Contains(t, resources, "mongodb_db_role")
	assert.Contains(t, resources, "mongodb_db_user")

	users := resources["mongodb_db_user"].(map[string]interface{})
	u1 := users["u1"].(map[string]interface{})
	assert.Equal(t, "mongodb.a1", u1["provider"])

	deps := u1["depends_on"].([]interface{})
	assert.Contains(t, deps, "mongodb_db_role.r1")
}

func TestStack_NoResourcesOmitsResourceKey(t *testing.T) {
	s := NewTerraformStack("", "1.0.0")
	s.AddProvider("a1", map[string]interface{}{"host": "h1"})

	m, err := s.SynthToMap()
	require.NoError(t, err)
	assert.NotContains(t, m, "resource")
}

func TestStack_DefaultTerraformVersion(t *testing.T) {
	s := NewTerraformStack("", "1.0.0")
	m, err := s.SynthToMap()
	require.NoError(t, err)
	tf := m["terraform"].(map[string]interface{})
	assert.Equal(t, DefaultTerraformVersion, tf["required_version"])
}

func TestStack_SynthProducesValidJSON(t *testing.T) {
	s := NewTerraformStack(">= 1.7.5", "1.0.0")
	s.AddProvider("a1", map[string]interface{}{"host": "h1", "port": "27017"})
	data, err := s.Synth()
	require.NoError(t, err)
	assert.True(t, json.Valid(data))
}
