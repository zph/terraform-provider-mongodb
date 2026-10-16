package mongodb

import (
	"context"
	"encoding/base64"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestResourceShardConfigParseId(t *testing.T) {
	r := &ResourceShardConfig{}
	cases := []struct {
		name      string
		id        string
		raw       bool // pass id as-is instead of base64-encoding it
		wantErr   bool
		wantShard string
		wantDB    string
	}{
		// TEST-007: valid base64 ID returns (shardName, database, nil)
		{name: "valid", id: "admin.shard01", wantShard: "shard01", wantDB: "admin"},
		// TEST-008: invalid inputs return errors
		{name: "invalid base64", id: "not-valid!@#", raw: true, wantErr: true},
		{name: "no separator", id: "nodotshere", wantErr: true},
		{name: "empty database", id: ".shardName", wantErr: true},
		{name: "empty shardName", id: "database.", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := tc.id
			if !tc.raw {
				id = base64.StdEncoding.EncodeToString([]byte(tc.id))
			}
			shardName, database, err := r.ParseId(id)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for case %q, got nil", tc.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil error, got: %v", err)
			}
			if shardName != tc.wantShard {
				t.Errorf("expected shardName %q, got %q", tc.wantShard, shardName)
			}
			if database != tc.wantDB {
				t.Errorf("expected database %q, got %q", tc.wantDB, database)
			}
		})
	}
}

func threeNodeRS() ConfigMembers {
	return ConfigMembers{
		{ID: 0, Host: "mongo1:27017", Priority: 1, Votes: intPtr(1), Hidden: boolPtr(false), ArbiterOnly: boolPtr(false), BuildIndexes: boolPtr(true)},
		{ID: 1, Host: "mongo2:27017", Priority: 1, Votes: intPtr(1), Hidden: boolPtr(false), ArbiterOnly: boolPtr(false), BuildIndexes: boolPtr(true)},
		{ID: 2, Host: "mongo3:27017", Priority: 1, Votes: intPtr(1), Hidden: boolPtr(false), ArbiterOnly: boolPtr(false), BuildIndexes: boolPtr(true)},
	}
}

