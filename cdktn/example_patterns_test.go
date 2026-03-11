package cdktn

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Pattern: full-cluster-setup (L3 MongoShardedCluster)
// ---------------------------------------------------------------------------

func TestExamplePattern_FullClusterSetup(t *testing.T) {
	cluster, err := NewMongoShardedCluster("prod", &MongoShardedClusterProps{
		Credentials: &DirectCredentials{Username: "admin", Password: "test-password"},

		ConfigServers: ConfigServerConfig{
			ReplicaSetName: "configRS",
			Members: []MemberConfig{
				{Host: "configsvr0.example.com", Port: 27019},
				{Host: "configsvr1.example.com", Port: 27019},
				{Host: "configsvr2.example.com", Port: 27019},
			},
			OriginalUsers: []OriginalUserConfig{
				{Host: "configsvr0.example.com", Port: 27019, Username: "admin", Password: "test-password", Roles: []UserRoleRef{{Role: "root", DB: "admin"}}},
			},
		},

		Shards: []ShardConfig{
			{
				ReplicaSetName: "shard01",
				Members: []MemberConfig{
					{Host: "shard01a.example.com", Port: 27018},
					{Host: "shard01b.example.com", Port: 27018},
					{Host: "shard01c.example.com", Port: 27018},
				},
				ShardConfig: &ShardConfigSettings{
					ChainingAllowed: true, HeartbeatIntervalMillis: 1000, HeartbeatTimeoutSecs: 10, ElectionTimeoutMillis: 10000,
					Members: []MemberOverrideConfig{
						{Host: "shard01a.example.com:27018", Priority: 10, Votes: 1, Tags: map[string]string{"role": "primary-preferred", "zone": "us-east-1a"}},
						{Host: "shard01b.example.com:27018", Priority: 5, Votes: 1, Tags: map[string]string{"role": "secondary", "zone": "us-east-1b"}},
						{Host: "shard01c.example.com:27018", Priority: 5, Votes: 1, Tags: map[string]string{"role": "secondary", "zone": "us-east-1c"}},
					},
				},
				OriginalUsers: []OriginalUserConfig{
					{Host: "shard01a.example.com", Port: 27018, Username: "admin", Password: "test-password", Roles: []UserRoleRef{{Role: "root", DB: "admin"}}},
				},
			},
			{
				ReplicaSetName: "shard02",
				Members: []MemberConfig{
					{Host: "shard02a.example.com", Port: 27018},
					{Host: "shard02b.example.com", Port: 27018},
					{Host: "shard02c.example.com", Port: 27018},
				},
				ShardConfig: &ShardConfigSettings{
					ChainingAllowed: true, HeartbeatIntervalMillis: 1000, HeartbeatTimeoutSecs: 10, ElectionTimeoutMillis: 10000,
					Members: []MemberOverrideConfig{
						{Host: "shard02a.example.com:27018", Priority: 10, Votes: 1, Tags: map[string]string{"role": "primary-preferred", "zone": "us-west-2a"}},
						{Host: "shard02b.example.com:27018", Priority: 5, Votes: 1, Tags: map[string]string{"role": "secondary", "zone": "us-west-2b"}},
						{Host: "shard02c.example.com:27018", Priority: 5, Votes: 1, Tags: map[string]string{"role": "secondary", "zone": "us-west-2c"}},
					},
				},
				OriginalUsers: []OriginalUserConfig{
					{Host: "shard02a.example.com", Port: 27018, Username: "admin", Password: "test-password", Roles: []UserRoleRef{{Role: "root", DB: "admin"}}},
				},
			},
		},

		Mongos: []MongosConfig{
			{
				Members: []MemberConfig{{Host: "mongos0.example.com", Port: 27017}},
				OriginalUsers: []OriginalUserConfig{
					{Host: "mongos0.example.com", Port: 27017, Username: "admin", Password: "test-password", Roles: []UserRoleRef{{Role: "root", DB: "admin"}}},
				},
			},
		},

		RegisterShards: true,
		Balancer:       &BalancerConfig{Enabled: true, ActiveWindowStart: "02:00", ActiveWindowStop: "06:00"},
		ShardZones: []ShardZoneConfig{
			{ShardName: "shard01", Zone: "US-East"},
			{ShardName: "shard02", Zone: "US-West"},
		},
		ZoneKeyRanges: []ZoneKeyRangeConfig{
			{Namespace: "app_db.orders", Zone: "US-East", Min: `{"_id":{"$minKey":1}}`, Max: `{"_id":0}`},
			{Namespace: "app_db.orders", Zone: "US-West", Min: `{"_id":0}`, Max: `{"_id":{"$maxKey":1}}`},
		},
		Roles: []RoleConfig{
			{
				Name: "app_readwrite", Database: "admin",
				Privileges: []Privilege{
					{DB: "app_db", Collection: "", Actions: []string{"find", "insert", "update", "remove"}},
					{DB: "app_db", Collection: "", Actions: []string{"createIndex", "dropIndex"}},
				},
			},
		},
		Users: []UserConfig{
			{Username: "app_service", Password: "app-pass", Database: "admin", Roles: []UserRoleRef{{Role: "app_readwrite", DB: "admin"}}},
		},
	})
	require.NoError(t, err)

	data, err := cluster.Stack.Synth()
	require.NoError(t, err)
	assert.True(t, json.Valid(data))
	goldenCompare(t, data)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	// 10 provider aliases: 3 configsvr + 3 shard01 + 3 shard02 + 1 mongos
	providers := m["provider"].(map[string]interface{})
	assert.Len(t, providers["mongodb"].([]interface{}), 10)

	resources := m["resource"].(map[string]interface{})

	// 4 original users: configsvr primary + shard01 primary + shard02 primary + mongos
	origUsers := resources[ResourceTypeOriginalUser].(map[string]interface{})
	assert.Len(t, origUsers, 4)

	// 2 shard registrations
	shards := resources[ResourceTypeShard].(map[string]interface{})
	assert.Len(t, shards, 2)
	assert.Contains(t, shards, "shard_shard01")
	assert.Contains(t, shards, "shard_shard02")

	// Balancer with active window
	balancer := resources[ResourceTypeBalancerConfig].(map[string]interface{})
	bal := balancer["balancer"].(map[string]interface{})
	assert.Equal(t, true, bal["enabled"])
	assert.Equal(t, "02:00", bal["active_window_start"])
	assert.Equal(t, "06:00", bal["active_window_stop"])

	// 2 shard zones
	zones := resources[ResourceTypeShardZone].(map[string]interface{})
	assert.Len(t, zones, 2)

	// 2 zone key ranges
	ranges := resources[ResourceTypeZoneKeyRange].(map[string]interface{})
	assert.Len(t, ranges, 2)

	// Cluster-level user on mongos + shard primaries (1 mongos + 2 shard primaries = 3)
	users := resources[ResourceTypeDBUser].(map[string]interface{})
	assert.Contains(t, users, "mongos_0_user_app_service")
	assert.Contains(t, users, "shard_shard01_0_user_app_service")
	assert.Contains(t, users, "shard_shard02_0_user_app_service")

	// Per-member overrides in shard01 config
	configs := resources[ResourceTypeShardConfig].(map[string]interface{})
	s01Cfg := configs["shard01_config"].(map[string]interface{})
	members := s01Cfg["member"].([]interface{})
	assert.Len(t, members, 3)
	m0 := members[0].(map[string]interface{})
	assert.Equal(t, float64(10), m0["priority"])
	tags := m0["tags"].(map[string]interface{})
	assert.Equal(t, "us-east-1a", tags["zone"])
}

