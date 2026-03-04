package mongodb

import (
	"testing"
)

// --- ParseShardHost tests ---

// DISC-T01: Standard "rsName/host1:port,host2:port" format
func TestParseShardHost_Standard(t *testing.T) {
	rsName, hosts, err := ParseShardHost("shard01/mongo1:27018,mongo2:27018,mongo3:27018")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rsName != "shard01" {
		t.Errorf("rsName: want shard01, got %s", rsName)
	}
	if len(hosts) != 3 {
		t.Fatalf("hosts: want 3, got %d", len(hosts))
	}
	if hosts[0] != "mongo1:27018" || hosts[1] != "mongo2:27018" || hosts[2] != "mongo3:27018" {
		t.Errorf("hosts: want [mongo1:27018 mongo2:27018 mongo3:27018], got %v", hosts)
	}
}

// DISC-T02: Single host
func TestParseShardHost_SingleHost(t *testing.T) {
	rsName, hosts, err := ParseShardHost("rs0/localhost:27017")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rsName != "rs0" {
		t.Errorf("rsName: want rs0, got %s", rsName)
	}
	if len(hosts) != 1 || hosts[0] != "localhost:27017" {
		t.Errorf("hosts: want [localhost:27017], got %v", hosts)
	}
}

// DISC-T03: No slash returns error
func TestParseShardHost_NoSlash(t *testing.T) {
	_, _, err := ParseShardHost("localhost:27017")
	if err == nil {
		t.Fatal("expected error for missing slash, got nil")
	}
}

// DISC-T04: Empty RS name returns error
func TestParseShardHost_EmptyRSName(t *testing.T) {
	_, _, err := ParseShardHost("/mongo1:27017")
	if err == nil {
		t.Fatal("expected error for empty RS name, got nil")
	}
}

// DISC-T05: Empty hosts after slash returns error
func TestParseShardHost_EmptyHosts(t *testing.T) {
	_, _, err := ParseShardHost("shard01/")
	if err == nil {
		t.Fatal("expected error for empty hosts, got nil")
	}
}

// --- FindShardByName tests ---

// DISC-T06: Shard found by name
func TestFindShardByName_Found(t *testing.T) {
	shards := &ShardList{
		Shards: []struct {
			ID    string `json:"_id" bson:"_id"`
			Host  string `json:"host" bson:"host"`
			State int    `json:"state" bson:"state"`
		}{
			{ID: "shard01", Host: "shard01/mongo1:27018,mongo2:27018"},
			{ID: "shard02", Host: "shard02/mongo3:27019,mongo4:27019"},
		},
	}
	host, err := FindShardByName(shards, "shard02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "shard02/mongo3:27019,mongo4:27019" {
		t.Errorf("host: want shard02/mongo3:27019,mongo4:27019, got %s", host)
	}
}

// DISC-T07: Shard not found lists available shards
func TestFindShardByName_NotFound(t *testing.T) {
	shards := &ShardList{
		Shards: []struct {
			ID    string `json:"_id" bson:"_id"`
			Host  string `json:"host" bson:"host"`
			State int    `json:"state" bson:"state"`
		}{
			{ID: "shard01", Host: "shard01/mongo1:27018"},
			{ID: "shard02", Host: "shard02/mongo3:27019"},
		},
	}
	_, err := FindShardByName(shards, "shard99")
	if err == nil {
		t.Fatal("expected error for missing shard, got nil")
	}
	// Error should list available shards
	errStr := err.Error()
	if !contains(errStr, "shard01") || !contains(errStr, "shard02") {
		t.Errorf("error should list available shards, got: %s", errStr)
	}
}

// DISC-T08: Empty shard list
func TestFindShardByName_EmptyList(t *testing.T) {
	shards := &ShardList{}
	_, err := FindShardByName(shards, "shard01")
	if err == nil {
		t.Fatal("expected error for empty shard list, got nil")
	}
}

// --- SplitHostPort tests ---

// DISC-T09: Standard host:port
func TestSplitHostPort_Standard(t *testing.T) {
	host, port, err := SplitHostPort("mongo1:27018")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "mongo1" {
		t.Errorf("host: want mongo1, got %s", host)
	}
	if port != "27018" {
		t.Errorf("port: want 27018, got %s", port)
	}
}

// DISC-T10: No port defaults to 27017
func TestSplitHostPort_DefaultPort(t *testing.T) {
	host, port, err := SplitHostPort("mongo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "mongo1" {
		t.Errorf("host: want mongo1, got %s", host)
	}
	if port != "27017" {
		t.Errorf("port: want 27017, got %s", port)
	}
}

// DISC-T11: Empty string returns error
func TestSplitHostPort_Empty(t *testing.T) {
	_, _, err := SplitHostPort("")
	if err == nil {
		t.Fatal("expected error for empty string, got nil")
	}
}

// DISC-T12: Empty host (":27017") returns error
func TestSplitHostPort_EmptyHost(t *testing.T) {
	_, _, err := SplitHostPort(":27017")
	if err == nil {
		t.Fatal("expected error for empty host, got nil")
	}
}

