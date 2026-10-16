package mongodb

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseShardHost(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantRS    string
		wantHosts []string
		wantErr   bool
	}{
		{"DISC-T01 standard", "shard01/mongo1:27018,mongo2:27018,mongo3:27018", "shard01", []string{"mongo1:27018", "mongo2:27018", "mongo3:27018"}, false},
		{"DISC-T02 single host", "rs0/localhost:27017", "rs0", []string{"localhost:27017"}, false},
		{"DISC-T03 no slash", "localhost:27017", "", nil, true},
		{"DISC-T04 empty RS name", "/mongo1:27017", "", nil, true},
		{"DISC-T05 empty hosts", "shard01/", "", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rsName, hosts, err := ParseShardHost(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rsName != tc.wantRS {
				t.Errorf("rsName: want %s, got %s", tc.wantRS, rsName)
			}
			if !reflect.DeepEqual(hosts, tc.wantHosts) {
				t.Errorf("hosts: want %v, got %v", tc.wantHosts, hosts)
			}
		})
	}
}

func TestFindShardByName(t *testing.T) {
	type shardEntry = struct {
		ID    string `json:"_id" bson:"_id"`
		Host  string `json:"host" bson:"host"`
		State int    `json:"state" bson:"state"`
	}
	twoShards := &ShardList{Shards: []shardEntry{
		{ID: "shard01", Host: "shard01/mongo1:27018,mongo2:27018"},
		{ID: "shard02", Host: "shard02/mongo3:27019,mongo4:27019"},
	}}
	cases := []struct {
		name          string
		shards        *ShardList
		lookup        string
		wantHost      string
		wantErrSubstr []string
	}{
		{"DISC-T06 found", twoShards, "shard02", "shard02/mongo3:27019,mongo4:27019", nil},
		// DISC-T07: the error lists the available shards.
		{"DISC-T07 not found", twoShards, "shard99", "", []string{"shard01", "shard02"}},
		{"DISC-T08 empty list", &ShardList{}, "shard01", "", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, err := FindShardByName(tc.shards, tc.lookup)
			if tc.wantErrSubstr != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				for _, s := range tc.wantErrSubstr {
					if !strings.Contains(err.Error(), s) {
						t.Errorf("error should mention %q, got: %s", s, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != tc.wantHost {
				t.Errorf("host: want %s, got %s", tc.wantHost, host)
			}
		})
	}
}

func TestSplitHostPort(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantHost string
		wantPort string
		wantErr  bool
	}{
		{"DISC-T09 standard", "mongo1:27018", "mongo1", "27018", false},
		{"DISC-T10 default port", "mongo1", "mongo1", "27017", false},
		{"DISC-T11 empty", "", "", "", true},
		{"DISC-T12 empty host", ":27017", "", "", true},
		{"DISC-T13 empty port", "mongo1:", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, port, err := SplitHostPort(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != tc.wantHost {
				t.Errorf("host: want %s, got %s", tc.wantHost, host)
			}
			if port != tc.wantPort {
				t.Errorf("port: want %s, got %s", tc.wantPort, port)
			}
		})
	}
}

// DISC-T14..T17: the shard client inherits credentials, TLS and proxy from
// the provider config and always connects direct to the given member.
func TestBuildShardClientConfig(t *testing.T) {
	cert := "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----"
	cases := []struct {
		name     string
		provider *ClientConfig
		host     string
		port     string
		rs       string
		want     ClientConfig
	}{
		{"DISC-T14 inherits credentials", &ClientConfig{Username: "admin", Password: "secret", DB: "admin"}, "mongo1", "27018", "shard01",
			ClientConfig{Username: "admin", Password: "secret", DB: "admin", Host: "mongo1", Port: "27018", ReplicaSet: "shard01", Direct: true}},
		{"DISC-T15 direct mode", &ClientConfig{}, "mongo1", "27018", "shard01",
			ClientConfig{Host: "mongo1", Port: "27018", ReplicaSet: "shard01", Direct: true}},
		{"DISC-T16 inherits TLS", &ClientConfig{Ssl: true, InsecureSkipVerify: true, Certificate: cert}, "mongo1", "27018", "rs0",
			ClientConfig{Ssl: true, InsecureSkipVerify: true, Certificate: cert, Host: "mongo1", Port: "27018", ReplicaSet: "rs0", Direct: true}},
		{"DISC-T17 inherits proxy", &ClientConfig{Proxy: "socks5://proxy:1080"}, "mongo1", "27018", "rs0",
			ClientConfig{Proxy: "socks5://proxy:1080", Host: "mongo1", Port: "27018", ReplicaSet: "rs0", Direct: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildShardClientConfig(tc.provider, tc.host, tc.port, tc.rs)
			if !reflect.DeepEqual(*got, tc.want) {
				t.Errorf("want %+v, got %+v", tc.want, *got)
			}
		})
	}
}

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
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.ct.String(); got != tc.want {
				t.Errorf("ConnectionType(%d).String(): want %q, got %q", tc.ct, tc.want, got)
			}
		})
	}
}