// ---------------------------------------------------------------------------
// Pattern: sharded-cluster (L3 MongoShardedCluster with SSL)
// ---------------------------------------------------------------------------

func TestExamplePattern_ShardedCluster(t *testing.T) {
	cluster, err := NewMongoShardedCluster("prod", &MongoShardedClusterProps{
		Credentials: &DirectCredentials{Username: "root", Password: "test-password"},
		SSL:         &SSLConfig{Enabled: true, Certificate: "TEST_CERT_PEM"},

		ConfigServers: ConfigServerConfig{
			ReplicaSetName: "configRS",
			Members: []MemberConfig{
				{Host: "configsvr0.example.com", Port: 27019},
				{Host: "configsvr1.example.com", Port: 27019},
				{Host: "configsvr2.example.com", Port: 27019},
			},
			ShardConfig: &ShardConfigSettings{
				ChainingAllowed: false, HeartbeatIntervalMillis: 1000, HeartbeatTimeoutSecs: 10, ElectionTimeoutMillis: 5000,
			},
		},

		Shards: []ShardConfig{
			{
				ReplicaSetName: "shard01",
				Members: []MemberConfig{
					{Host: "shard01a.example.com", Port: 27018},
					{Host: "shard01b.example.com", Port: 27018},
					{Host: "shard01c.example.com", Port: 27018},
				},
				ShardConfig: &ShardConfigSettings{
					ChainingAllowed: false, HeartbeatIntervalMillis: 1000, HeartbeatTimeoutSecs: 10, ElectionTimeoutMillis: 5000,
				},
			},
			{
				ReplicaSetName: "shard02",
				Members: []MemberConfig{
					{Host: "shard02a.example.com", Port: 27018},
					{Host: "shard02b.example.com", Port: 27018},
					{Host: "shard02c.example.com", Port: 27018},
				},
				ShardConfig: &ShardConfigSettings{
					ChainingAllowed: false, HeartbeatIntervalMillis: 1000, HeartbeatTimeoutSecs: 10, ElectionTimeoutMillis: 5000,
				},
			},
		},

		Mongos: []MongosConfig{
			{Members: []MemberConfig{{Host: "mongos0.example.com", Port: 27017}}},
		},

		Roles: []RoleConfig{
			{Name: "app_readwrite", Database: "admin", Privileges: []Privilege{
				{DB: "app_db", Collection: "", Actions: []string{"find", "insert", "update", "remove"}},
			}},
			{Name: "ops_monitoring", Database: "admin", Privileges: []Privilege{
				{Cluster: true, Actions: []string{"replSetGetStatus", "serverStatus"}},
				{DB: "admin", Collection: "", Actions: []string{"collStats", "dbStats"}},
			}},
		},

		Users: []UserConfig{
			{Username: "app_service", Password: "app-pass", Database: "admin", Roles: []UserRoleRef{{Role: "app_readwrite", DB: "admin"}}},
			{Username: "ops_monitor", Password: "ops-pass", Database: "admin", Roles: []UserRoleRef{
				{Role: "ops_monitoring", DB: "admin"},
				{Role: "clusterMonitor", DB: "admin"},
			}},
		},
	})
	require.NoError(t, err)

	data, err := cluster.Stack.Synth()
	require.NoError(t, err)
	assert.True(t, json.Valid(data))
	goldenCompare(t, data)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	// All providers have SSL enabled and certificate
	providers := m["provider"].(map[string]interface{})
	for _, p := range providers["mongodb"].([]interface{}) {
		pMap := p.(map[string]interface{})
		assert.Equal(t, true, pMap["ssl"], "all providers must have ssl=true")
		assert.Equal(t, "TEST_CERT_PEM", pMap["certificate"], "all providers must have certificate")
	}

	resources := m["resource"].(map[string]interface{})

	// chaining_allowed=false on all shard configs
	configs := resources[ResourceTypeShardConfig].(map[string]interface{})
	for name, cfg := range configs {
		cfgMap := cfg.(map[string]interface{})
		assert.Equal(t, false, cfgMap["chaining_allowed"], "chaining_allowed should be false on %s", name)
	}

	// ops_monitor user has 2 role refs
	users := resources[ResourceTypeDBUser].(map[string]interface{})
	opsUser := users["mongos_0_user_ops_monitor"].(map[string]interface{})
	opsRoles := opsUser["role"].([]interface{})
	assert.Len(t, opsRoles, 2)
}

