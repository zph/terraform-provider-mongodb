package mongodb

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// --- Infrastructure tests ---

// PREVIEW-T01: PREVIEW-010 — commandPreviewEnabled returns false when meta is nil
func TestCommandPreviewEnabled_NilMeta(t *testing.T) {
	if commandPreviewEnabled(nil) {
		t.Error("expected false when meta is nil")
	}
}

// PREVIEW-T02: PREVIEW-002 — commandPreviewEnabled returns false when disabled
func TestCommandPreviewEnabled_Disabled(t *testing.T) {
	meta := &MongoDatabaseConfiguration{CommandPreview: false}
	if commandPreviewEnabled(meta) {
		t.Error("expected false when CommandPreview is false")
	}
}

// PREVIEW-T03: PREVIEW-003 — commandPreviewEnabled returns true when enabled
func TestCommandPreviewEnabled_Enabled(t *testing.T) {
	meta := &MongoDatabaseConfiguration{CommandPreview: true}
	if !commandPreviewEnabled(meta) {
		t.Error("expected true when CommandPreview is true")
	}
}

// PREVIEW-T04: PREVIEW-010 — commandPreviewEnabled returns false for wrong type
func TestCommandPreviewEnabled_WrongType(t *testing.T) {
	if commandPreviewEnabled("not a config") {
		t.Error("expected false for non-MongoDatabaseConfiguration type")
	}
}

// PREVIEW-T05: PREVIEW-005 — commandPreviewSchema returns Computed TypeString
func TestCommandPreviewSchema(t *testing.T) {
	s := commandPreviewSchema()
	if s.Type != schema.TypeString {
		t.Errorf("expected TypeString, got %v", s.Type)
	}
	if !s.Computed {
		t.Error("planned_commands should be Computed")
	}
}

// PREVIEW-T06: PREVIEW-001 — Provider schema includes command_preview
func TestProviderSchema_CommandPreview(t *testing.T) {
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
}

// PREVIEW-T07: PREVIEW-005 — All resources have planned_commands field
func TestCommandPreview_AllResourcesHavePlannedCommands(t *testing.T) {
	for _, reg := range AllResources() {
		res := reg.Factory()
		field, ok := res.Schema["planned_commands"]
		if !ok {
			t.Errorf("resource %q missing planned_commands field", reg.Name)
			continue
		}
		if !field.Computed {
			t.Errorf("resource %q planned_commands should be Computed", reg.Name)
		}
		if field.Type != schema.TypeString {
			t.Errorf("resource %q planned_commands type: want TypeString, got %v", reg.Name, field.Type)
		}
	}
}

// --- Per-resource builder tests ---

