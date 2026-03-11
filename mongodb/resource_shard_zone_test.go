package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// ZONE-T01: ZONE-001 — Schema has all expected fields with correct types
func TestShardZoneSchema_AllFields(t *testing.T) {
	res := resourceShardZone()
	expected := map[string]schema.ValueType{
		"shard_name":       schema.TypeString,
		"zone":             schema.TypeString,
		"planned_commands": schema.TypeString,
	}
	for name, typ := range expected {
		field, ok := res.Schema[name]
		if !ok {
			t.Errorf("schema missing field %q", name)
			continue
		}
		if field.Type != typ {
			t.Errorf("field %q: want type %v, got %v", name, typ, field.Type)
		}
	}
}

// ZONE-T02: ZONE-001 — shard_name is Required and not ForceNew
func TestShardZoneSchema_ShardName(t *testing.T) {
	res := resourceShardZone()
	field := res.Schema["shard_name"]
	if !field.Required {
		t.Error("shard_name should be Required")
	}
	if field.ForceNew {
		t.Error("shard_name should not be ForceNew (DANGER-010); use CustomizeDiff instead")
	}
}

// ZONE-T03: ZONE-001 — zone is Required and not ForceNew
func TestShardZoneSchema_Zone(t *testing.T) {
	res := resourceShardZone()
	field := res.Schema["zone"]
	if !field.Required {
		t.Error("zone should be Required")
	}
	if field.ForceNew {
		t.Error("zone should not be ForceNew (DANGER-010); use CustomizeDiff instead")
	}
}

// ZONE-T04: ZONE-001 — planned_commands is Computed
func TestShardZoneSchema_PlannedCommands(t *testing.T) {
	res := resourceShardZone()
	field := res.Schema["planned_commands"]
	if !field.Computed {
		t.Error("planned_commands should be Computed")
	}
}

// ZONE-T05: ZONE-009 — formatShardZoneID produces correct format
func TestFormatShardZoneID(t *testing.T) {
	tests := []struct {
		shard, zone, want string
	}{
		{"shard01", "US-East", "shard01:US-East"},
		{"rs0", "zone-a", "rs0:zone-a"},
	}
	for _, tt := range tests {
		got := formatShardZoneID(tt.shard, tt.zone)
		if got != tt.want {
			t.Errorf("formatShardZoneID(%q, %q) = %q, want %q", tt.shard, tt.zone, got, tt.want)
		}
	}
}

// ZONE-T06: ZONE-009 — parseShardZoneID round-trips with formatShardZoneID
func TestParseShardZoneID_RoundTrip(t *testing.T) {
	tests := []struct {
		shard, zone string
	}{
		{"shard01", "US-East"},
		{"rs0", "zone-a"},
		{"my-shard", "datacenter:rack1"},
	}
	for _, tt := range tests {
		id := formatShardZoneID(tt.shard, tt.zone)
		shard, zone, err := parseShardZoneID(id)
		if err != nil {
			t.Errorf("parseShardZoneID(%q): unexpected error: %v", id, err)
			continue
		}
		if shard != tt.shard || zone != tt.zone {
			t.Errorf("parseShardZoneID(%q) = (%q, %q), want (%q, %q)", id, shard, zone, tt.shard, tt.zone)
		}
	}
}

// ZONE-T07: ZONE-009 — parseShardZoneID rejects invalid IDs
func TestParseShardZoneID_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"noseparator",
		":zone",
		"shard:",
	}
	for _, id := range invalid {
		_, _, err := parseShardZoneID(id)
		if err == nil {
			t.Errorf("parseShardZoneID(%q): expected error, got nil", id)
		}
	}
}