// ---------------------------------------------------------------------------
// Pattern: monitoring-user (L2 MongoMongos)
// ---------------------------------------------------------------------------

func TestExamplePattern_MonitoringUser(t *testing.T) {
	stack := NewTerraformStack(DefaultTerraformVersion, "9.9.9")

	_, err := NewMongoMongos(stack, "mongos", &MongosProps{
		Members:     []MemberConfig{{Host: "127.0.0.1", Port: 27017}},
		Credentials: &DirectCredentials{Username: "root", Password: "test-password"},
		Roles: []RoleConfig{
			{
				Name: "metrics_exporter", Database: "admin",
				Privileges: []Privilege{
					{Cluster: true, Actions: []string{"serverStatus", "replSetGetStatus"}},
					{DB: "", Collection: "", Actions: []string{"dbStats", "collStats", "indexStats"}},
					{DB: "local", Collection: "oplog.rs", Actions: []string{"find"}},
				},
			},
		},
		Users: []UserConfig{
			{
				Username: "mongodb_exporter", Password: "exporter-pass", Database: "admin",
				Roles: []UserRoleRef{
					{Role: "metrics_exporter", DB: "admin"},
					{Role: "clusterMonitor", DB: "admin"},
				},
			},
		},
	})
	require.NoError(t, err)

	data, err := stack.Synth()
	require.NoError(t, err)
	assert.True(t, json.Valid(data))
	goldenCompare(t, data)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	// 1 provider alias, direct=false
	providers := m["provider"].(map[string]interface{})
	mongodbProviders := providers["mongodb"].([]interface{})
	assert.Len(t, mongodbProviders, 1)
	assert.Equal(t, false, mongodbProviders[0].(map[string]interface{})["direct"])

	resources := m["resource"].(map[string]interface{})

	// 1 role with 3 privilege blocks
	roles := resources[ResourceTypeDBRole].(map[string]interface{})
	assert.Len(t, roles, 1)
	role := roles["mongos_0_role_metrics_exporter"].(map[string]interface{})
	privs := role["privilege"].([]interface{})
	assert.Len(t, privs, 3)

	// 1 user with 2 role refs
	users := resources[ResourceTypeDBUser].(map[string]interface{})
	assert.Len(t, users, 1)
	user := users["mongos_0_user_mongodb_exporter"].(map[string]interface{})
	userRoles := user["role"].([]interface{})
	assert.Len(t, userRoles, 2)

	// No shard_config resource
	_, hasShardConfig := resources[ResourceTypeShardConfig]
	assert.False(t, hasShardConfig, "mongos must not have shard_config")
}

