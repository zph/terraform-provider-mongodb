package cdktn

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CDKTN-003: Members slice with Host and Port
func TestMemberConfig_HostPort(t *testing.T) {
	m := MemberConfig{Host: "mongo1.example.com", Port: 27018}
	assert.Equal(t, "mongo1.example.com", m.Host)
	assert.Equal(t, 27018, m.Port)
}

func TestMemberConfig_DefaultPort(t *testing.T) {
	m := MemberConfig{Host: "mongo1.example.com"}
	assert.Equal(t, 0, m.Port) // zero value; resolved at factory level
}

func TestMemberConfig_HostPortString(t *testing.T) {
	m := MemberConfig{Host: "mongo1.example.com", Port: 27018}
	assert.Equal(t, "mongo1.example.com:27018", m.HostPort())
}

// CDKTN-032: MongoShardedClusterProps fields
func TestMongoShardedClusterProps_AllFields(t *testing.T) {
	props := MongoShardedClusterProps{
		Mongos: []MongosConfig{
			{Members: []MemberConfig{{Host: "mongos1", Port: 27017}}},
		},
		ConfigServers: ConfigServerConfig{
			ReplicaSetName: "csrs",
			Members:        []MemberConfig{{Host: "cfg1", Port: 27019}},
		},
		Shards: []ShardConfig{
			{ReplicaSetName: "shard01", Members: []MemberConfig{{Host: "s1", Port: 27018}}},
		},
		Credentials:     &DirectCredentials{Username: "admin", Password: "pass"},
		SSL:             &SSLConfig{Enabled: true},
		Users:           []UserConfig{{Username: "app", Password: "secret", Database: "admin"}},
		ProviderVersion: ">= 1.0.0",
	}

	assert.Len(t, props.Mongos, 1)
	assert.Equal(t, "csrs", props.ConfigServers.ReplicaSetName)
	assert.Len(t, props.Shards, 1)
	assert.NotNil(t, props.Credentials)
	assert.NotNil(t, props.SSL)
	assert.Len(t, props.Users, 1)
	assert.Equal(t, ">= 1.0.0", props.ProviderVersion)
}

// CDKTN-033: ShardConfig has ReplicaSetName and Members
func TestShardConfig_RequiredFields(t *testing.T) {
	sc := ShardConfig{
		ReplicaSetName: "shard01",
		Members: []MemberConfig{
			{Host: "s1m1", Port: 27018},
			{Host: "s1m2", Port: 27019},
			{Host: "s1m3", Port: 27020},
		},
	}
	assert.Equal(t, "shard01", sc.ReplicaSetName)
	assert.Len(t, sc.Members, 3)
}

func TestUserConfig_RoleAssignment(t *testing.T) {
	u := UserConfig{
		Username: "appuser",
		Password: "secret",
		Database: "admin",
		Roles: []UserRoleRef{
			{Role: "readWrite", DB: "mydb"},
			{Role: "clusterMonitor", DB: "admin"},
		},
	}
	assert.Equal(t, "appuser", u.Username)
	assert.Len(t, u.Roles, 2)
	assert.Equal(t, "readWrite", u.Roles[0].Role)
}

func TestRoleConfig_PrivilegesAndInheritedRoles(t *testing.T) {
	r := RoleConfig{
		Name:     "StaffRole",
		Database: "admin",
		Privileges: []Privilege{
			{DB: "*", Collection: "*", Actions: []string{"collStats"}},
		},
		InheritedRoles: []InheritedRole{
			{Role: "clusterMonitor", DB: "admin"},
		},
	}
	assert.Equal(t, "StaffRole", r.Name)
	assert.Len(t, r.Privileges, 1)
	assert.Len(t, r.InheritedRoles, 1)
}

func TestPrivilege_ClusterLevel(t *testing.T) {
	p := Privilege{
		Cluster: true,
		Actions: []string{"replSetGetConfig", "replSetGetStatus"},
	}
	assert.True(t, p.Cluster)
	assert.Len(t, p.Actions, 2)
}

func TestSSLConfig_Fields(t *testing.T) {
	ssl := SSLConfig{
		Enabled:            true,
		Certificate:        "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
		InsecureSkipVerify: false,
	}
	assert.True(t, ssl.Enabled)
	assert.Contains(t, ssl.Certificate, "BEGIN CERTIFICATE")
	assert.False(t, ssl.InsecureSkipVerify)
}

func TestShardConfigSettings_Defaults(t *testing.T) {
	s := DefaultShardConfigSettings()
	require.NotNil(t, s)
	assert.True(t, s.ChainingAllowed)
	assert.Equal(t, 1000, s.HeartbeatIntervalMillis)
	assert.Equal(t, 10, s.HeartbeatTimeoutSecs)
	assert.Equal(t, 10000, s.ElectionTimeoutMillis)
}

func TestMongoShardProps_WithShardConfig(t *testing.T) {
	props := MongoShardProps{
		ReplicaSetName: "shard01",
		Members: []MemberConfig{
			{Host: "h1", Port: 27018},
			{Host: "h2", Port: 27019},
			{Host: "h3", Port: 27020},
		},
		ShardConfig: &ShardConfigSettings{
			ChainingAllowed:         true,
			HeartbeatIntervalMillis: 2000,
			HeartbeatTimeoutSecs:    10,
			ElectionTimeoutMillis:   10000,
		},
	}
	assert.Equal(t, "shard01", props.ReplicaSetName)
	assert.NotNil(t, props.ShardConfig)
	assert.Equal(t, 2000, props.ShardConfig.HeartbeatIntervalMillis)
}
