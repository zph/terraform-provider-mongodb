package cdktn

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CDKTN-004: Provider alias naming pattern
func TestProviderAliasName(t *testing.T) {
	tests := []struct {
		ct       ComponentType
		rsName   string
		idx      int
		expected string
	}{
		{ComponentTypeShard, "shard01", 0, "shard_shard01_0"},
		{ComponentTypeShard, "shard01", 2, "shard_shard01_2"},
		{ComponentTypeConfigServer, "csrs", 1, "configsvr_csrs_1"},
		{ComponentTypeMongos, "", 0, "mongos_0"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := ProviderAliasName(tt.ct, tt.rsName, tt.idx)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestProviderRef(t *testing.T) {
	ref := ProviderRef("shard_shard01_0")
	assert.Equal(t, "mongodb.shard_shard01_0", ref)
}

// CDKTN-005: Provider block contains host, port, username, password, auth_database
func TestBuildProviderConfig_DirectCredentials(t *testing.T) {
	member := MemberConfig{Host: "mongo1.example.com", Port: 27018}
	creds := &DirectCredentials{Username: "admin", Password: "s3cret"}

	config := BuildProviderConfig(member, creds, nil, "", true)

	assert.Equal(t, "mongo1.example.com", config["host"])
	assert.Equal(t, "27018", config["port"])
	assert.Equal(t, "admin", config["username"])
	assert.Equal(t, "s3cret", config["password"])
	assert.Equal(t, DefaultAuthDatabase, config["auth_database"])
}

// CDKTN-035: direct=true for shard/config server
func TestBuildProviderConfig_DirectTrue(t *testing.T) {
	member := MemberConfig{Host: "h1", Port: 27018}
	creds := &DirectCredentials{Username: "u", Password: "p"}

	config := BuildProviderConfig(member, creds, nil, "", true)
	assert.Equal(t, true, config["direct"])
}

// CDKTN-036: direct=false for mongos
func TestBuildProviderConfig_DirectFalse(t *testing.T) {
	member := MemberConfig{Host: "h1", Port: 27017}
	creds := &DirectCredentials{Username: "u", Password: "p"}

	config := BuildProviderConfig(member, creds, nil, "", false)
	assert.Equal(t, false, config["direct"])
}

// CDKTN-039: retrywrites=true by default
func TestBuildProviderConfig_RetryWrites(t *testing.T) {
	member := MemberConfig{Host: "h1", Port: 27017}
	creds := &DirectCredentials{Username: "u", Password: "p"}

	config := BuildProviderConfig(member, creds, nil, "", true)
	assert.Equal(t, true, config["retrywrites"])
}

// CDKTN-040: auth_database defaults to "admin"
func TestBuildProviderConfig_AuthDatabaseDefault(t *testing.T) {
	member := MemberConfig{Host: "h1", Port: 27017}
	creds := &DirectCredentials{Username: "u", Password: "p"}

	config := BuildProviderConfig(member, creds, nil, "", true)
	assert.Equal(t, "admin", config["auth_database"])
}

// CDKTN-018: SSL enabled
func TestBuildProviderConfig_SSLEnabled(t *testing.T) {
	member := MemberConfig{Host: "h1", Port: 27017}
	creds := &DirectCredentials{Username: "u", Password: "p"}
	ssl := &SSLConfig{Enabled: true}

	config := BuildProviderConfig(member, creds, ssl, "", true)
	assert.Equal(t, true, config["ssl"])
}

// CDKTN-019: SSL certificate
func TestBuildProviderConfig_SSLCertificate(t *testing.T) {
	member := MemberConfig{Host: "h1", Port: 27017}
	creds := &DirectCredentials{Username: "u", Password: "p"}
	ssl := &SSLConfig{Enabled: true, Certificate: "CERT_PEM"}

	config := BuildProviderConfig(member, creds, ssl, "", true)
	assert.Equal(t, "CERT_PEM", config["certificate"])
}

// CDKTN-020: InsecureSkipVerify
func TestBuildProviderConfig_InsecureSkipVerify(t *testing.T) {
	member := MemberConfig{Host: "h1", Port: 27017}
	creds := &DirectCredentials{Username: "u", Password: "p"}
	ssl := &SSLConfig{Enabled: true, InsecureSkipVerify: true}

	config := BuildProviderConfig(member, creds, ssl, "", true)
	assert.Equal(t, true, config["insecure_skip_verify"])
}

// CDKTN-034: Proxy configuration
func TestBuildProviderConfig_Proxy(t *testing.T) {
	member := MemberConfig{Host: "h1", Port: 27017}
	creds := &DirectCredentials{Username: "u", Password: "p"}

	config := BuildProviderConfig(member, creds, nil, "socks5://proxy:1080", true)
	assert.Equal(t, "socks5://proxy:1080", config["proxy"])
}

func TestBuildProviderConfig_NoProxyOmitted(t *testing.T) {
	member := MemberConfig{Host: "h1", Port: 27017}
	creds := &DirectCredentials{Username: "u", Password: "p"}

	config := BuildProviderConfig(member, creds, nil, "", true)
	_, hasProxy := config["proxy"]
	assert.False(t, hasProxy)
}

// CDKTN-007: EnvCredentials produce empty username/password
func TestBuildProviderConfig_EnvCredentials(t *testing.T) {
	member := MemberConfig{Host: "h1", Port: 27017}
	creds := &EnvCredentials{UsernameEnvVar: "MONGO_USR", PasswordEnvVar: "MONGO_PWD"}

	config := BuildProviderConfig(member, creds, nil, "", true)
	assert.Equal(t, "", config["username"])
	assert.Equal(t, "", config["password"])
}

// CDKTN-010: Per-member credential override
func TestBuildProviderConfig_MemberCredentialOverride(t *testing.T) {
	memberCreds := &DirectCredentials{Username: "member_user", Password: "member_pass"}
	member := MemberConfig{Host: "h1", Port: 27017, Credentials: memberCreds}
	clusterCreds := &DirectCredentials{Username: "cluster_user", Password: "cluster_pass"}

	config := BuildProviderConfig(member, clusterCreds, nil, "", true)
	assert.Equal(t, "member_user", config["username"])
	assert.Equal(t, "member_pass", config["password"])
}

func TestBuildProviderConfig_NilMemberCredsUsesCluster(t *testing.T) {
	member := MemberConfig{Host: "h1", Port: 27017}
	clusterCreds := &DirectCredentials{Username: "cluster_user", Password: "cluster_pass"}

	config := BuildProviderConfig(member, clusterCreds, nil, "", true)
	assert.Equal(t, "cluster_user", config["username"])
	assert.Equal(t, "cluster_pass", config["password"])
}

// Test building full provider set for a replica set
func TestBuildProviders_ThreeMembers(t *testing.T) {
	members := []MemberConfig{
		{Host: "h1", Port: 27018},
		{Host: "h2", Port: 27019},
		{Host: "h3", Port: 27020},
	}
	creds := &DirectCredentials{Username: "admin", Password: "pass"}
	stack := NewTerraformStack("", "1.0.0")

	aliases := BuildProviders(stack, ComponentTypeShard, "shard01", members, creds, nil, "")
	require.Len(t, aliases, 3)
	assert.Equal(t, "shard_shard01_0", aliases[0])
	assert.Equal(t, "shard_shard01_1", aliases[1])
	assert.Equal(t, "shard_shard01_2", aliases[2])
}
