package cdktn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// CDKTN-007: CredentialSource interface with Direct and Env implementations
func TestDirectCredentials_ReturnsLiteralValues(t *testing.T) {
	creds := &DirectCredentials{Username: "admin", Password: "s3cret"}
	assert.Equal(t, "admin", creds.GetUsername())
	assert.Equal(t, "s3cret", creds.GetPassword())
	assert.False(t, creds.IsEnvBased())
}

func TestEnvCredentials_ReturnsEmptyStrings(t *testing.T) {
	creds := &EnvCredentials{
		UsernameEnvVar: "MONGO_USR",
		PasswordEnvVar: "MONGO_PWD",
	}
	// EnvCredentials return empty strings so Terraform falls through to env var defaults
	assert.Empty(t, creds.GetUsername())
	assert.Empty(t, creds.GetPassword())
	assert.True(t, creds.IsEnvBased())
}

func TestEnvCredentials_EnvVarNames(t *testing.T) {
	creds := &EnvCredentials{
		UsernameEnvVar: "MY_USER",
		PasswordEnvVar: "MY_PASS",
	}
	assert.Equal(t, "MY_USER", creds.UsernameEnvVar)
	assert.Equal(t, "MY_PASS", creds.PasswordEnvVar)
}

func TestCredentialSource_InterfaceSatisfaction(t *testing.T) {
	var _ CredentialSource = &DirectCredentials{}
	var _ CredentialSource = &EnvCredentials{}
}
