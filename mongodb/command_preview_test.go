package mongodb

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// --- Infrastructure tests ---

func TestCommandPreviewEnabled(t *testing.T) {
	cases := []struct {
		name string
		meta interface{}
		want bool
	}{
		{"PREVIEW-T01 PREVIEW-010 nil meta", nil, false},
		{"PREVIEW-T02 PREVIEW-002 disabled", &MongoDatabaseConfiguration{CommandPreview: false}, false},
		{"PREVIEW-T03 PREVIEW-003 enabled", &MongoDatabaseConfiguration{CommandPreview: true}, true},
		{"PREVIEW-T04 PREVIEW-010 wrong type", "not a config", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := commandPreviewEnabled(tc.meta); got != tc.want {
				t.Errorf("commandPreviewEnabled(%v) = %v, want %v", tc.meta, got, tc.want)
			}
		})
	}
}

func TestCommandPreviewSchema(t *testing.T) {
	// PREVIEW-T05: PREVIEW-005 — commandPreviewSchema returns Computed TypeString
	t.Run("planned_commands helper", func(t *testing.T) {
		s := commandPreviewSchema()
		if s.Type != schema.TypeString {
			t.Errorf("expected TypeString, got %v", s.Type)
		}
		if !s.Computed {
			t.Error("planned_commands should be Computed")
		}
	})

	// PREVIEW-T06: PREVIEW-001 — Provider schema includes command_preview
	t.Run("provider command_preview", func(t *testing.T) {
		p := Provider()
		field, ok := p.Schema["command_preview"]
		if !ok {
			t.Fatal("provider schema missing command_preview field")
		}
		if field.Type != schema.TypeBool {
			t.Errorf("command_preview type: want TypeBool, got %v", field.Type)
		}
		if field.Required {
			t.Error("command_preview should not be Required")
		}
	})

	// PREVIEW-T07: PREVIEW-005 — All resources have planned_commands field
	for _, reg := range AllResources() {
		t.Run(reg.Name+" planned_commands", func(t *testing.T) {
			field, ok := reg.Factory().Schema["planned_commands"]
			if !ok {
				t.Fatal("missing planned_commands field")
			}
			if !field.Computed {
				t.Error("planned_commands should be Computed")
			}
			if field.Type != schema.TypeString {
				t.Errorf("planned_commands type: want TypeString, got %v", field.Type)
			}
		})
	}
}

// --- Per-resource builder tests ---

func TestResourcePreviewBuilders(t *testing.T) {
	roles := []previewRole{{Role: "readWrite", DB: "mydb"}}
	cases := []struct {
		name     string
		got      string
		want     string   // exact match when set
		contains []string // substrings that must appear
		absent   []string // substrings that must not appear
	}{
		{
			name: "PREVIEW-T10 PREVIEW-013 server_parameter create",
			got:  buildServerParameterPreview("notablescan", "true", true),
			want: `db.adminCommand({setParameter: 1, "notablescan": "true"})`,
		},
		{
			name: "PREVIEW-T11 PREVIEW-012 profiler create",
			got:  buildProfilerPreview("mydb", 1, 100, 1),
			want: `db.getSiblingDB("mydb").runCommand({profile: 1, slowms: 100, ratelimit: 1})`,
		},
		{
			name: "PREVIEW-T12 PREVIEW-020 fcv",
			got:  buildFCVPreview("7.0"),
			want: `db.adminCommand({setFeatureCompatibilityVersion: "7.0"})`,
		},
		{
			name:     "PREVIEW-T13 PREVIEW-016 db_user create redacts password",
			got:      buildDbUserPreview("admin", "appuser", roles, true, true),
			contains: []string{"createUser", "[REDACTED]", "readWrite"},
		},
		{
			name:     "PREVIEW-T14 PREVIEW-017 DANGER-021 db_user update with password change",
			got:      buildDbUserPreview("admin", "appuser", roles, false, true),
			contains: []string{"updateUser", "[REDACTED]"},
		},
		{
			name:   "PREVIEW-T14 DANGER-021 db_user role-only update omits pwd",
			got:    buildDbUserPreview("admin", "appuser", roles, false, false),
			absent: []string{"pwd"},
		},
		{
			name: "PREVIEW-T15 PREVIEW-018 db_role create",
			got: buildDbRolePreview("admin", "customRole",
				[]previewPrivilege{{DB: "mydb", Collection: "users", Actions: []string{"find", "insert"}}},
				[]previewRole{{Role: "read", DB: "mydb"}}, true),
			contains: []string{"createRole", "find"},
		},
		{
			name: "PREVIEW-T15 PREVIEW-019 db_role update",
			got: buildDbRolePreview("admin", "customRole",
				[]previewPrivilege{{DB: "mydb", Collection: "", Actions: []string{"find"}}}, nil, false),
			contains: []string{"updateRole"},
		},
		{
			name: "PREVIEW-T16 PREVIEW-015 shard create",
			got:  buildShardPreview("shard01", []string{"host1:27018", "host2:27018"}),
			want: `db.adminCommand({addShard: "shard01/host1:27018,host2:27018"})`,
		},
		{
			name:     "PREVIEW-T17 PREVIEW-024 original_user create",
			got:      buildOriginalUserPreview("admin", "admin", []previewRole{{Role: "root", DB: "admin"}}),
			contains: []string{"createUser", "[REDACTED]"},
		},
		{
			name:     "PREVIEW-T18 PREVIEW-014 collection_balancing shows both FCV paths",
			got:      buildCollectionBalancingPreview("mydb.users", true, 0),
			contains: []string{"configureCollectionBalancing", "config"},
		},
		{
			name:     "PREVIEW-T18 collection_balancing with chunk size",
			got:      buildCollectionBalancingPreview("mydb.users", true, 128),
			contains: []string{"chunkSize: 128"},
		},
		{
			name: "PREVIEW-T19 PREVIEW-021 balancer_config multi-command",
			got: buildBalancerConfigPreview(balancerPreviewInput{
				Enabled:     true,
				WindowStart: "06:00",
				WindowStop:  "09:00",
				ChunkSizeMB: 128,
			}),
			contains: []string{"balancerStart", "06:00", "128"},
		},
		{
			name:     "PREVIEW-T19 balancer_config disabled",
			got:      buildBalancerConfigPreview(balancerPreviewInput{Enabled: false}),
			contains: []string{"balancerStop"},
		},
		{
			name: "PREVIEW-T21 ZONE-013 shard zone",
			got:  buildShardZonePreview("shard01", "US-East"),
			want: `db.adminCommand({addShardToZone: "shard01", zone: "US-East"})`,
		},
		{
			name:     "PREVIEW-T22 ZONE-030 zone key range",
			got:      buildZoneKeyRangePreview("app_db.orders", "US-East", `{"region": "east"}`, `{"region": "west"}`),
			contains: []string{"updateZoneKeyRange", `"app_db.orders"`, `"US-East"`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.want != "" && tc.got != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", tc.got, tc.want)
			}
			for _, s := range tc.contains {
				if !strings.Contains(tc.got, s) {
					t.Errorf("should contain %q, got: %s", s, tc.got)
				}
			}
			for _, s := range tc.absent {
				if strings.Contains(tc.got, s) {
					t.Errorf("must not contain %q, got: %s", s, tc.got)
				}
			}
		})
	}
}

