package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// --- BuildShardConnectionString tests ---

// CLUS-T01: BuildShardConnectionString with 3 hosts
func TestBuildShardConnectionString_ThreeHosts(t *testing.T) {
	result := BuildShardConnectionString("shard01", []string{
		"mongo1:27017", "mongo2:27017", "mongo3:27017",
	})
	expected := "shard01/mongo1:27017,mongo2:27017,mongo3:27017"
	if result != expected {
		t.Errorf("want %q, got %q", expected, result)
	}
}

// CLUS-T02: BuildShardConnectionString with 1 host
func TestBuildShardConnectionString_SingleHost(t *testing.T) {
	result := BuildShardConnectionString("rs0", []string{"localhost:27018"})
	expected := "rs0/localhost:27018"
	if result != expected {
		t.Errorf("want %q, got %q", expected, result)
	}
}

// CLUS-T03: Schema: shard_name Required, ForceNew
func TestShardSchema_ShardName(t *testing.T) {
	res := resourceShard()
	field, ok := res.Schema["shard_name"]
	if !ok {
		t.Fatal("schema missing 'shard_name' field")
	}
	if !field.Required {
		t.Error("shard_name should be Required")
	}
	if !field.ForceNew {
		t.Error("shard_name should be ForceNew")
	}
}

// CLUS-T04: DANGER-016 — hosts Required, TypeList, immutable via CustomizeDiff (not ForceNew)
func TestShardSchema_Hosts(t *testing.T) {
	res := resourceShard()
	field, ok := res.Schema["hosts"]
	if !ok {
		t.Fatal("schema missing 'hosts' field")
	}
	if !field.Required {
		t.Error("hosts should be Required")
	}
	if field.Type != schema.TypeList {
		t.Errorf("hosts type: want TypeList, got %v", field.Type)
	}
	if field.ForceNew {
		t.Error("hosts should not be ForceNew (DANGER-010); use CustomizeDiff instead")
	}
}

// CLUS-T05: Schema: state Computed
func TestShardSchema_State(t *testing.T) {
	res := resourceShard()
	field, ok := res.Schema["state"]
	if !ok {
		t.Fatal("schema missing 'state' field")
	}
	if !field.Computed {
		t.Error("state should be Computed")
	}
}

// CLUS-T06: Schema: remove_timeout_secs Optional, Default 300
func TestShardSchema_RemoveTimeoutSecs(t *testing.T) {
	res := resourceShard()
	field, ok := res.Schema["remove_timeout_secs"]
	if !ok {
		t.Fatal("schema missing 'remove_timeout_secs' field")
	}
	if field.Required {
		t.Error("remove_timeout_secs should be Optional, not Required")
	}
	if field.Default != DefaultRemoveTimeoutSecs {
		t.Errorf("remove_timeout_secs default: want %d, got %v", DefaultRemoveTimeoutSecs, field.Default)
	}
}