// ---------------------------------------------------------------------------
// Pattern: role-hierarchy (L2 MongoMongos)
// ---------------------------------------------------------------------------

func TestExamplePattern_RoleHierarchy(t *testing.T) {
	stack := NewTerraformStack(DefaultTerraformVersion, "9.9.9")

	_, err := NewMongoMongos(stack, "mongos", &MongosProps{
		Members:     []MemberConfig{{Host: "127.0.0.1", Port: 27017}},
		Credentials: &DirectCredentials{Username: "root", Password: "test-password"},
		Roles: []RoleConfig{
			{
				Name: "app_viewer", Database: "admin",
				Privileges: []Privilege{
					{DB: "app_db", Collection: "", Actions: []string{"find", "listCollections"}},
					{DB: "app_db", Collection: "", Actions: []string{"collStats", "dbStats"}},
				},
			},
			{
				Name: "app_editor", Database: "admin",
				Privileges: []Privilege{
					{DB: "app_db", Collection: "", Actions: []string{"insert", "update", "remove"}},
				},
				InheritedRoles: []InheritedRole{{Role: "app_viewer", DB: "admin"}},
			},
			{
				Name: "app_admin", Database: "admin",
				Privileges: []Privilege{
					{DB: "app_db", Collection: "", Actions: []string{"createIndex", "dropIndex", "createCollection", "dropCollection"}},
				},
				InheritedRoles: []InheritedRole{{Role: "app_editor", DB: "admin"}},
			},
		},
		Users: []UserConfig{
			{Username: "viewer_user", Password: "viewer-pass", Database: "admin", Roles: []UserRoleRef{{Role: "app_viewer", DB: "admin"}}},
			{Username: "editor_user", Password: "editor-pass", Database: "admin", Roles: []UserRoleRef{{Role: "app_editor", DB: "admin"}}},
			{Username: "admin_user", Password: "admin-pass", Database: "admin", Roles: []UserRoleRef{{Role: "app_admin", DB: "admin"}}},
		},
	})
	require.NoError(t, err)

	data, err := stack.Synth()
	require.NoError(t, err)
	assert.True(t, json.Valid(data))
	goldenCompare(t, data)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})

	// 3 roles on single mongos alias
	roles := resources[ResourceTypeDBRole].(map[string]interface{})
	assert.Len(t, roles, 3)

	// editor inherits viewer
	editor := roles["mongos_0_role_app_editor"].(map[string]interface{})
	editorInherited := editor["inherited_role"].([]interface{})
	assert.Len(t, editorInherited, 1)
	assert.Equal(t, "app_viewer", editorInherited[0].(map[string]interface{})["role"])

	// admin inherits editor
	admin := roles["mongos_0_role_app_admin"].(map[string]interface{})
	adminInherited := admin["inherited_role"].([]interface{})
	assert.Len(t, adminInherited, 1)
	assert.Equal(t, "app_editor", adminInherited[0].(map[string]interface{})["role"])

	// 3 users, each with depends_on all 3 roles
	users := resources[ResourceTypeDBUser].(map[string]interface{})
	assert.Len(t, users, 3)
	for _, u := range users {
		uMap := u.(map[string]interface{})
		deps := uMap["depends_on"].([]interface{})
		assert.Len(t, deps, 3, "each user should depend on all 3 roles")
	}
}