// DISC-T13: Host with empty port ("mongo1:") returns error
func TestSplitHostPort_EmptyPort(t *testing.T) {
	_, _, err := SplitHostPort("mongo1:")
	if err == nil {
		t.Fatal("expected error for empty port, got nil")
	}
}

// --- BuildShardClientConfig tests ---

// DISC-T14: Inherits credentials from provider config
func TestBuildShardClientConfig_InheritsCredentials(t *testing.T) {
	providerCfg := &ClientConfig{
		Username: "admin",
		Password: "secret",
		DB:       "admin",
	}
	cfg := BuildShardClientConfig(providerCfg, "mongo1", "27018", "shard01")
	if cfg.Username != "admin" {
		t.Errorf("Username: want admin, got %s", cfg.Username)
	}
	if cfg.Password != "secret" {
		t.Errorf("Password: want secret, got %s", cfg.Password)
	}
	if cfg.DB != "admin" {
		t.Errorf("DB: want admin, got %s", cfg.DB)
	}
}

// DISC-T15: Sets direct mode and correct host/port/replicaSet
func TestBuildShardClientConfig_DirectMode(t *testing.T) {
	providerCfg := &ClientConfig{}
	cfg := BuildShardClientConfig(providerCfg, "mongo1", "27018", "shard01")
	if !cfg.Direct {
		t.Error("Direct: want true, got false")
	}
	if cfg.Host != "mongo1" {
		t.Errorf("Host: want mongo1, got %s", cfg.Host)
	}
	if cfg.Port != "27018" {
		t.Errorf("Port: want 27018, got %s", cfg.Port)
	}
	if cfg.ReplicaSet != "shard01" {
		t.Errorf("ReplicaSet: want shard01, got %s", cfg.ReplicaSet)
	}
}

// DISC-T16: Inherits TLS config
func TestBuildShardClientConfig_InheritsTLS(t *testing.T) {
	providerCfg := &ClientConfig{
		Ssl:                true,
		InsecureSkipVerify: true,
		Certificate:        "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----",
	}
	cfg := BuildShardClientConfig(providerCfg, "mongo1", "27018", "rs0")
	if !cfg.Ssl {
		t.Error("Ssl: want true, got false")
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify: want true, got false")
	}
	if cfg.Certificate != providerCfg.Certificate {
		t.Error("Certificate: not inherited from provider")
	}
}

// DISC-T17: Inherits proxy
func TestBuildShardClientConfig_InheritsProxy(t *testing.T) {
	providerCfg := &ClientConfig{
		Proxy: "socks5://proxy:1080",
	}
	cfg := BuildShardClientConfig(providerCfg, "mongo1", "27018", "rs0")
	if cfg.Proxy != "socks5://proxy:1080" {
		t.Errorf("Proxy: want socks5://proxy:1080, got %s", cfg.Proxy)
	}
}

// --- DetectConnectionType tests ---

// DISC-T18: Mongos detected from Msg field
func TestDetectConnectionType_Mongos(t *testing.T) {
	resp := &IsMasterResp{Msg: "isdbgrid"}
	ct := classifyConnectionType(resp)
	if ct != ConnTypeMongos {
		t.Errorf("want ConnTypeMongos, got %s", ct)
	}
}

// DISC-T19: Replica set detected from SetName field
func TestDetectConnectionType_ReplicaSet(t *testing.T) {
	resp := &IsMasterResp{SetName: "shard01"}
	ct := classifyConnectionType(resp)
	if ct != ConnTypeReplicaSet {
		t.Errorf("want ConnTypeReplicaSet, got %s", ct)
	}
}

// DISC-T20: Standalone when neither msg nor setName
func TestDetectConnectionType_Standalone(t *testing.T) {
	resp := &IsMasterResp{}
	ct := classifyConnectionType(resp)
	if ct != ConnTypeStandalone {
		t.Errorf("want ConnTypeStandalone, got %s", ct)
	}
}

// --- Schema test ---

// DISC-T21: host_override attribute exists in schema
func TestShardConfigSchema_HostOverride(t *testing.T) {
	res := resourceShardConfig()
	s, ok := res.Schema["host_override"]
	if !ok {
		t.Fatal("schema missing 'host_override' field")
	}
	if s.Required {
		t.Error("host_override should be Optional, not Required")
	}
}

// --- ConnectionType.String() coverage ---

// DISC-T22: String() returns human-readable labels
func TestConnectionType_String(t *testing.T) {
	cases := []struct {
		ct   ConnectionType
		want string
	}{
		{ConnTypeMongos, "mongos"},
		{ConnTypeReplicaSet, "replica_set"},
		{ConnTypeStandalone, "standalone"},
		{ConnectionType(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.ct.String(); got != tc.want {
			t.Errorf("ConnectionType(%d).String(): want %q, got %q", tc.ct, tc.want, got)
		}
	}
}

// helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