func TestMergeMembers(t *testing.T) {
	cases := []struct {
		name      string
		setup     func(rs ConfigMembers) // mutates the starting set before the merge
		overrides []MemberOverride
		wantErr   bool
		want      func(rs ConfigMembers) // mutates a fresh threeNodeRS into the expected result
	}{
		{
			// SHARD-T01: SHARD-005 — one override changes only the matched member's priority
			name:      "single member priority",
			overrides: []MemberOverride{{Host: "mongo2:27017", Priority: 5, Votes: 1, BuildIndexes: true}},
			want:      func(rs ConfigMembers) { rs[1].Priority = 5 },
		},
		{
			// SHARD-T02: SHARD-005 — two overrides on different hosts
			name: "multiple member overrides",
			overrides: []MemberOverride{
				{Host: "mongo1:27017", Priority: 3, Votes: 1, BuildIndexes: true},
				{Host: "mongo3:27017", Priority: 0, Votes: 0, Hidden: true, BuildIndexes: true},
			},
			want: func(rs ConfigMembers) {
				rs[0].Priority = 3
				rs[2].Priority, rs[2].Votes, rs[2].Hidden = 0, intPtr(0), boolPtr(true)
			},
		},
		{
			// SHARD-T03: SHARD-004 — error for unknown host
			name:      "host not found",
			overrides: []MemberOverride{{Host: "unknown:27017", Priority: 1}},
			wantErr:   true,
		},
		{
			// SHARD-T04: SHARD-002 — empty overrides = no changes
			name: "empty overrides",
			want: func(ConfigMembers) {},
		},
		{
			// SHARD-T05: SHARD-005 — all fields applied
			name: "all fields",
			overrides: []MemberOverride{{
				Host: "mongo1:27017", Priority: 10, Votes: 1, Hidden: true, ArbiterOnly: false, BuildIndexes: false,
				Tags: map[string]string{"dc": "east", "rack": "r1"},
			}},
			want: func(rs ConfigMembers) {
				rs[0].Priority, rs[0].Hidden, rs[0].BuildIndexes = 10, boolPtr(true), boolPtr(false)
				rs[0].Tags = ReplsetTags{"dc": "east", "rack": "r1"}
			},
		},
		{
			// SHARD-T06: SHARD-005 — declaring a member sets all its fields, so an
			// omitted hidden comes through as the zero value, not the prior one
			name:  "partial fields",
			setup: func(rs ConfigMembers) { rs[0].Hidden = boolPtr(true) },
			overrides: []MemberOverride{{Host: "mongo1:27017", Priority: 5, Votes: 1, BuildIndexes: true,
				Tags: map[string]string{"zone": "a"}}},
			want: func(rs ConfigMembers) {
				rs[0].Priority, rs[0].Tags = 5, ReplsetTags{"zone": "a"}
			},
		},
		{
			// SHARD-T07: SHARD-009 — tags replaced entirely, not merged
			name:  "tags replace",
			setup: func(rs ConfigMembers) { rs[0].Tags = ReplsetTags{"old": "value", "keep": "this"} },
			overrides: []MemberOverride{{Host: "mongo1:27017", Priority: 1, Votes: 1, BuildIndexes: true,
				Tags: map[string]string{"dc": "east"}}},
			want: func(rs ConfigMembers) { rs[0].Tags = ReplsetTags{"dc": "east"} },
		},
		{
			// SHARD-T08: SHARD-006 — unlisted members byte-for-byte identical
			name: "unlisted preserved",
			setup: func(rs ConfigMembers) {
				rs[1].Tags, rs[1].Priority = ReplsetTags{"existing": "tag"}, 7.0
			},
			overrides: []MemberOverride{{Host: "mongo1:27017", Priority: 2, Votes: 1, BuildIndexes: true}},
			want: func(rs ConfigMembers) {
				rs[0].Priority = 2
				rs[1].Tags, rs[1].Priority = ReplsetTags{"existing": "tag"}, 7.0
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := threeNodeRS()
			if tc.setup != nil {
				tc.setup(rs)
			}
			result, err := MergeMembers(rs, tc.overrides)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := threeNodeRS()
			tc.want(want)
			if !reflect.DeepEqual(result, want) {
				t.Errorf("merged members:\n got %+v\nwant %+v", result, want)
			}
		})
	}
}

