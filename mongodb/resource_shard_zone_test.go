package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// ZONE-T01..T04: ZONE-001 — identity fields Required (immutable via
// CustomizeDiff, not ForceNew), planned_commands Computed.
func TestShardZoneSchema(t *testing.T) {
	checkSchemaFields(t, resourceShardZone(), []schemaField{
		{name: "shard_name", typ: schema.TypeString, required: true},
		{name: "zone", typ: schema.TypeString, required: true},
		{name: "planned_commands", typ: schema.TypeString, computed: true},
	})
}

// ZONE-T05, ZONE-T06: ZONE-009 — formatShardZoneID produces "shard:zone" and
// parseShardZoneID round-trips it, even when the zone contains the separator.
func TestShardZoneID_RoundTrip(t *testing.T) {
	cases := []struct {
		shard, zone, wantID string
	}{
		{"shard01", "US-East", "shard01:US-East"},
		{"rs0", "zone-a", "rs0:zone-a"},
		{"my-shard", "datacenter:rack1", "my-shard:datacenter:rack1"},
	}
	for _, tc := range cases {
		t.Run(tc.wantID, func(t *testing.T) {
			id := formatShardZoneID(tc.shard, tc.zone)
			if id != tc.wantID {
				t.Errorf("formatShardZoneID(%q, %q) = %q, want %q", tc.shard, tc.zone, id, tc.wantID)
			}
			shard, zone, err := parseShardZoneID(id)
			if err != nil {
				t.Fatalf("parseShardZoneID(%q): unexpected error: %v", id, err)
			}
			if shard != tc.shard || zone != tc.zone {
				t.Errorf("parseShardZoneID(%q) = (%q, %q), want (%q, %q)", id, shard, zone, tc.shard, tc.zone)
			}
		})
	}
}

// ZONE-T07: ZONE-009 — parseShardZoneID rejects invalid IDs
func TestParseShardZoneID_Invalid(t *testing.T) {
	for _, id := range []string{"", "noseparator", ":zone", "shard:"} {
		t.Run(id, func(t *testing.T) {
			if _, _, err := parseShardZoneID(id); err == nil {
				t.Errorf("parseShardZoneID(%q): expected error, got nil", id)
			}
		})
	}
}
