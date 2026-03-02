package mongodb

import (
	"encoding/base64"
	"testing"
)

// TEST-007: Valid base64 ID returns (shardName, database, nil)
func TestResourceShardConfigParseId_Valid(t *testing.T) {
	r := &ResourceShardConfig{}
	id := base64.StdEncoding.EncodeToString([]byte("admin.shard01"))
	shardName, database, err := r.ParseId(id)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if shardName != "shard01" {
		t.Errorf("expected shardName 'shard01', got '%s'", shardName)
	}
	if database != "admin" {
		t.Errorf("expected database 'admin', got '%s'", database)
	}
}

// TEST-008: Invalid inputs return errors
func TestResourceShardConfigParseId_InvalidInputs(t *testing.T) {
	r := &ResourceShardConfig{}
	cases := []struct {
		name string
		id   string
		raw  bool
	}{
		{"invalid base64", "not-valid!@#", true},
		{"no separator", "nodotshere", false},
		{"empty database", ".shardName", false},
		{"empty shardName", "database.", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := tc.id
			if !tc.raw {
				id = base64.StdEncoding.EncodeToString([]byte(tc.id))
			}
			_, _, err := r.ParseId(id)
			if err == nil {
				t.Fatalf("expected error for case %q, got nil", tc.name)
			}
		})
	}
}
