package cdktn

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func minimalClusterProps() *MongoShardedClusterProps {
	return &MongoShardedClusterProps{
		Mongos: []MongosConfig{
			{Members: []MemberConfig{{Host: "mongos1", Port: 27017}}},
		},
		ConfigServers: ConfigServerConfig{
			ReplicaSetName: "csrs",
			Members: []MemberConfig{
				{Host: "cfg1", Port: 27019},
				{Host: "cfg2", Port: 27020},
				{Host: "cfg3", Port: 27021},
			},
		},
		Shards: []ShardConfig{
			{
				ReplicaSetName: "shard01",
				Members: []MemberConfig{
					{Host: "s1m1", Port: 27018},
					{Host: "s1m2", Port: 27019},
					{Host: "s1m3", Port: 27020},
				},
			},
		},
		Credentials:     &DirectCredentials{Username: "admin", Password: "pass"},
		ProviderVersion: "9.9.9",
	}
}

func fullClusterProps() *MongoShardedClusterProps {
	p := minimalClusterProps()
	p.Mongos = append(p.Mongos, MongosConfig{
		Members: []MemberConfig{{Host: "mongos2", Port: 27017}},
	})
	p.Shards = append(p.Shards, ShardConfig{
		ReplicaSetName: "shard02",
		Members: []MemberConfig{
			{Host: "s2m1", Port: 27018},
			{Host: "s2m2", Port: 27019},
			{Host: "s2m3", Port: 27020},
		},
	})
	p.SSL = &SSLConfig{Enabled: true}
	p.Proxy = "socks5://proxy:1080"
	p.Roles = []RoleConfig{
		{Name: "StaffRole", Database: "admin"},
	}
	p.Users = []UserConfig{
		{Username: "appuser", Password: "secret", Database: "admin",
			Roles: []UserRoleRef{{Role: "readWrite", DB: "mydb"}}},
	}
	return p
}

// CDKTN-002: MongoShardedCluster is an exported struct
func TestNewMongoShardedCluster_ReturnsNonNil(t *testing.T) {
	cluster, err := NewMongoShardedCluster("test-cluster", minimalClusterProps())
	require.NoError(t, err)
	require.NotNil(t, cluster)
}

// CDKTN-023: Empty mongos returns error
func TestNewMongoShardedCluster_NoMongos(t *testing.T) {
	props := minimalClusterProps()
	props.Mongos = nil
	_, err := NewMongoShardedCluster("test-cluster", props)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mongos")
}

// CDKTN-024: Empty shards returns error
func TestNewMongoShardedCluster_NoShards(t *testing.T) {
	props := minimalClusterProps()
	props.Shards = nil
	_, err := NewMongoShardedCluster("test-cluster", props)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shard")
}

// CDKTN-028: Duplicate shard RS names returns error
func TestNewMongoShardedCluster_DuplicateShardNames(t *testing.T) {
	props := minimalClusterProps()
	props.Shards = append(props.Shards, ShardConfig{
		ReplicaSetName: "shard01", // duplicate
		Members: []MemberConfig{
			{Host: "s2m1", Port: 27028},
			{Host: "s2m2", Port: 27029},
			{Host: "s2m3", Port: 27030},
		},
	})
	_, err := NewMongoShardedCluster("test-cluster", props)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shard01")
}

// CDKTN-028: Shard name duplicating config server RS name
func TestNewMongoShardedCluster_ShardDuplicatesConfigServerName(t *testing.T) {
	props := minimalClusterProps()
	props.Shards[0].ReplicaSetName = "csrs" // same as config server
	_, err := NewMongoShardedCluster("test-cluster", props)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "csrs")
}

// CDKTN-009: Cluster-level credentials propagate to all components
func TestNewMongoShardedCluster_CredentialsCascade(t *testing.T) {
	cluster, err := NewMongoShardedCluster("test-cluster", minimalClusterProps())
	require.NoError(t, err)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	providers := m["provider"].(map[string]interface{})
	for _, p := range providers["mongodb"].([]interface{}) {
		pMap := p.(map[string]interface{})
		assert.Equal(t, "admin", pMap["username"])
		assert.Equal(t, "pass", pMap["password"])
	}
}

// CDKTN-018: SSL cascades to all providers
func TestNewMongoShardedCluster_SSLCascade(t *testing.T) {
	props := minimalClusterProps()
	props.SSL = &SSLConfig{Enabled: true}
	cluster, err := NewMongoShardedCluster("test-cluster", props)
	require.NoError(t, err)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	providers := m["provider"].(map[string]interface{})
	for _, p := range providers["mongodb"].([]interface{}) {
		pMap := p.(map[string]interface{})
		assert.Equal(t, true, pMap["ssl"])
	}
}

// CDKTN-037: Cluster-level users on mongos and shard primaries
func TestNewMongoShardedCluster_UserPropagation(t *testing.T) {
	props := fullClusterProps()
	cluster, err := NewMongoShardedCluster("test-cluster", props)
	require.NoError(t, err)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	users := resources["mongodb_db_user"].(map[string]interface{})

	// Cluster-level user should be on:
	// - mongos_0 (mongos1)
	// - mongos_1 (mongos2)
	// - shard_shard01_0 (first member of shard01)
	// - shard_shard02_0 (first member of shard02)
	assert.Contains(t, users, "mongos_0_user_appuser")
	assert.Contains(t, users, "mongos_1_user_appuser")
	assert.Contains(t, users, "shard_shard01_0_user_appuser")
	assert.Contains(t, users, "shard_shard02_0_user_appuser")
}