// stateMember is the state map RSConfigMembersToState emits for a member with
// nil pointer fields; extra overrides individual keys.
func stateMember(host string, priority float64, extra map[string]interface{}) map[string]interface{} {
	m := map[string]interface{}{
		"host": host, "priority": priority, "votes": 0, "hidden": false,
		"arbiter_only": false, "build_indexes": false, "tags": map[string]interface{}{},
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestRSConfigMembersToState(t *testing.T) {
	threePlain := ConfigMembers{
		{ID: 0, Host: "mongo1:27017", Priority: 1},
		{ID: 1, Host: "mongo2:27017", Priority: 2},
		{ID: 2, Host: "mongo3:27017", Priority: 3},
	}
	cases := []struct {
		name    string
		members ConfigMembers
		managed []string
		want    []interface{}
	}{
		{
			// SHARD-T09: SHARD-007 — full conversion with all fields
			name: "all fields",
			members: ConfigMembers{{
				ID: 0, Host: "mongo1:27017", Priority: 5,
				Votes: intPtr(1), Hidden: boolPtr(true), ArbiterOnly: boolPtr(false), BuildIndexes: boolPtr(true),
				Tags: ReplsetTags{"dc": "east"},
			}},
			managed: []string{"mongo1:27017"},
			want: []interface{}{stateMember("mongo1:27017", 5, map[string]interface{}{
				"votes": 1, "hidden": true, "build_indexes": true, "tags": map[string]interface{}{"dc": "east"},
			})},
		},
		{
			// SHARD-T10: SHARD-008 — only managed hosts returned
			name:    "managed filter",
			members: threePlain,
			managed: []string{"mongo2:27017"},
			want:    []interface{}{stateMember("mongo2:27017", 2, nil)},
		},
		{
			// SHARD-T11: SHARD-002 — nil managed hosts returns nil
			name:    "nil managed",
			members: ConfigMembers{{ID: 0, Host: "mongo1:27017"}},
			want:    nil,
		},
		{
			// SHARD-T12: nil pointer fields don't panic and deref to zero values
			name:    "nil pointers",
			members: ConfigMembers{{ID: 0, Host: "mongo1:27017", Priority: 1}},
			managed: []string{"mongo1:27017"},
			want:    []interface{}{stateMember("mongo1:27017", 1, nil)},
		},
		{
			// SHARD-T35: SHARD-025 — state follows block order and skips hosts not in the set
			name:    "block order",
			members: threePlain,
			managed: []string{"mongo3:27017", "mongo1:27017", "gone:27017"},
			want:    []interface{}{stateMember("mongo3:27017", 3, nil), stateMember("mongo1:27017", 1, nil)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RSConfigMembersToState(tc.members, tc.managed); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("RSConfigMembersToState:\n got %v\nwant %v", got, tc.want)
			}
		})
	}
}

// SHARD-T35: SHARD-025 — managed hosts follow block order, deduped, blanks dropped
func TestManagedHostsFromState_BlockOrder(t *testing.T) {
	data := schema.TestResourceDataRaw(t, resourceShardConfig().Schema, map[string]interface{}{
		"shard_name": "rs0",
		"member": []interface{}{
			map[string]interface{}{"host": "b:1"},
			map[string]interface{}{"host": "a:1"},
			map[string]interface{}{"host": "b:1"},
			map[string]interface{}{"host": ""},
		},
	})
	got := managedHostsFromState(data)
	if len(got) != 2 || got[0] != "b:1" || got[1] != "a:1" {
		t.Errorf("managedHostsFromState: want [b:1 a:1] in block order without duplicates, got %v", got)
	}
}

func TestShardConfigSchema(t *testing.T) {
	cases := []struct {
		name  string
		check func(t *testing.T, s map[string]*schema.Schema)
	}{
		{
			// SHARD-T13: SHARD-001 — schema has correct member sub-fields
			name: "member block",
			check: func(t *testing.T, s map[string]*schema.Schema) {
				memberSchema, ok := s["member"]
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
				for _, f := range []string{"host", "tags", "priority", "votes", "hidden", "arbiter_only", "build_indexes"} {
					if _, exists := elem.Schema[f]; !exists {
						t.Errorf("member sub-schema missing field %q", f)
					}
				}
				if !elem.Schema["host"].Required {
					t.Error("member.host should be Required")
				}
			},
		},
		{
			// SHARD-T26: SHARD-020 — priority and votes default to 1 in the schema, so the
			// documented defaults are sent explicitly instead of relying on the server.
			name: "member defaults",
			check: func(t *testing.T, s map[string]*schema.Schema) {
				elem := s["member"].Elem.(*schema.Resource)
				priority := elem.Schema["priority"]
				if priority.Type != schema.TypeFloat {
					t.Errorf("priority type: want TypeFloat, got %v", priority.Type)
				}
				if priority.Default != 1.0 {
					t.Errorf("priority default: want 1.0, got %v", priority.Default)
				}
				votes := elem.Schema["votes"]
				if votes.Type != schema.TypeInt {
					t.Errorf("votes type: want TypeInt, got %v", votes.Type)
				}
				if votes.Default != 1 {
					t.Errorf("votes default: want 1, got %v", votes.Default)
				}
			},
		},
		{
			// OPLOG-T01: OPLOG-001/002 — oplog_size_mb is Optional TypeFloat with validation
			name: "oplog_size_mb",
			check: func(t *testing.T, s map[string]*schema.Schema) {
				field, ok := s["oplog_size_mb"]
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
			},
		},
		{
			// CATCHUP-T01: CATCHUP-001 — catch_up_timeout_millis is Optional TypeInt Default -1
			name: "catch_up_timeout_millis",
			check: func(t *testing.T, s map[string]*schema.Schema) {
				field, ok := s["catch_up_timeout_millis"]
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
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.check(t, resourceShardConfig().Schema) })
	}
}

// OPLOG-T02: BytesPerMB constant is correct
func TestBytesPerMB(t *testing.T) {
	if BytesPerMB != 1048576 {
		t.Errorf("BytesPerMB should be 1048576, got %d", BytesPerMB)
	}
}

func TestOplogMemberHosts(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(rs ConfigMembers)
		primary string
		want    []string
	}{
		{
			// OPLOG-T04: OPLOG-011 — arbiters carry no oplog and are skipped
			name:    "skips arbiters",
			setup:   func(rs ConfigMembers) { rs[2].ArbiterOnly = boolPtr(true) },
			primary: "mongo1:27017",
			want:    []string{"mongo2:27017", "mongo1:27017"},
		},
		// OPLOG-T05: OPLOG-010 — unknown or empty primary host keeps config order
		{
			name: "empty primary keeps config order",
			want: []string{"mongo1:27017", "mongo2:27017", "mongo3:27017"},
		},
		{
			name:    "unknown primary keeps config order",
			primary: "other:27017",
			want:    []string{"mongo1:27017", "mongo2:27017", "mongo3:27017"},
		},
		{
			// OPLOG-T12: OPLOG-011/010 — an arbiter positioned before the primary keeps
			// the skip and primary-last ordering consistent
			name:    "arbiter before primary",
			setup:   func(rs ConfigMembers) { rs[0].ArbiterOnly = boolPtr(true) },
			primary: "mongo3:27017",
			want:    []string{"mongo2:27017", "mongo3:27017"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := threeNodeRS()
			if tc.setup != nil {
				tc.setup(rs)
			}
			if hosts := OplogMemberHosts(rs, tc.primary); !slices.Equal(hosts, tc.want) {
				t.Errorf("primary %q: want %v, got %v", tc.primary, tc.want, hosts)
			}
		})
	}
}