// ---------------------------------------------------------------------------
// Pattern: add-replicaset-to-cluster (L2 MongoShard)
// ---------------------------------------------------------------------------

func TestExamplePattern_AddReplicaset(t *testing.T) {
	stack := NewTerraformStack(DefaultTerraformVersion, "9.9.9")

	shard, err := NewMongoShard(stack, "shard03", &MongoShardProps{
		ReplicaSetName: "shard03",
		Members: []MemberConfig{
			{Host: "shard03a.example.com", Port: 27018},
			{Host: "shard03b.example.com", Port: 27018},
			{Host: "shard03c.example.com", Port: 27018},
		},
		Credentials: &DirectCredentials{Username: "admin", Password: "test-password"},
		OriginalUsers: []OriginalUserConfig{
			{Host: "shard03a.example.com", Port: 27018, Username: "admin", Password: "test-password", Roles: []UserRoleRef{{Role: "root", DB: "admin"}}},
		},
		ShardConfig: &ShardConfigSettings{
			ChainingAllowed: true, HeartbeatIntervalMillis: 1000, HeartbeatTimeoutSecs: 10, ElectionTimeoutMillis: 10000,
			Members: []MemberOverrideConfig{
				{Host: "shard03a.example.com:27018", Priority: 10, Votes: 1, Tags: map[string]string{"role": "primary-preferred"}},
				{Host: "shard03b.example.com:27018", Priority: 5, Votes: 1, Tags: map[string]string{"role": "secondary"}},
				{Host: "shard03c.example.com:27018", Priority: 5, Votes: 1, Tags: map[string]string{"role": "secondary"}},
			},
		},
	})
	require.NoError(t, err)

	data, err := stack.Synth()
	require.NoError(t, err)
	assert.True(t, json.Valid(data))
	goldenCompare(t, data)

	m, err := stack.SynthToMap()
	require.NoError(t, err)

	// 3 provider aliases, all direct=true
	providers := m["provider"].(map[string]interface{})
	mongodbProviders := providers["mongodb"].([]interface{})
	assert.Len(t, mongodbProviders, 3)
	assert.Equal(t, []string{"shard_shard03_0", "shard_shard03_1", "shard_shard03_2"}, shard.Aliases)
	for _, p := range mongodbProviders {
		assert.Equal(t, true, p.(map[string]interface{})["direct"])
	}

	resources := m["resource"].(map[string]interface{})

	// 1 original user on primary
	origUsers := resources[ResourceTypeOriginalUser].(map[string]interface{})
	assert.Len(t, origUsers, 1)
	assert.Contains(t, origUsers, "shard_shard03_0_origuser_admin")

	// 1 shard_config with 3 member overrides
	configs := resources[ResourceTypeShardConfig].(map[string]interface{})
	assert.Len(t, configs, 1)
	cfg := configs["shard03_config"].(map[string]interface{})
	members := cfg["member"].([]interface{})
	assert.Len(t, members, 3)
	m0 := members[0].(map[string]interface{})
	assert.Equal(t, float64(10), m0["priority"])
	assert.Equal(t, "primary-preferred", m0["tags"].(map[string]interface{})["role"])

	// No mongos or config server resources
	_, hasBalancer := resources[ResourceTypeBalancerConfig]
	assert.False(t, hasBalancer, "standalone shard should not have balancer")
}

