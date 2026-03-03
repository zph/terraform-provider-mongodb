package mongodb

import (
	"encoding/base64"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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

// --- MergeMembers tests ---

func threeNodeRS() ConfigMembers {
	return ConfigMembers{
		{ID: 0, Host: "mongo1:27017", Priority: 1, Votes: intPtr(1), Hidden: boolPtr(false), ArbiterOnly: boolPtr(false), BuildIndexes: boolPtr(true)},
		{ID: 1, Host: "mongo2:27017", Priority: 1, Votes: intPtr(1), Hidden: boolPtr(false), ArbiterOnly: boolPtr(false), BuildIndexes: boolPtr(true)},
		{ID: 2, Host: "mongo3:27017", Priority: 1, Votes: intPtr(1), Hidden: boolPtr(false), ArbiterOnly: boolPtr(false), BuildIndexes: boolPtr(true)},
	}
}

// SHARD-T01: SHARD-005 — One override changes only the matched member's priority
func TestMergeMembers_SingleMemberPriority(t *testing.T) {
	rs := threeNodeRS()
	overrides := []MemberOverride{
		{Host: "mongo2:27017", Priority: 5, Votes: 1, BuildIndexes: true},
	}
	result, err := MergeMembers(rs, overrides)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[1].Priority != 5 {
		t.Errorf("expected priority 5 on mongo2, got %d", result[1].Priority)
	}
	if result[0].Priority != 1 {
		t.Errorf("expected priority 1 on mongo1 unchanged, got %d", result[0].Priority)
	}
	if result[2].Priority != 1 {
		t.Errorf("expected priority 1 on mongo3 unchanged, got %d", result[2].Priority)
	}
}

// SHARD-T02: SHARD-005 — Two overrides on different hosts
func TestMergeMembers_MultipleMemberOverrides(t *testing.T) {
	rs := threeNodeRS()
	overrides := []MemberOverride{
		{Host: "mongo1:27017", Priority: 3, Votes: 1, BuildIndexes: true},
		{Host: "mongo3:27017", Priority: 0, Votes: 0, Hidden: true, BuildIndexes: true},
	}
	result, err := MergeMembers(rs, overrides)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].Priority != 3 {
		t.Errorf("expected mongo1 priority=3, got %d", result[0].Priority)
	}
	if result[2].Priority != 0 {
		t.Errorf("expected mongo3 priority=0, got %d", result[2].Priority)
	}
	if *result[2].Hidden != true {
		t.Errorf("expected mongo3 hidden=true, got %v", *result[2].Hidden)
	}
	if *result[2].Votes != 0 {
		t.Errorf("expected mongo3 votes=0, got %d", *result[2].Votes)
	}
}

// SHARD-T03: SHARD-004 — Error for unknown host
func TestMergeMembers_HostNotFound(t *testing.T) {
	rs := threeNodeRS()
	overrides := []MemberOverride{
		{Host: "unknown:27017", Priority: 1},
	}
	_, err := MergeMembers(rs, overrides)
	if err == nil {
		t.Fatal("expected error for unknown host, got nil")
	}
}

