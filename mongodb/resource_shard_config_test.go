package mongodb

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

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
		t.Errorf("expected priority 5 on mongo2, got %v", result[1].Priority)
	}
	if result[0].Priority != 1 {
		t.Errorf("expected priority 1 on mongo1 unchanged, got %v", result[0].Priority)
	}
	if result[2].Priority != 1 {
		t.Errorf("expected priority 1 on mongo3 unchanged, got %v", result[2].Priority)
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
		t.Errorf("expected mongo1 priority=3, got %v", result[0].Priority)
	}
	if result[2].Priority != 0 {
		t.Errorf("expected mongo3 priority=0, got %v", result[2].Priority)
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
		t.Errorf("priority: want 10, got %v", m.Priority)
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
		t.Errorf("priority: want 5, got %v", m.Priority)
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
	rs[1].Priority = 7.0

	overrides := []MemberOverride{
		{Host: "mongo1:27017", Priority: 2, Votes: 1, BuildIndexes: true},
	}
	result, err := MergeMembers(rs, overrides)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// mongo2 should be completely unchanged
	if result[1].Priority != 7 {
		t.Errorf("unlisted member priority changed: want 7, got %v", result[1].Priority)
	}
	if result[1].Tags["existing"] != "tag" {
		t.Errorf("unlisted member tags changed: want {existing:tag}, got %v", result[1].Tags)
	}
	// mongo3 should also be unchanged
	if result[2].Priority != 1 {
		t.Errorf("unlisted member mongo3 priority changed: want 1, got %v", result[2].Priority)
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
	if m["priority"] != 5.0 {
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

// --- Oplog Configuration tests ---

// OPLOG-T01: OPLOG-001/002 — Schema has oplog_size_mb field, Optional TypeFloat
func TestShardConfigSchema_OplogSizeMB(t *testing.T) {
	res := resourceShardConfig()
	field, ok := res.Schema["oplog_size_mb"]
	if !ok {
		t.Fatal("schema missing 'oplog_size_mb' field")
	}
	if field.Required {
		t.Error("oplog_size_mb should be Optional")
	}
	if field.Type != schema.TypeFloat {
		t.Errorf("oplog_size_mb should be TypeFloat, got %v", field.Type)
	}
	if field.ValidateFunc == nil {
		t.Error("oplog_size_mb should have a ValidateFunc")
	}
}

// --- CatchUp Timeout tests ---

// CATCHUP-T01: CATCHUP-001 — Schema has catch_up_timeout_millis, Optional TypeInt Default -1
func TestShardConfigSchema_CatchUpTimeoutMillis(t *testing.T) {
	res := resourceShardConfig()
	field, ok := res.Schema["catch_up_timeout_millis"]
	if !ok {
		t.Fatal("schema missing 'catch_up_timeout_millis' field")
	}
	if field.Required {
		t.Error("catch_up_timeout_millis should be Optional")
	}
	if field.Type != schema.TypeInt {
		t.Errorf("catch_up_timeout_millis should be TypeInt, got %v", field.Type)
	}
	if field.Default != -1 {
		t.Errorf("catch_up_timeout_millis default should be -1, got %v", field.Default)
	}
}

// OPLOG-T02: BytesPerMB constant is correct
func TestBytesPerMB(t *testing.T) {
	if BytesPerMB != 1048576 {
		t.Errorf("BytesPerMB should be 1048576, got %d", BytesPerMB)
	}
}

// --- Oplog member fan-out tests ---

// OPLOG-T03: OPLOG-009/010 — resize reaches every data-bearing member,
// secondaries first and the primary last. Regression: previously only the
// single member the provider was connected to was resized.
func TestResizeOplogAcrossMembers_AllMembersPrimaryLast(t *testing.T) {
	rs := threeNodeRS()
	var resized []string
	err := ResizeOplogAcrossMembers(context.Background(), rs, "mongo1:27017", func(_ context.Context, host string) error {
		resized = append(resized, host)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"mongo2:27017", "mongo3:27017", "mongo1:27017"}
	if len(resized) != len(want) {
		t.Fatalf("expected %d members resized, got %d: %v", len(want), len(resized), resized)
	}
	for i := range want {
		if resized[i] != want[i] {
			t.Errorf("resize order[%d]: want %s, got %s", i, want[i], resized[i])
		}
	}
}

// OPLOG-T04: OPLOG-011 — arbiters carry no oplog and are skipped
func TestOplogMemberHosts_SkipsArbiters(t *testing.T) {
	rs := threeNodeRS()
	rs[2].ArbiterOnly = boolPtr(true)
	hosts := OplogMemberHosts(rs, "mongo1:27017")
	want := []string{"mongo2:27017", "mongo1:27017"}
	if len(hosts) != len(want) {
		t.Fatalf("expected %d hosts, got %d: %v", len(want), len(hosts), hosts)
	}
	for i := range want {
		if hosts[i] != want[i] {
			t.Errorf("hosts[%d]: want %s, got %s", i, want[i], hosts[i])
		}
	}
}

// OPLOG-T05: OPLOG-010 — unknown or empty primary host keeps config order
func TestOplogMemberHosts_UnknownPrimary(t *testing.T) {
	rs := threeNodeRS()
	for _, primaryHost := range []string{"", "other:27017"} {
		hosts := OplogMemberHosts(rs, primaryHost)
		want := []string{"mongo1:27017", "mongo2:27017", "mongo3:27017"}
		if len(hosts) != len(want) {
			t.Fatalf("primary %q: expected %d hosts, got %v", primaryHost, len(want), hosts)
		}
		for i := range want {
			if hosts[i] != want[i] {
				t.Errorf("primary %q: hosts[%d]: want %s, got %s", primaryHost, i, want[i], hosts[i])
			}
		}
	}
}

// OPLOG-T06: OPLOG-012 — a failing member stops the fan-out with an error
// naming that member
func TestResizeOplogAcrossMembers_ErrorNamesMember(t *testing.T) {
	rs := threeNodeRS()
	var resized []string
	err := ResizeOplogAcrossMembers(context.Background(), rs, "mongo1:27017", func(_ context.Context, host string) error {
		if host == "mongo3:27017" {
			return fmt.Errorf("connection refused")
		}
		resized = append(resized, host)
		return nil
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mongo3:27017") {
		t.Errorf("error should name the failing member, got: %v", err)
	}
	if len(resized) != 1 || resized[0] != "mongo2:27017" {
		t.Errorf("expected only mongo2 resized before the failure, got %v", resized)
	}
}

// OPLOG-T07: OPLOG-013 — read reports the common size when members agree and
// OplogSizeMismatch when they disagree, so oversized members (e.g. after a
// partially failed shrink) surface as drift too
func TestOplogSizeAcrossMembers_MismatchSurfaces(t *testing.T) {
	rs := threeNodeRS()
	agreeing := map[string]float64{
		"mongo1:27017": 51200,
		"mongo2:27017": 51200,
		"mongo3:27017": 51200,
	}
	size, err := OplogSizeAcrossMembers(context.Background(), rs, func(_ context.Context, host string) (float64, error) {
		return agreeing[host], nil
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 51200 {
		t.Errorf("expected common size 51200, got %v", size)
	}

	// A failed shrink leaves some members large: min() would report 2048 and
	// mask them; the mismatch marker must be stored instead.
	diverged := map[string]float64{
		"mongo1:27017": 51200,
		"mongo2:27017": 2048,
		"mongo3:27017": 51200,
	}
	size, err = OplogSizeAcrossMembers(context.Background(), rs, func(_ context.Context, host string) (float64, error) {
		return diverged[host], nil
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != OplogSizeMismatch {
		t.Errorf("expected OplogSizeMismatch (%v), got %v", OplogSizeMismatch, size)
	}
}

// OPLOG-T08: OPLOG-008 — an unreadable member is skipped (reported via
// onSkip) rather than failing the read; the size comes from the rest
func TestOplogSizeAcrossMembers_SkipsUnreadableMember(t *testing.T) {
	rs := threeNodeRS()
	var skipped []string
	size, err := OplogSizeAcrossMembers(context.Background(), rs, func(_ context.Context, host string) (float64, error) {
		if host == "mongo2:27017" {
			return 0, fmt.Errorf("connection refused")
		}
		return 1024, nil
	}, func(host string, _ error) {
		skipped = append(skipped, host)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 1024 {
		t.Errorf("expected size 1024 from remaining members, got %v", size)
	}
	if len(skipped) != 1 || skipped[0] != "mongo2:27017" {
		t.Errorf("expected mongo2 skipped, got %v", skipped)
	}
}

// OPLOG-T09: A replica set with no data-bearing members is an error for both
// the read and the resize fan-out (previously the resize silently no-opped)
func TestOplogFanOut_NoDataBearingMembers(t *testing.T) {
	rs := ConfigMembers{
		{ID: 0, Host: "arbiter:27017", ArbiterOnly: boolPtr(true)},
	}
	_, err := OplogSizeAcrossMembers(context.Background(), rs, func(_ context.Context, _ string) (float64, error) {
		return 1024, nil
	}, nil)
	if err == nil {
		t.Fatal("expected read error for arbiter-only membership, got nil")
	}
	err = ResizeOplogAcrossMembers(context.Background(), rs, "", func(_ context.Context, _ string) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected resize error for arbiter-only membership, got nil")
	}
}

// OPLOG-T10: OPLOG-008 — when no member can be read, the error propagates
// and names a member
func TestOplogSizeAcrossMembers_AllUnreadable(t *testing.T) {
	rs := threeNodeRS()
	_, err := OplogSizeAcrossMembers(context.Background(), rs, func(_ context.Context, host string) (float64, error) {
		return 0, fmt.Errorf("connection refused")
	}, nil)
	if err == nil {
		t.Fatal("expected error when no member is readable, got nil")
	}
	if !strings.Contains(err.Error(), ":27017") {
		t.Errorf("error should name a member, got: %v", err)
	}
}

// OPLOG-T11: a member reporting a non-positive size is treated as unreadable
// so 0 can never reach state, where GetOk would treat oplog_size_mb as unset
// and silently disable oplog management
func TestOplogSizeAcrossMembers_ZeroSizeTreatedAsError(t *testing.T) {
	rs := threeNodeRS()
	sizes := map[string]float64{
		"mongo1:27017": 1024,
		"mongo2:27017": 0,
		"mongo3:27017": 1024,
	}
	size, err := OplogSizeAcrossMembers(context.Background(), rs, func(_ context.Context, host string) (float64, error) {
		return sizes[host], nil
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 1024 {
		t.Errorf("expected 1024 with the zero-reporting member skipped, got %v", size)
	}
}

// OPLOG-T14: OPLOG-018 — member reads run concurrently so refresh latency is
// bounded by the slowest member, not the sum across members. Every read
// blocks until all of them have started; a sequential implementation would
// hit the per-read timeout and fail.
func TestOplogSizeAcrossMembers_ReadsConcurrently(t *testing.T) {
	rs := threeNodeRS()
	started := make(chan struct{}, len(rs))
	release := make(chan struct{})
	go func() {
		for i := 0; i < len(rs); i++ {
			<-started
		}
		close(release)
	}()
	size, err := OplogSizeAcrossMembers(context.Background(), rs, func(_ context.Context, _ string) (float64, error) {
		started <- struct{}{}
		select {
		case <-release:
			return 1024, nil
		case <-time.After(5 * time.Second):
			return 0, fmt.Errorf("read did not overlap with the other members")
		}
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 1024 {
		t.Errorf("expected 1024, got %v", size)
	}
}

// OPLOG-T12: OPLOG-011/010 — an arbiter positioned before the primary keeps
// the skip and primary-last ordering consistent
func TestOplogMemberHosts_ArbiterBeforePrimary(t *testing.T) {
	rs := threeNodeRS()
	rs[0].ArbiterOnly = boolPtr(true)
	hosts := OplogMemberHosts(rs, "mongo3:27017")
	want := []string{"mongo2:27017", "mongo3:27017"}
	if len(hosts) != len(want) {
		t.Fatalf("expected %d hosts, got %d: %v", len(want), len(hosts), hosts)
	}
	for i := range want {
		if hosts[i] != want[i] {
			t.Errorf("hosts[%d]: want %s, got %s", i, want[i], hosts[i])
		}
	}
}

// OPLOG-T13: OPLOG-004/014 — applyOplogConfig touches no connection when
// oplog_size_mb is unset or unchanged (nil client/providerConf would panic
// if the gates were broken)
func TestApplyOplogConfig_GatesBeforeDialing(t *testing.T) {
	unset := schema.TestResourceDataRaw(t, resourceShardConfig().Schema, map[string]interface{}{
		"shard_name": "shard01",
	})
	if err := applyOplogConfig(context.Background(), nil, unset, threeNodeRS(), nil, false); err != nil {
		t.Fatalf("unexpected error with oplog_size_mb unset: %v", err)
	}

	// Rebuild from serialized prior state (TestResourceDataRaw alone diffs
	// against nil state and would register the attribute as changed), so
	// HasChange is false and the OPLOG-014 gate must skip the fan-out.
	seed := schema.TestResourceDataRaw(t, resourceShardConfig().Schema, map[string]interface{}{
		"shard_name":    "shard01",
		"oplog_size_mb": 51200.0,
	})
	seed.SetId("shard01")
	unchanged := resourceShardConfig().Data(seed.State())
	if unchanged.HasChange("oplog_size_mb") {
		t.Fatal("precondition failed: oplog_size_mb must register as unchanged")
	}
	if err := applyOplogConfig(context.Background(), nil, unchanged, threeNodeRS(), nil, false); err != nil {
		t.Fatalf("unexpected error with oplog_size_mb unchanged: %v", err)
	}
}