// PREVIEW-T20: PREVIEW-022, PREVIEW-023 — shard_config preview
func TestShardConfigPreviewBuild(t *testing.T) {
	cases := []struct {
		name    string
		create  bool
		changes []previewMemberChange
		check   func(t *testing.T, got string)
	}{
		{
			name:   "create",
			create: true,
			changes: []previewMemberChange{
				{previewMember: previewMember{Host: "mongo2:27017", Votes: 1, Priority: 1}, Add: true, Promote: true},
				{previewMember: previewMember{Host: "mongo3:27017"}, Add: true},
			},
			check: func(t *testing.T, got string) {
				if !strings.Contains(got, "replSetInitiate") {
					t.Errorf("create should show replSetInitiate, got: %s", got)
				}
				// settings, add mongo2, promote mongo2, add mongo3
				if n := strings.Count(got, "replSetReconfig"); n != 4 {
					t.Errorf("create with a voting and a non-voting add should show four replSetReconfig lines, got %d: %s", n, got)
				}
				for _, host := range []string{"mongo2:27017", "mongo3:27017"} {
					if !strings.Contains(got, host) {
						t.Errorf("create should name added host %s, got: %s", host, got)
					}
				}
			},
		},
		{
			name: "update without member changes",
			check: func(t *testing.T, got string) {
				if strings.Contains(got, "replSetInitiate") {
					t.Errorf("update should not show replSetInitiate, got: %s", got)
				}
				if n := strings.Count(got, "replSetReconfig"); n != 1 {
					t.Errorf("update without member changes should show one replSetReconfig, got %d: %s", n, got)
				}
			},
		},
		{
			// PREVIEW-T22: PREVIEW-023, SHARD-022 — a voting add is two lines: the staged add, then the promotion
			name: "PREVIEW-T22 SHARD-022 update adds voter",
			changes: []previewMemberChange{
				{previewMember: previewMember{Host: "mongo3:27017", Votes: 1, Priority: 2}, Add: true, Promote: true},
			},
			check: func(t *testing.T, got string) {
				lines := strings.Split(got, "\n")
				if len(lines) != 3 {
					t.Fatalf("want settings, add, promote = 3 lines, got %d: %s", len(lines), got)
				}
				if !strings.Contains(lines[1], `host: "mongo3:27017", votes: 0, priority: 0`) || !strings.Contains(lines[1], "<max+1>") {
					t.Errorf("add line should stage the member as a non-voter with _id max+1, got: %s", lines[1])
				}
				if !strings.Contains(lines[2], `host: "mongo3:27017", votes: 1, priority: 2`) || !strings.Contains(lines[2], "SECONDARY") {
					t.Errorf("promotion line should carry the configured votes and priority, got: %s", lines[2])
				}
			},
		},
		{
			// PREVIEW-T23: arbiters and non-voters are one line each; a promotion of a live member is one line
			name: "PREVIEW-T23 arbiter, non-voter and live promotion",
			changes: []previewMemberChange{
				{previewMember: previewMember{Host: "arb:27017", Votes: 1, ArbiterOnly: true}, Add: true},
				{previewMember: previewMember{Host: "hidden:27017"}, Add: true},
				{previewMember: previewMember{Host: "old:27017", Votes: 1, Priority: 1}, Promote: true},
			},
			check: func(t *testing.T, got string) {
				lines := strings.Split(got, "\n")
				if len(lines) != 4 {
					t.Fatalf("want settings + 3 lines, got %d: %s", len(lines), got)
				}
				if !strings.Contains(lines[1], `host: "arb:27017", arbiterOnly: true`) || strings.Contains(lines[1], "votes: 0") {
					t.Errorf("arbiter line should add in final form, got: %s", lines[1])
				}
				if !strings.Contains(lines[2], `host: "hidden:27017", votes: 0, priority: 0`) || strings.Contains(lines[2], "SECONDARY") {
					t.Errorf("non-voter line should be a single staged add, got: %s", lines[2])
				}
				if strings.Contains(lines[3], "<max+1>") || !strings.Contains(lines[3], `host: "old:27017", votes: 1, priority: 1`) {
					t.Errorf("promotion of a live member should not look like an add, got: %s", lines[3])
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, buildShardConfigPreview("shard01", tc.create, tc.changes))
		})
	}
}