func TestResizeOplogAcrossMembers(t *testing.T) {
	arbiterOnly := ConfigMembers{{ID: 0, Host: "arbiter:27017", ArbiterOnly: boolPtr(true)}}
	cases := []struct {
		name    string
		rs      ConfigMembers
		primary string
		// resize may append to resized to record the fan-out order
		resize      func(host string, resized *[]string) (bool, error)
		wantResized []string
		wantErr     bool
		wantErrName string // substring the error must contain
	}{
		{
			// OPLOG-T03: OPLOG-009/010 — resize reaches every data-bearing member,
			// secondaries first and the primary last. Regression: previously only the
			// single member the provider was connected to was resized.
			name:    "all members primary last",
			primary: "mongo1:27017",
			resize: func(host string, resized *[]string) (bool, error) {
				*resized = append(*resized, host)
				return true, nil
			},
			wantResized: []string{"mongo2:27017", "mongo3:27017", "mongo1:27017"},
		},
		{
			// OPLOG-T14: OPLOG-019 — a fan-out in which every member was skipped (no
			// member PRIMARY or SECONDARY) is an error. Regression: it returned nil, so
			// an apply that resized nothing was indistinguishable from a successful one.
			name:        "all members skipped",
			resize:      func(string, *[]string) (bool, error) { return false, nil },
			wantErr:     true,
			wantErrName: "mongo1:27017",
		},
		{
			// OPLOG-T14: a single resized member is still a resize
			name:   "one member resized",
			resize: func(host string, _ *[]string) (bool, error) { return host == "mongo2:27017", nil },
		},
		{
			// OPLOG-T06: OPLOG-012 — a failing member stops the fan-out with an error
			// naming that member
			name:    "error names member",
			primary: "mongo1:27017",
			resize: func(host string, resized *[]string) (bool, error) {
				if host == "mongo3:27017" {
					return false, fmt.Errorf("connection refused")
				}
				*resized = append(*resized, host)
				return true, nil
			},
			wantResized: []string{"mongo2:27017"},
			wantErr:     true,
			wantErrName: "mongo3:27017",
		},
		{
			// OPLOG-T09: a replica set with no data-bearing members is an error
			// (previously the resize silently no-opped)
			name:    "no data-bearing members",
			rs:      arbiterOnly,
			resize:  func(string, *[]string) (bool, error) { return true, nil },
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := tc.rs
			if rs == nil {
				rs = threeNodeRS()
			}
			var resized []string
			err := ResizeOplogAcrossMembers(context.Background(), rs, tc.primary, func(_ context.Context, host string) (bool, error) {
				return tc.resize(host, &resized)
			})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantErrName != "" && !strings.Contains(err.Error(), tc.wantErrName) {
					t.Errorf("error should name %s, got: %v", tc.wantErrName, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantResized != nil && !slices.Equal(resized, tc.wantResized) {
				t.Errorf("resized members: want %v, got %v", tc.wantResized, resized)
			}
		})
	}
}

func TestOplogSizeAcrossMembers(t *testing.T) {
	bySize := func(sizes map[string]float64) func(context.Context, string) (float64, error) {
		return func(_ context.Context, host string) (float64, error) { return sizes[host], nil }
	}
	cases := []struct {
		name        string
		rs          ConfigMembers
		read        func(t *testing.T) func(context.Context, string) (float64, error)
		wantSize    float64
		wantErr     bool
		wantErrName string // substring the error must contain
		wantSkipped []string
	}{
		{
			// OPLOG-T07: OPLOG-013 — read reports the common size when members agree
			name: "agreeing members",
			read: func(*testing.T) func(context.Context, string) (float64, error) {
				return bySize(map[string]float64{"mongo1:27017": 51200, "mongo2:27017": 51200, "mongo3:27017": 51200})
			},
			wantSize: 51200,
		},
		{
			// OPLOG-T07: OPLOG-013 — OplogSizeMismatch when members disagree, so oversized
			// members (e.g. after a partially failed shrink) surface as drift too.
			// min() would report 2048 and mask them; the mismatch marker must be stored instead.
			name: "mismatch surfaces",
			read: func(*testing.T) func(context.Context, string) (float64, error) {
				return bySize(map[string]float64{"mongo1:27017": 51200, "mongo2:27017": 2048, "mongo3:27017": 51200})
			},
			wantSize: OplogSizeMismatch,
		},
		{
			// OPLOG-T08: OPLOG-008 — an unreadable member is skipped (reported via
			// onSkip) rather than failing the read; the size comes from the rest
			name: "skips unreadable member",
			read: func(*testing.T) func(context.Context, string) (float64, error) {
				return func(_ context.Context, host string) (float64, error) {
					if host == "mongo2:27017" {
						return 0, fmt.Errorf("connection refused")
					}
					return 1024, nil
				}
			},
			wantSize:    1024,
			wantSkipped: []string{"mongo2:27017"},
		},
		{
			// OPLOG-T09: a replica set with no data-bearing members is an error
			name: "no data-bearing members",
			rs:   ConfigMembers{{ID: 0, Host: "arbiter:27017", ArbiterOnly: boolPtr(true)}},
			read: func(*testing.T) func(context.Context, string) (float64, error) {
				return func(context.Context, string) (float64, error) { return 1024, nil }
			},
			wantErr: true,
		},
		{
			// OPLOG-T10: OPLOG-008 — when no member can be read, the error propagates
			// and names a member
			name: "all unreadable",
			read: func(*testing.T) func(context.Context, string) (float64, error) {
				return func(context.Context, string) (float64, error) { return 0, fmt.Errorf("connection refused") }
			},
			wantErr:     true,
			wantErrName: ":27017",
		},
		{
			// OPLOG-T11: a member reporting a non-positive size is treated as unreadable
			// so 0 can never reach state, where GetOk would treat oplog_size_mb as unset
			// and silently disable oplog management
			name: "zero size treated as error",
			read: func(*testing.T) func(context.Context, string) (float64, error) {
				return bySize(map[string]float64{"mongo1:27017": 1024, "mongo2:27017": 0, "mongo3:27017": 1024})
			},
			wantSize: 1024,
		},
		{
			// OPLOG-T14: OPLOG-018 — member reads run concurrently so refresh latency is
			// bounded by the slowest member, not the sum across members. Every read
			// blocks until all of them have started; a sequential implementation would
			// hit the per-read timeout and fail.
			name: "reads concurrently",
			read: func(*testing.T) func(context.Context, string) (float64, error) {
				n := len(threeNodeRS())
				started := make(chan struct{}, n)
				release := make(chan struct{})
				go func() {
					for i := 0; i < n; i++ {
						<-started
					}
					close(release)
				}()
				return func(context.Context, string) (float64, error) {
					started <- struct{}{}
					select {
					case <-release:
						return 1024, nil
					case <-time.After(5 * time.Second):
						return 0, fmt.Errorf("read did not overlap with the other members")
					}
				}
			},
			wantSize: 1024,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := tc.rs
			if rs == nil {
				rs = threeNodeRS()
			}
			var skipped []string
			var onSkip func(host string, _ error)
			if tc.wantSkipped != nil {
				onSkip = func(host string, _ error) { skipped = append(skipped, host) }
			}
			size, err := OplogSizeAcrossMembers(context.Background(), rs, tc.read(t), onSkip)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantErrName != "" && !strings.Contains(err.Error(), tc.wantErrName) {
					t.Errorf("error should name a member, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if size != tc.wantSize {
				t.Errorf("expected size %v, got %v", tc.wantSize, size)
			}
			if tc.wantSkipped != nil && !slices.Equal(skipped, tc.wantSkipped) {
				t.Errorf("expected skipped %v, got %v", tc.wantSkipped, skipped)
			}
		})
	}
}

// OPLOG-T13: OPLOG-004/014 — applyOplogConfig touches no connection when
// oplog_size_mb is unset or unchanged (nil client/providerConf would panic
// if the gates were broken)
func TestApplyOplogConfig_GatesBeforeDialing(t *testing.T) {
	cases := []struct {
		name string
		data func(t *testing.T) *schema.ResourceData
	}{
		{
			name: "unset",
			data: func(t *testing.T) *schema.ResourceData {
				return schema.TestResourceDataRaw(t, resourceShardConfig().Schema, map[string]interface{}{
					"shard_name": "shard01",
				})
			},
		},
		{
			// Rebuild from serialized prior state (TestResourceDataRaw alone diffs
			// against nil state and would register the attribute as changed), so
			// HasChange is false and the OPLOG-014 gate must skip the fan-out.
			name: "unchanged",
			data: func(t *testing.T) *schema.ResourceData {
				seed := schema.TestResourceDataRaw(t, resourceShardConfig().Schema, map[string]interface{}{
					"shard_name":    "shard01",
					"oplog_size_mb": 51200.0,
				})
				seed.SetId("shard01")
				unchanged := resourceShardConfig().Data(seed.State())
				if unchanged.HasChange("oplog_size_mb") {
					t.Fatal("precondition failed: oplog_size_mb must register as unchanged")
				}
				return unchanged
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			attempted, err := applyOplogConfig(context.Background(), nil, tc.data(t), threeNodeRS(), nil, false)
			if err != nil {
				t.Fatalf("unexpected error with oplog_size_mb %s: %v", tc.name, err)
			}
			if attempted {
				t.Errorf("fan-out must not run with oplog_size_mb %s", tc.name)
			}
		})
	}
}

// SHARD-T36: SHARD-026 — duplicate hosts are rejected before anything is sent
func TestValidateMemberOverrides(t *testing.T) {
	cases := []struct {
		name      string
		overrides []MemberOverride
		wantInMsg []string // non-empty means an error is expected
	}{
		{"duplicate host", []MemberOverride{{Host: "a:1"}, {Host: "b:1"}, {Host: "a:1"}}, []string{"index 2", `"a:1"`, "index 0"}},
		{"distinct hosts", []MemberOverride{{Host: "a:1"}, {Host: "b:1"}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := validateMemberOverrides(tc.overrides)
			if tc.wantInMsg == nil {
				if diags.HasError() {
					t.Errorf("distinct hosts should pass, got %v", diags)
				}
				return
			}
			if !diags.HasError() {
				t.Fatal("want an error for the duplicate host")
			}
			for _, want := range tc.wantInMsg {
				if !strings.Contains(diags[0].Summary, want) {
					t.Errorf("error should mention %s, got: %s", want, diags[0].Summary)
				}
			}
		})
	}
}

// SHARD-T37: SHARD-024 — an arbiter block sends priority 0 whatever the schema default says
func TestExtractMemberOverrides_ArbiterPriorityZero(t *testing.T) {
	data := schema.TestResourceDataRaw(t, resourceShardConfig().Schema, map[string]interface{}{
		"shard_name": "rs0",
		"member": []interface{}{
			map[string]interface{}{"host": "data:1", "priority": 2.0},
			map[string]interface{}{"host": "arb:1", "arbiter_only": true},
		},
	})
	overrides, ok := extractMemberOverrides(data)
	if !ok || len(overrides) != 2 {
		t.Fatalf("want 2 overrides, got %v (ok=%v)", overrides, ok)
	}
	if overrides[0].Priority != 2 {
		t.Errorf("data member priority: want 2, got %v", overrides[0].Priority)
	}
	if !overrides[1].ArbiterOnly || overrides[1].Priority != 0 || overrides[1].Votes != 1 {
		t.Errorf("arbiter: want priority 0 with votes 1, got %+v", overrides[1])
	}
}

// SHARD-T38: SHARD-024 — priority diffs are suppressed on arbiter blocks only
func TestSuppressArbiterPriority(t *testing.T) {
	data := schema.TestResourceDataRaw(t, resourceShardConfig().Schema, map[string]interface{}{
		"shard_name": "rs0",
		"member": []interface{}{
			map[string]interface{}{"host": "data:1"},
			map[string]interface{}{"host": "arb:1", "arbiter_only": true},
		},
	})
	if suppressArbiterPriority("member.0.priority", "0", "1", data) {
		t.Error("a data-bearing member's priority diff must not be suppressed")
	}
	if !suppressArbiterPriority("member.1.priority", "0", "1", data) {
		t.Error("an arbiter's priority diff must be suppressed")
	}
}

// shardConfigDiffData plans config against prior state and returns the
// ResourceData an Update would see, plus the raw diff.
func shardConfigDiffData(t *testing.T, prior map[string]interface{}, config map[string]interface{}, meta interface{}) (*schema.ResourceData, *terraform.InstanceDiff) {
	t.Helper()
	res := resourceShardConfig()
	state := schema.TestResourceDataRaw(t, res.Schema, prior)
	state.SetId("rs0")
	diff, err := res.Diff(context.Background(), state.State(), terraform.NewResourceConfigRaw(config), meta)
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	data, err := schema.InternalMap(res.Schema).Data(state.State(), diff)
	if err != nil {
		t.Fatalf("Data returned error: %v", err)
	}
	return data, diff
}

func shardConfigPrior() map[string]interface{} {
	return map[string]interface{}{
		"shard_name": "rs0",
		"member": []interface{}{
			map[string]interface{}{"host": "a:27017"},
		},
		"init_timeout_secs": 60,
	}
}

// SHARD-031: only client-side attributes may be skipped by Update; anything
// the server holds still reconfigures, with or without command preview.
func TestOnlyClientSideChanges(t *testing.T) {
	preview := &MongoDatabaseConfiguration{
		CommandPreview:  true,
		FeaturesEnabled: map[string]bool{"mongodb_shard_config": true},
	}
	cases := []struct {
		name       string
		meta       interface{}
		change     func(config map[string]interface{})
		clientSide bool
		check      func(t *testing.T, data *schema.ResourceData, diff *terraform.InstanceDiff)
	}{
		{
			// a plan that only records init_timeout_secs, as the first plan after an
			// import without the seed does, is not a reconfig
			name:       "timeout only",
			change:     func(c map[string]interface{}) { c["init_timeout_secs"] = 180 },
			clientSide: true,
			check: func(t *testing.T, data *schema.ResourceData, diff *terraform.InstanceDiff) {
				if diff == nil || len(diff.Attributes) == 0 {
					t.Fatal("expected a diff on init_timeout_secs")
				}
				if !data.HasChange("init_timeout_secs") {
					t.Error("the planned timeout must still be visible to Update")
				}
			},
		},
		{
			name:       "host_override only",
			change:     func(c map[string]interface{}) { c["host_override"] = "override:27017" },
			clientSide: true,
		},
		{
			name: "chaining_allowed is a server setting",
			change: func(c map[string]interface{}) {
				c["init_timeout_secs"] = 180
				c["chaining_allowed"] = false
			},
		},
		{
			name: "member add",
			change: func(c map[string]interface{}) {
				c["member"] = []interface{}{
					map[string]interface{}{"host": "a:27017"},
					map[string]interface{}{"host": "b:27017"},
				}
			},
		},
		{
			name:   "oplog_size_mb is applied to the members",
			change: func(c map[string]interface{}) { c["oplog_size_mb"] = 2048.0 },
		},
		{
			// with command preview on, a client-side-only plan shows no command and
			// planned_commands does not itself turn into a change
			name:       "preview client-side only is empty",
			meta:       preview,
			change:     func(c map[string]interface{}) { c["init_timeout_secs"] = 180 },
			clientSide: true,
			check: func(t *testing.T, _ *schema.ResourceData, diff *terraform.InstanceDiff) {
				if attr, ok := diff.Attributes["planned_commands"]; ok && attr.New != "" {
					t.Errorf("planned_commands = %q, want none for a client-side-only change", attr.New)
				}
			},
		},
		{
			name: "preview settings change shows replSetReconfig",
			meta: preview,
			change: func(c map[string]interface{}) {
				c["init_timeout_secs"] = 180
				c["chaining_allowed"] = false
			},
			check: func(t *testing.T, _ *schema.ResourceData, diff *terraform.InstanceDiff) {
				attr, ok := diff.Attributes["planned_commands"]
				if !ok || !strings.Contains(attr.New, "replSetReconfig") {
					t.Errorf("a settings change must still preview a replSetReconfig, got %v", diff.Attributes["planned_commands"])
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := shardConfigPrior()
			tc.change(config)
			data, diff := shardConfigDiffData(t, shardConfigPrior(), config, tc.meta)
			if got := onlyClientSideChanges(data); got != tc.clientSide {
				t.Errorf("onlyClientSideChanges = %v, want %v, diff %v", got, tc.clientSide, diff.Attributes)
			}
			if tc.check != nil {
				tc.check(t, data, diff)
			}
		})
	}
}