// ---------------------------------------------------------------------------
// Pattern: zone-sharding (L3 MongoShardedCluster)
// ---------------------------------------------------------------------------

func TestExamplePattern_ZoneSharding(t *testing.T) {
	cluster, err := NewMongoShardedCluster("zones", &MongoShardedClusterProps{
		Credentials: &DirectCredentials{Username: "admin", Password: "test-password"},

		ConfigServers: ConfigServerConfig{
			ReplicaSetName: "configRS",
			Members: []MemberConfig{
				{Host: "cfg0.example.com", Port: 27019},
				{Host: "cfg1.example.com", Port: 27019},
				{Host: "cfg2.example.com", Port: 27019},
			},
		},

		Shards: []ShardConfig{
			{ReplicaSetName: "shard01", Members: []MemberConfig{
				{Host: "shard01a.example.com", Port: 27018},
				{Host: "shard01b.example.com", Port: 27018},
				{Host: "shard01c.example.com", Port: 27018},
			}},
			{ReplicaSetName: "shard02", Members: []MemberConfig{
				{Host: "shard02a.example.com", Port: 27018},
				{Host: "shard02b.example.com", Port: 27018},
				{Host: "shard02c.example.com", Port: 27018},
			}},
			{ReplicaSetName: "shard03", Members: []MemberConfig{
				{Host: "shard03a.example.com", Port: 27018},
				{Host: "shard03b.example.com", Port: 27018},
				{Host: "shard03c.example.com", Port: 27018},
			}},
		},

		Mongos: []MongosConfig{
			{Members: []MemberConfig{{Host: "mongos0.example.com", Port: 27017}}},
		},

		RegisterShards: true,

		ShardZones: []ShardZoneConfig{
			{ShardName: "shard01", Zone: "US-East"},
			{ShardName: "shard02", Zone: "US-West"},
			{ShardName: "shard03", Zone: "US-East"},
			{ShardName: "shard03", Zone: "Backup"},
		},

		ZoneKeyRanges: []ZoneKeyRangeConfig{
			{Namespace: "app_db.orders", Zone: "US-East", Min: `{"_id":{"$minKey":1}}`, Max: `{"_id":0}`},
			{Namespace: "app_db.orders", Zone: "US-West", Min: `{"_id":0}`, Max: `{"_id":{"$maxKey":1}}`},
			{Namespace: "analytics.logs", Zone: "Backup", Min: `{"tenant":{"$minKey":1},"timestamp":{"$minKey":1}}`, Max: `{"tenant":{"$maxKey":1},"timestamp":{"$maxKey":1}}`},
		},
	})
	require.NoError(t, err)

	data, err := cluster.Stack.Synth()
	require.NoError(t, err)
	assert.True(t, json.Valid(data))
	goldenCompare(t, data)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})

	// 3 shard registrations
	shards := resources[ResourceTypeShard].(map[string]interface{})
	assert.Len(t, shards, 3)

	// 4 shard zone mappings (shard03 in 2 zones)
	zones := resources[ResourceTypeShardZone].(map[string]interface{})
	assert.Len(t, zones, 4)

	// 3 zone key ranges (2 for orders + 1 for logs)
	ranges := resources[ResourceTypeZoneKeyRange].(map[string]interface{})
	assert.Len(t, ranges, 3)

	// Zone key ranges have depends_on zone resources
	for _, r := range ranges {
		rMap := r.(map[string]interface{})
		deps, hasDeps := rMap["depends_on"]
		assert.True(t, hasDeps, "zone key ranges must have depends_on")
		assert.NotEmpty(t, deps.([]interface{}), "zone key range depends_on must not be empty")
	}
}

