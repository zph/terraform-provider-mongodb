package cdktn

import "fmt"

// MemberConfig identifies a single MongoDB node in a replica set or mongos pool. // CDKTN-003
type MemberConfig struct {
	Host        string
	Port        int
	Credentials CredentialSource // CDKTN-010: per-member credential override
}

// HostPort returns the "host:port" string for this member.
func (m MemberConfig) HostPort() string {
	return fmt.Sprintf("%s:%d", m.Host, m.Port)
}

// UserRoleRef is a role reference attached to a user.
type UserRoleRef struct {
	Role string
	DB   string
}

// UserConfig defines a MongoDB user to be created. // CDKTN-014
type UserConfig struct {
	Username string
	Password string
	Database string // auth database for this user
	Roles    []UserRoleRef
}

// Privilege defines a custom privilege for a role.
type Privilege struct {
	DB         string
	Collection string
	Cluster    bool
	Actions    []string
}

// InheritedRole is a role inherited by a custom role.
type InheritedRole struct {
	Role string
	DB   string
}

// RoleConfig defines a custom MongoDB role. // CDKTN-012
type RoleConfig struct {
	Name           string
	Database       string
	Privileges     []Privilege
	InheritedRoles []InheritedRole
}

// SSLConfig holds TLS settings for provider aliases. // CDKTN-018, CDKTN-019, CDKTN-020
type SSLConfig struct {
	Enabled            bool
	Certificate        string
	InsecureSkipVerify bool
}

// MemberOverrideConfig specifies per-member replica set configuration overrides.
// Maps to the provider's `member` block in mongodb_shard_config. // CDKTN-051
type MemberOverrideConfig struct {
	Host               string
	Priority           int
	Votes              int
	Hidden             bool
	ArbiterOnly        bool
	BuildIndexes       bool
	SecondaryDelaySecs int
	Tags               map[string]string
}

// OriginalUserConfig defines a bootstrap admin user on a no-auth MongoDB instance.
// Each produces a mongodb_original_user resource with inline connection params. // CDKTN-052
type OriginalUserConfig struct {
	Host         string
	Port         int
	Username     string
	Password     string
	AuthDatabase string // default "admin"
	Roles        []UserRoleRef
	ReplicaSet   string // optional, auto-discovered if empty
	SSL          *SSLConfig
}

// ShardConfigSettings holds replica set configuration knobs. // CDKTN-015, CDKTN-016
type ShardConfigSettings struct {
	ChainingAllowed         bool
	HeartbeatIntervalMillis int
	HeartbeatTimeoutSecs    int
	ElectionTimeoutMillis   int
	Members                 []MemberOverrideConfig // CDKTN-051: per-member overrides
}

// DefaultShardConfigSettings returns settings matching provider schema defaults. // CDKTN-016
func DefaultShardConfigSettings() *ShardConfigSettings {
	return &ShardConfigSettings{
		ChainingAllowed:         DefaultChainingAllowed,
		HeartbeatIntervalMillis: DefaultHeartbeatIntervalMillis,
		HeartbeatTimeoutSecs:    DefaultHeartbeatTimeoutSecs,
		ElectionTimeoutMillis:   DefaultElectionTimeoutMillis,
	}
}

// BalancerConfig configures the global balancer (mongodb_balancer_config). // BAL-001
type BalancerConfig struct {
	Enabled           bool
	ActiveWindowStart string // HH:MM format
	ActiveWindowStop  string // HH:MM format
	ChunkSizeMB       int
	SecondaryThrottle string
	WaitForDelete     *bool
}

// ShardZoneConfig maps a shard to a zone (mongodb_shard_zone). // ZONE-001
type ShardZoneConfig struct {
	ShardName string // RS name of the shard
	Zone      string
}

// ZoneKeyRangeConfig assigns a key range to a zone (mongodb_zone_key_range). // ZONE-014
type ZoneKeyRangeConfig struct {
	Namespace string // db.collection format
	Zone      string
	Min       string // JSON string
	Max       string // JSON string
}

// CollectionBalancingConfig manages per-collection balancing (mongodb_collection_balancing). // CBAL-001
type CollectionBalancingConfig struct {
	Namespace   string // db.collection format
	Enabled     bool
	ChunkSizeMB int
}

// ProfilerConfig manages per-database profiler settings (mongodb_profiler). // PROF-001
type ProfilerConfig struct {
	Database  string
	Level     int
	SlowMs    int
	RateLimit int
}