// PREVIEW-T24: previewMemberChanges — adds and promotions in block order; known, empty and duplicate hosts skipped
func TestPreviewMemberChanges(t *testing.T) {
	state := []previewMember{{Host: "a:1", Votes: 1, Priority: 1}, {Host: "d:1"}}
	planned := []previewMember{
		{Host: "a:1", Votes: 1, Priority: 1},       // unchanged
		{Host: "b:1", Votes: 1, Priority: 1},       // add, then promote
		{Host: ""},                                 // unknown at plan time
		{Host: "b:1", Votes: 1, Priority: 1},       // duplicate
		{Host: "c:1"},                              // add as a non-voter, nothing more
		{Host: "d:1", Votes: 1},                    // live member promoted
		{Host: "e:1", Votes: 1, ArbiterOnly: true}, // arbiter add, no promotion
	}
	cases := []struct {
		name    string
		state   []previewMember
		planned []previewMember
		want    []previewMemberChange
	}{
		{
			name:    "block order",
			state:   state,
			planned: planned,
			want: []previewMemberChange{
				{previewMember: planned[1], Add: true, Promote: true},
				{previewMember: planned[4], Add: true},
				{previewMember: planned[5], Promote: true},
				{previewMember: planned[6], Add: true},
			},
		},
		{name: "nil inputs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := previewMemberChanges(tc.state, tc.planned)
			if len(got) != len(tc.want) {
				t.Fatalf("want %d changes, got %d: %+v", len(tc.want), len(got), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("change %d: want %+v, got %+v", i, tc.want[i], got[i])
				}
			}
		})
	}
}

// PREVIEW-T25: previewMembersFromRaw reads the fields it needs and tolerates unknowns
func TestPreviewMembersFromRaw(t *testing.T) {
	cases := []struct {
		name string
		raw  interface{}
		want []previewMember
	}{
		{
			name: "reads fields and skips non-blocks",
			raw: []interface{}{
				map[string]interface{}{"host": "a:1", "priority": 2.0, "votes": 1, "arbiter_only": true},
				map[string]interface{}{"host": nil}, // unknown host reads as empty with zero fields
				"not a block",
			},
			want: []previewMember{{Host: "a:1", Votes: 1, Priority: 2, ArbiterOnly: true}, {}},
		},
		{name: "nil raw yields nil"}, // untyped nil, not a nil []interface{}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := previewMembersFromRaw(tc.raw); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// PREVIEW-T26: PREVIEW-025 — db_role preview reads actions from the set and sorts them
func TestDbRolePreview_ActionsFromSetAreSorted(t *testing.T) {
	res := resourceDatabaseRole()
	data := schema.TestResourceDataRaw(t, res.Schema, map[string]interface{}{
		"name": "customRole",
		"privilege": []interface{}{map[string]interface{}{
			"db":         "mydb",
			"collection": "users",
			"actions":    []interface{}{"update", "find", "insert"},
		}},
	})
	privs := extractPreviewPrivileges(data.Get("privilege").(*schema.Set).List())
	if len(privs) != 1 {
		t.Fatalf("expected 1 privilege, got %d", len(privs))
	}
	want := []string{"find", "insert", "update"}
	if !reflect.DeepEqual(privs[0].Actions, want) {
		t.Errorf("actions: got %v, want %v", privs[0].Actions, want)
	}
	got := buildDbRolePreview("admin", "customRole", privs, nil, true)
	if !strings.Contains(got, `actions: ["find", "insert", "update"]`) {
		t.Errorf("preview should list sorted actions, got: %s", got)
	}
}