// PREVIEW-T10: PREVIEW-013 — server_parameter create preview
func TestServerParameterPreviewBuild(t *testing.T) {
	got := buildServerParameterPreview("notablescan", "true", true)
	want := `db.adminCommand({setParameter: 1, "notablescan": "true"})`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// PREVIEW-T11: PREVIEW-012 — profiler create preview
func TestProfilerPreviewBuild(t *testing.T) {
	got := buildProfilerPreview("mydb", 1, 100, 1)
	want := `db.getSiblingDB("mydb").runCommand({profile: 1, slowms: 100, ratelimit: 1})`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// PREVIEW-T12: PREVIEW-020 — fcv preview
func TestFCVPreviewBuild(t *testing.T) {
	got := buildFCVPreview("7.0")
	want := `db.adminCommand({setFeatureCompatibilityVersion: "7.0"})`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// PREVIEW-T13: PREVIEW-016 — db_user create preview (password redacted)
func TestDbUserPreviewBuild_Create(t *testing.T) {
	roles := []previewRole{{Role: "readWrite", DB: "mydb"}}
	got := buildDbUserPreview("admin", "appuser", roles, true, true)
	if !strings.Contains(got, "createUser") {
		t.Errorf("create preview should use createUser, got: %s", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("password should be redacted, got: %s", got)
	}
	if !strings.Contains(got, "readWrite") {
		t.Errorf("should include role name, got: %s", got)
	}
}

// PREVIEW-T14: PREVIEW-017, DANGER-021 — db_user update preview shows pwd
// only when the password changed
func TestDbUserPreviewBuild_Update(t *testing.T) {
	roles := []previewRole{{Role: "readWrite", DB: "mydb"}}

	got := buildDbUserPreview("admin", "appuser", roles, false, true)
	if !strings.Contains(got, "updateUser") {
		t.Errorf("update preview should use updateUser, got: %s", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("password-change preview should show redacted pwd, got: %s", got)
	}

	got = buildDbUserPreview("admin", "appuser", roles, false, false)
	if strings.Contains(got, "pwd") {
		t.Errorf("role-only update preview must omit pwd, got: %s", got)
	}
}

// PREVIEW-T15: PREVIEW-018, PREVIEW-019 — db_role create/update preview
func TestDbRolePreviewBuild_Create(t *testing.T) {
	privs := []previewPrivilege{{DB: "mydb", Collection: "users", Actions: []string{"find", "insert"}}}
	inherited := []previewRole{{Role: "read", DB: "mydb"}}
	got := buildDbRolePreview("admin", "customRole", privs, inherited, true)
	if !strings.Contains(got, "createRole") {
		t.Errorf("create preview should use createRole, got: %s", got)
	}
	if !strings.Contains(got, "find") {
		t.Errorf("should include action, got: %s", got)
	}
}

func TestDbRolePreviewBuild_Update(t *testing.T) {
	privs := []previewPrivilege{{DB: "mydb", Collection: "", Actions: []string{"find"}}}
	got := buildDbRolePreview("admin", "customRole", privs, nil, false)
	if !strings.Contains(got, "updateRole") {
		t.Errorf("update preview should use updateRole, got: %s", got)
	}
}

// PREVIEW-T16: PREVIEW-015 — shard create preview
func TestShardPreviewBuild(t *testing.T) {
	got := buildShardPreview("shard01", []string{"host1:27018", "host2:27018"})
	want := `db.adminCommand({addShard: "shard01/host1:27018,host2:27018"})`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// PREVIEW-T17: PREVIEW-024 — original_user create preview
func TestOriginalUserPreviewBuild(t *testing.T) {
	roles := []previewRole{{Role: "root", DB: "admin"}}
	got := buildOriginalUserPreview("admin", "admin", roles)
	if !strings.Contains(got, "createUser") {
		t.Errorf("should use createUser, got: %s", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("password should be redacted, got: %s", got)
	}
}

// PREVIEW-T18: PREVIEW-014 — collection_balancing shows both FCV paths
func TestCollectionBalancingPreviewBuild(t *testing.T) {
	got := buildCollectionBalancingPreview("mydb.users", true, 0)
	if !strings.Contains(got, "configureCollectionBalancing") {
		t.Errorf("should show FCV >= 6.0 path, got: %s", got)
	}
	if !strings.Contains(got, "config") {
		t.Errorf("should show legacy path, got: %s", got)
	}
}

func TestCollectionBalancingPreviewBuild_WithChunkSize(t *testing.T) {
	got := buildCollectionBalancingPreview("mydb.users", true, 128)
	if !strings.Contains(got, "chunkSize: 128") {
		t.Errorf("should include chunkSize, got: %s", got)
	}
}

// PREVIEW-T19: PREVIEW-021 — balancer_config multi-command preview
func TestBalancerConfigPreviewBuild(t *testing.T) {
	got := buildBalancerConfigPreview(balancerPreviewInput{
		Enabled:     true,
		WindowStart: "06:00",
		WindowStop:  "09:00",
		ChunkSizeMB: 128,
	})
	if !strings.Contains(got, "balancerStart") {
		t.Errorf("should include balancerStart, got: %s", got)
	}
	if !strings.Contains(got, "06:00") {
		t.Errorf("should include active window, got: %s", got)
	}
	if !strings.Contains(got, "128") {
		t.Errorf("should include chunk size, got: %s", got)
	}
}

func TestBalancerConfigPreviewBuild_Disabled(t *testing.T) {
	got := buildBalancerConfigPreview(balancerPreviewInput{Enabled: false})
	if !strings.Contains(got, "balancerStop") {
		t.Errorf("should include balancerStop, got: %s", got)
	}
}

// PREVIEW-T20: PREVIEW-022, PREVIEW-023 — shard_config preview
func TestShardConfigPreviewBuild_Create(t *testing.T) {
	got := buildShardConfigPreview("shard01", true, []string{"mongo2:27017", "mongo3:27017"})
	if !strings.Contains(got, "replSetInitiate") {
		t.Errorf("create should show replSetInitiate, got: %s", got)
	}
	if n := strings.Count(got, "replSetReconfig"); n != 3 {
		t.Errorf("create with two added hosts should show three replSetReconfig lines, got %d: %s", n, got)
	}
	for _, host := range []string{"mongo2:27017", "mongo3:27017"} {
		if !strings.Contains(got, host) {
			t.Errorf("create should name added host %s, got: %s", host, got)
		}
	}
}

func TestShardConfigPreviewBuild_Update(t *testing.T) {
	got := buildShardConfigPreview("shard01", false, nil)
	if strings.Contains(got, "replSetInitiate") {
		t.Errorf("update should not show replSetInitiate, got: %s", got)
	}
	if n := strings.Count(got, "replSetReconfig"); n != 1 {
		t.Errorf("update without added hosts should show one replSetReconfig, got %d: %s", n, got)
	}
}

// PREVIEW-T22: PREVIEW-023 — update lists one reconfig per added host, with _id max+1
func TestShardConfigPreviewBuild_UpdateAddsHosts(t *testing.T) {
	got := buildShardConfigPreview("shard01", false, []string{"mongo3:27017"})
	if n := strings.Count(got, "replSetReconfig"); n != 2 {
		t.Errorf("update with one added host should show two replSetReconfig lines, got %d: %s", n, got)
	}
	if !strings.Contains(got, `host: "mongo3:27017"`) {
		t.Errorf("added host line should name the host, got: %s", got)
	}
	if !strings.Contains(got, "<max+1>") {
		t.Errorf("added host line should show _id as max+1, got: %s", got)
	}
}

// PREVIEW-T23: previewAddedHosts skips known, empty and duplicate hosts, keeps order
func TestPreviewAddedHosts(t *testing.T) {
	got := previewAddedHosts([]string{"a:1"}, []string{"a:1", "b:1", "", "b:1", "c:1"})
	want := []string{"b:1", "c:1"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: want %s, got %s", i, want[i], got[i])
		}
	}
	if got := previewAddedHosts(nil, nil); len(got) != 0 {
		t.Errorf("nil inputs should yield no hosts, got %v", got)
	}
}

// PREVIEW-T24: memberHostsFromRaw reads hosts from the raw member list and tolerates unknowns
func TestMemberHostsFromRaw(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{"host": "a:1", "priority": 1.0},
		map[string]interface{}{"host": nil},
		"not a block",
	}
	got := memberHostsFromRaw(raw)
	if len(got) != 2 || got[0] != "a:1" || got[1] != "" {
		t.Errorf("want [a:1 \"\"], got %v", got)
	}
	if got := memberHostsFromRaw(nil); got != nil {
		t.Errorf("nil raw should yield nil, got %v", got)
	}
}

// PREVIEW-T21: ZONE-013 — shard zone preview shows addShardToZone
func TestShardZonePreviewBuild(t *testing.T) {
	got := buildShardZonePreview("shard01", "US-East")
	want := `db.adminCommand({addShardToZone: "shard01", zone: "US-East"})`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// PREVIEW-T22: ZONE-030 — zone key range preview shows updateZoneKeyRange
func TestZoneKeyRangePreviewBuild(t *testing.T) {
	got := buildZoneKeyRangePreview("app_db.orders", "US-East", `{"region": "east"}`, `{"region": "west"}`)
	if !strings.Contains(got, "updateZoneKeyRange") {
		t.Errorf("should contain updateZoneKeyRange, got: %s", got)
	}
	if !strings.Contains(got, `"app_db.orders"`) {
		t.Errorf("should contain namespace, got: %s", got)
	}
	if !strings.Contains(got, `"US-East"`) {
		t.Errorf("should contain zone name, got: %s", got)
	}
}