// ServerParameterConfig manages a server parameter (mongodb_server_parameter). // PARAM-001
type ServerParameterConfig struct {
	Parameter string
	Value     string
}

// FCVConfig manages featureCompatibilityVersion (mongodb_feature_compatibility_version). // FCV-001
type FCVConfig struct {
	Version    string
	DangerMode bool
}

// MongoShardProps configures a MongoShard L2 construct. // CDKTN-001
type MongoShardProps struct {
	ReplicaSetName   string
	Members          []MemberConfig
	Credentials      CredentialSource
	SSL              *SSLConfig
	Proxy            string
	Users            []UserConfig
	Roles            []RoleConfig
	ShardConfig      *ShardConfigSettings
	OriginalUsers    []OriginalUserConfig    // CDKTN-052
	Profilers        []ProfilerConfig        // per-node profiler configs
	ServerParameters []ServerParameterConfig // per-node server params
	FCV              *FCVConfig              // FCV on primary
}

// ConfigServerProps configures a MongoConfigServer L2 construct. // CDKTN-001
type ConfigServerProps struct {
	ReplicaSetName   string
	Members          []MemberConfig
	Credentials      CredentialSource
	SSL              *SSLConfig
	Proxy            string
	Users            []UserConfig
	Roles            []RoleConfig
	ShardConfig      *ShardConfigSettings
	OriginalUsers    []OriginalUserConfig    // CDKTN-052
	Profilers        []ProfilerConfig        // per-node profiler configs
	ServerParameters []ServerParameterConfig // per-node server params
	FCV              *FCVConfig              // FCV on primary
}

// MongosProps configures a MongoMongos L2 construct. // CDKTN-001
type MongosProps struct {
	Members          []MemberConfig
	Credentials      CredentialSource
	SSL              *SSLConfig
	Proxy            string
	Users            []UserConfig
	Roles            []RoleConfig
	OriginalUsers    []OriginalUserConfig    // CDKTN-052
	Profilers        []ProfilerConfig        // per-node profiler configs
	ServerParameters []ServerParameterConfig // per-node server params
}

// ShardConfig is an entry in MongoShardedClusterProps.Shards. // CDKTN-033
type ShardConfig struct {
	ReplicaSetName string
	Members        []MemberConfig
	Users          []UserConfig
	Roles          []RoleConfig
	ShardConfig    *ShardConfigSettings
	OriginalUsers  []OriginalUserConfig // CDKTN-052
}

// ConfigServerConfig is the config server definition in cluster props.
type ConfigServerConfig struct {
	ReplicaSetName string
	Members        []MemberConfig
	Users          []UserConfig
	Roles          []RoleConfig
	ShardConfig    *ShardConfigSettings
	OriginalUsers  []OriginalUserConfig // CDKTN-052
}

// MongosConfig is a mongos instance definition in cluster props.
type MongosConfig struct {
	Members       []MemberConfig
	Users         []UserConfig
	Roles         []RoleConfig
	OriginalUsers []OriginalUserConfig // CDKTN-052
}

// MongoShardedClusterProps configures the L3 cluster construct. // CDKTN-032
type MongoShardedClusterProps struct {
	Mongos          []MongosConfig
	ConfigServers   ConfigServerConfig
	Shards          []ShardConfig
	Credentials     CredentialSource // CDKTN-009: cluster-level credentials
	SSL             *SSLConfig       // CDKTN-018: cluster-level SSL
	Proxy           string           // CDKTN-034: cluster-level proxy
	Users           []UserConfig     // CDKTN-037: cluster-level users
	Roles           []RoleConfig
	ProviderVersion string               // CDKTN-042
	OriginalUsers   []OriginalUserConfig // CDKTN-052: cluster-level original users

	// Cluster-level resource configs
	RegisterShards      bool                        // opt-in: generate mongodb_shard resources via mongos
	Balancer            *BalancerConfig             // global balancer settings
	ShardZones          []ShardZoneConfig           // shard-to-zone mappings
	ZoneKeyRanges       []ZoneKeyRangeConfig        // zone key range assignments
	CollectionBalancing []CollectionBalancingConfig // per-collection balancing
	Profilers           []ProfilerConfig            // cluster-level profilers (cascade to all nodes)
	ServerParameters    []ServerParameterConfig     // cluster-level server params (cascade to all nodes)
	FCV                 *FCVConfig                  // cluster-level FCV (applied to primary of each RS)
}
