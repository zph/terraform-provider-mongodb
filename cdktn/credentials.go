package cdktn

// CredentialSource provides username and password for MongoDB provider aliases. // CDKTN-007
type CredentialSource interface {
	GetUsername() string
	GetPassword() string
	IsEnvBased() bool
}

// DirectCredentials provides literal username/password values. // CDKTN-007
type DirectCredentials struct {
	Username string
	Password string
}

func (d *DirectCredentials) GetUsername() string { return d.Username }
func (d *DirectCredentials) GetPassword() string { return d.Password }
func (d *DirectCredentials) IsEnvBased() bool    { return false }

// EnvCredentials relies on environment variable defaults in the provider schema.
// Returns empty strings so the provider falls through to its EnvDefaultFunc. // CDKTN-007
type EnvCredentials struct {
	UsernameEnvVar string
	PasswordEnvVar string
}

func (e *EnvCredentials) GetUsername() string { return "" }
func (e *EnvCredentials) GetPassword() string { return "" }
func (e *EnvCredentials) IsEnvBased() bool    { return true }