// ---------------------------------------------------------------------------
// Pattern: collection-balancing (L3 MongoShardedCluster)
// ---------------------------------------------------------------------------

func TestExamplePattern_CollectionBalancing(t *testing.T) {
	cluster, err := NewMongoShardedCluster("colbal", &MongoShardedClusterProps{
		Credentials: &DirectCredentials{Username: "admin", Password: "test-password"},

		ConfigServers: ConfigServerConfig{
			ReplicaSetName: "configRS",
			Members: []MemberConfig{
				{Host: "cfg0.example.com", Port: 27019},
				{Host: "cfg1.example.com", Port: 27019},
				{Host: "cfg2.example.com", Port: 27019},
			},
		},

		Shards: []ShardConfig{
			{ReplicaSetName: "shard01", Members: []MemberConfig{
				{Host: "shard01a.example.com", Port: 27018},
				{Host: "shard01b.example.com", Port: 27018},
				{Host: "shard01c.example.com", Port: 27018},
			}},
			{ReplicaSetName: "shard02", Members: []MemberConfig{
				{Host: "shard02a.example.com", Port: 27018},
				{Host: "shard02b.example.com", Port: 27018},
				{Host: "shard02c.example.com", Port: 27018},
			}},
		},

		Mongos: []MongosConfig{
			{Members: []MemberConfig{{Host: "mongos0.example.com", Port: 27017}}},
		},

		RegisterShards: true,

		Balancer: &BalancerConfig{
			Enabled:           true,
			ActiveWindowStart: "02:00",
			ActiveWindowStop:  "06:00",
			ChunkSizeMB:       128,
		},

		CollectionBalancing: []CollectionBalancingConfig{
			{Namespace: "app_db.staging_imports", Enabled: false},
			{Namespace: "app_db.events", Enabled: true, ChunkSizeMB: 256},
			{Namespace: "app_db.config_data", Enabled: true, ChunkSizeMB: 32},
		},
	})
	require.NoError(t, err)

	data, err := cluster.Stack.Synth()
	require.NoError(t, err)
	assert.True(t, json.Valid(data))
	goldenCompare(t, data)

	m, err := cluster.Stack.SynthToMap()
	require.NoError(t, err)

	resources := m["resource"].(map[string]interface{})

	// Global balancer with window and chunk size
	balancer := resources[ResourceTypeBalancerConfig].(map[string]interface{})
	bal := balancer["balancer"].(map[string]interface{})
	assert.Equal(t, true, bal["enabled"])
	assert.Equal(t, "02:00", bal["active_window_start"])
	assert.Equal(t, "06:00", bal["active_window_stop"])
	assert.Equal(t, float64(128), bal["chunk_size_mb"])

	// 3 per-collection balancing configs
	colBals := resources[ResourceTypeCollBalancing].(map[string]interface{})
	assert.Len(t, colBals, 3)

	// staging_imports: disabled
	staging := colBals["colbal_app_db_staging_imports"].(map[string]interface{})
	assert.Equal(t, false, staging["enabled"])

	// events: chunk_size_mb=256
	events := colBals["colbal_app_db_events"].(map[string]interface{})
	assert.Equal(t, true, events["enabled"])
	assert.Equal(t, float64(256), events["chunk_size_mb"])

	// config_data: chunk_size_mb=32
	configData := colBals["colbal_app_db_config_data"].(map[string]interface{})
	assert.Equal(t, true, configData["enabled"])
	assert.Equal(t, float64(32), configData["chunk_size_mb"])

	// Collection balancing depends on shard registrations
	for _, cb := range colBals {
		cbMap := cb.(map[string]interface{})
		deps, hasDeps := cbMap["depends_on"]
		assert.True(t, hasDeps, "collection balancing must have depends_on shard registrations")
		assert.NotEmpty(t, deps.([]interface{}))
	}
}