// SHARD-T04: SHARD-002 — Empty overrides = no changes
func TestMergeMembers_EmptyTFMembers(t *testing.T) {
	rs := threeNodeRS()
	result, err := MergeMembers(rs, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := range rs {
		if result[i].Host != rs[i].Host || result[i].Priority != rs[i].Priority {
			t.Errorf("member %d changed unexpectedly", i)
		}
	}
}

// SHARD-T05: SHARD-005 — All fields applied
func TestMergeMembers_AllFields(t *testing.T) {
	rs := threeNodeRS()
	overrides := []MemberOverride{
		{
			Host:         "mongo1:27017",
			Priority:     10,
			Votes:        1,
			Hidden:       true,
			ArbiterOnly:  false,
			BuildIndexes: false,
			Tags:         map[string]string{"dc": "east", "rack": "r1"},
		},
	}
	result, err := MergeMembers(rs, overrides)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result[0]
	if m.Priority != 10 {
		t.Errorf("priority: want 10, got %d", m.Priority)
	}
	if *m.Votes != 1 {
		t.Errorf("votes: want 1, got %d", *m.Votes)
	}
	if *m.Hidden != true {
		t.Errorf("hidden: want true, got %v", *m.Hidden)
	}
	if *m.ArbiterOnly != false {
		t.Errorf("arbiterOnly: want false, got %v", *m.ArbiterOnly)
	}
	if *m.BuildIndexes != false {
		t.Errorf("buildIndexes: want false, got %v", *m.BuildIndexes)
	}
	if m.Tags["dc"] != "east" || m.Tags["rack"] != "r1" {
		t.Errorf("tags: want {dc:east, rack:r1}, got %v", m.Tags)
	}
}

// SHARD-T06: SHARD-005 — Declaring a member sets all its fields
func TestMergeMembers_PartialFields(t *testing.T) {
	rs := threeNodeRS()
	overrides := []MemberOverride{
		{Host: "mongo1:27017", Priority: 5, Votes: 1, BuildIndexes: true,
			Tags: map[string]string{"zone": "a"}},
	}
	result, err := MergeMembers(rs, overrides)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := result[0]
	if m.Priority != 5 {
		t.Errorf("priority: want 5, got %d", m.Priority)
	}
	if m.Tags["zone"] != "a" {
		t.Errorf("tags: want {zone:a}, got %v", m.Tags)
	}
	// hidden should be set to the override value (false, the zero value)
	if *m.Hidden != false {
		t.Errorf("hidden: want false, got %v", *m.Hidden)
	}
}

// SHARD-T07: SHARD-009 — Tags replaced entirely, not merged
func TestMergeMembers_TagsReplace(t *testing.T) {
	rs := threeNodeRS()
	rs[0].Tags = ReplsetTags{"old": "value", "keep": "this"}

	overrides := []MemberOverride{
		{Host: "mongo1:27017", Priority: 1, Votes: 1, BuildIndexes: true,
			Tags: map[string]string{"dc": "east"}},
	}
	result, err := MergeMembers(rs, overrides)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result[0].Tags["old"]; ok {
		t.Error("old tag should have been replaced, not merged")
	}
	if result[0].Tags["dc"] != "east" {
		t.Errorf("new tag missing: want dc=east, got %v", result[0].Tags)
	}
}

// SHARD-T08: SHARD-006 — Unlisted members byte-for-byte identical
func TestMergeMembers_UnlistedPreserved(t *testing.T) {
	rs := threeNodeRS()
	rs[1].Tags = ReplsetTags{"existing": "tag"}
	rs[1].Priority = 7

	overrides := []MemberOverride{
		{Host: "mongo1:27017", Priority: 2, Votes: 1, BuildIndexes: true},
	}
	result, err := MergeMembers(rs, overrides)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// mongo2 should be completely unchanged
	if result[1].Priority != 7 {
		t.Errorf("unlisted member priority changed: want 7, got %d", result[1].Priority)
	}
	if result[1].Tags["existing"] != "tag" {
		t.Errorf("unlisted member tags changed: want {existing:tag}, got %v", result[1].Tags)
	}
	// mongo3 should also be unchanged
	if result[2].Priority != 1 {
		t.Errorf("unlisted member mongo3 priority changed: want 1, got %d", result[2].Priority)
	}
}

// --- RSConfigMembersToState tests ---

// SHARD-T09: SHARD-007 — Full conversion with all fields
func TestRSConfigMembersToState_AllFields(t *testing.T) {
	members := ConfigMembers{
		{
			ID: 0, Host: "mongo1:27017", Priority: 5,
			Votes: intPtr(1), Hidden: boolPtr(true), ArbiterOnly: boolPtr(false),
			BuildIndexes: boolPtr(true),
			Tags:         ReplsetTags{"dc": "east"},
		},
	}
	managed := map[string]bool{"mongo1:27017": true}
	result := RSConfigMembersToState(members, managed)
	if len(result) != 1 {
		t.Fatalf("expected 1 member, got %d", len(result))
	}
	m := result[0].(map[string]interface{})
	if m["host"] != "mongo1:27017" {
		t.Errorf("host: want mongo1:27017, got %v", m["host"])
	}
	if m["priority"] != 5 {
		t.Errorf("priority: want 5, got %v", m["priority"])
	}
	if m["votes"] != 1 {
		t.Errorf("votes: want 1, got %v", m["votes"])
	}
	if m["hidden"] != true {
		t.Errorf("hidden: want true, got %v", m["hidden"])
	}
	if m["arbiter_only"] != false {
		t.Errorf("arbiter_only: want false, got %v", m["arbiter_only"])
	}
	if m["build_indexes"] != true {
		t.Errorf("build_indexes: want true, got %v", m["build_indexes"])
	}
	tags := m["tags"].(map[string]interface{})
	if tags["dc"] != "east" {
		t.Errorf("tags: want dc=east, got %v", tags)
	}
}

// SHARD-T10: SHARD-008 — Only managed hosts returned
func TestRSConfigMembersToState_ManagedFilter(t *testing.T) {
	members := ConfigMembers{
		{ID: 0, Host: "mongo1:27017", Priority: 1},
		{ID: 1, Host: "mongo2:27017", Priority: 2},
		{ID: 2, Host: "mongo3:27017", Priority: 3},
	}
	managed := map[string]bool{"mongo2:27017": true}
	result := RSConfigMembersToState(members, managed)
	if len(result) != 1 {
		t.Fatalf("expected 1 member, got %d", len(result))
	}
	m := result[0].(map[string]interface{})
	if m["host"] != "mongo2:27017" {
		t.Errorf("expected mongo2, got %v", m["host"])
	}
}

// SHARD-T11: SHARD-002 — Nil managed hosts returns nil
func TestRSConfigMembersToState_NilManaged(t *testing.T) {
	members := ConfigMembers{
		{ID: 0, Host: "mongo1:27017"},
	}
	result := RSConfigMembersToState(members, nil)
	if result != nil {
		t.Errorf("expected nil for nil managedHosts, got %v", result)
	}
}

// SHARD-T12: Nil pointer fields don't panic
func TestRSConfigMembersToState_NilPointers(t *testing.T) {
	members := ConfigMembers{
		{ID: 0, Host: "mongo1:27017", Priority: 1},
	}
	managed := map[string]bool{"mongo1:27017": true}
	result := RSConfigMembersToState(members, managed)
	if len(result) != 1 {
		t.Fatalf("expected 1 member, got %d", len(result))
	}
	m := result[0].(map[string]interface{})
	if m["votes"] != 0 {
		t.Errorf("nil votes should deref to 0, got %v", m["votes"])
	}
	if m["hidden"] != false {
		t.Errorf("nil hidden should deref to false, got %v", m["hidden"])
	}
}

// SHARD-T13: SHARD-001 — Schema has correct member sub-fields
func TestShardConfigSchema_MemberBlock(t *testing.T) {
	res := resourceShardConfig()
	memberSchema, ok := res.Schema["member"]
	if !ok {
		t.Fatal("schema missing 'member' field")
	}
	if memberSchema.Required {
		t.Error("member should be Optional, not Required")
	}

	elem, ok := memberSchema.Elem.(*schema.Resource)
	if !ok {
		t.Fatal("member Elem should be *schema.Resource")
	}

	expectedFields := []string{
		"host", "tags", "priority", "votes", "hidden",
		"arbiter_only", "build_indexes",
	}
	for _, f := range expectedFields {
		if _, exists := elem.Schema[f]; !exists {
			t.Errorf("member sub-schema missing field %q", f)
		}
	}

	if !elem.Schema["host"].Required {
		t.Error("member.host should be Required")
	}
}