// CDKTN-038: L2-level users only on that construct's members
func TestNewMongoShardedCluster_L2UserScoping(t *testing.T) {
	props := minimalClusterProps()
	props.Shards[0].Users = []UserConfig{
		{Username: "shard_only", Password: "secret", Database: "admin"},
	}
	cluster, err := NewMongoShardedCluster("test-cluster", props)
	require.NoError(t, err)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	users := resources["mongodb_db_user"].(map[string]interface{})

	// shard_only should appear on shard01 members only
	assert.Contains(t, users, "shard_shard01_0_user_shard_only")
	assert.Contains(t, users, "shard_shard01_1_user_shard_only")
	assert.Contains(t, users, "shard_shard01_2_user_shard_only")

	// shard_only should NOT appear on mongos or config server
	assert.NotContains(t, users, "mongos_0_user_shard_only")
	assert.NotContains(t, users, "configsvr_csrs_0_user_shard_only")
}

// CDKTN-017: No shard_config on mongos
func TestNewMongoShardedCluster_NoShardConfigOnMongos(t *testing.T) {
	cluster, err := NewMongoShardedCluster("test-cluster", minimalClusterProps())
	require.NoError(t, err)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	configs := resources["mongodb_shard_config"].(map[string]interface{})

	// Should have shard_config for shard01 and csrs, but not mongos
	for name := range configs {
		assert.NotContains(t, name, "mongos")
	}
}

// Provider count: 1 mongos + 3 config + 3 shard = 7
func TestNewMongoShardedCluster_ProviderCount(t *testing.T) {
	cluster, err := NewMongoShardedCluster("test-cluster", minimalClusterProps())
	require.NoError(t, err)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	providers := m["provider"].(map[string]interface{})
	mongodbProviders := providers["mongodb"].([]interface{})
	assert.Len(t, mongodbProviders, 7)
}

// CDKTN-035/036: Direct mode correct per component type
func TestNewMongoShardedCluster_DirectModeByType(t *testing.T) {
	cluster, err := NewMongoShardedCluster("test-cluster", minimalClusterProps())
	require.NoError(t, err)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	providers := m["provider"].(map[string]interface{})
	for _, p := range providers["mongodb"].([]interface{}) {
		pMap := p.(map[string]interface{})
		alias := pMap["alias"].(string)
		if len(alias) >= 6 && alias[:6] == "mongos" {
			assert.Equal(t, false, pMap["direct"], "mongos MUST have direct=false")
		} else {
			assert.Equal(t, true, pMap["direct"], "shard/configsvr MUST have direct=true")
		}
	}
}

func TestNewMongoShardedCluster_MinimalGolden(t *testing.T) {
	cluster, err := NewMongoShardedCluster("test-cluster", minimalClusterProps())
	require.NoError(t, err)

	data, err := cluster.Stack.Synth()
	require.NoError(t, err)
	assert.True(t, json.Valid(data))
	goldenCompare(t, "cluster_minimal.json", data)
}

func TestNewMongoShardedCluster_FullGolden(t *testing.T) {
	cluster, err := NewMongoShardedCluster("test-cluster", fullClusterProps())
	require.NoError(t, err)

	data, err := cluster.Stack.Synth()
	require.NoError(t, err)
	assert.True(t, json.Valid(data))
	goldenCompare(t, "cluster_full.json", data)
}

// CDKTN-052: Cluster-level original users cascade to components
func TestNewMongoShardedCluster_ClusterOriginalUsers(t *testing.T) {
	props := minimalClusterProps()
	props.OriginalUsers = []OriginalUserConfig{
		{
			Host:     "bootstrap-host",
			Port:     27017,
			Username: "root_admin",
			Password: "bootstrap_pass",
			Roles:    []UserRoleRef{{Role: "root", DB: "admin"}},
		},
	}
	cluster, err := NewMongoShardedCluster("test-cluster", props)
	require.NoError(t, err)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	origUsers := resources[ResourceTypeOriginalUser].(map[string]interface{})

	// Should cascade to: mongos_0, configsvr_csrs_0, shard_shard01_0
	assert.Contains(t, origUsers, "mongos_0_origuser_root_admin")
	assert.Contains(t, origUsers, "configsvr_csrs_0_origuser_root_admin")
	assert.Contains(t, origUsers, "shard_shard01_0_origuser_root_admin")
	assert.Len(t, origUsers, 3, "one original user per component first alias")
}

// CDKTN-052: Component-level original users stay scoped
func TestNewMongoShardedCluster_ComponentOriginalUsers(t *testing.T) {
	props := minimalClusterProps()
	props.Shards[0].OriginalUsers = []OriginalUserConfig{
		{
			Host:     "shard-bootstrap",
			Port:     27018,
			Username: "shard_admin",
			Password: "shard_pass",
		},
	}
	cluster, err := NewMongoShardedCluster("test-cluster", props)
	require.NoError(t, err)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})
	origUsers := resources[ResourceTypeOriginalUser].(map[string]interface{})

	// Should only be on shard_shard01_0, not on mongos or config server
	assert.Contains(t, origUsers, "shard_shard01_0_origuser_shard_admin")
	assert.NotContains(t, origUsers, "mongos_0_origuser_shard_admin")
	assert.NotContains(t, origUsers, "configsvr_csrs_0_origuser_shard_admin")
	assert.Len(t, origUsers, 1)
}

// CDKTN-042: Provider version constraint
func TestNewMongoShardedCluster_ProviderVersion(t *testing.T) {
	props := minimalClusterProps()
	props.ProviderVersion = ">= 2.0.0"
	cluster, err := NewMongoShardedCluster("test-cluster", props)
	require.NoError(t, err)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	tf := m["terraform"].(map[string]interface{})
	rp := tf["required_providers"].(map[string]interface{})
	mongodb := rp["mongodb"].(map[string]interface{})
	assert.Equal(t, ">= 2.0.0", mongodb["version"])
}
