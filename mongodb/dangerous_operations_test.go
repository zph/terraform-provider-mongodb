package mongodb

import (
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Every resource that must refuse or preview a change does so through
// CustomizeDiff rather than ForceNew (DANGER-010).
func TestDangerousOps_CustomizeDiffPresent(t *testing.T) {
	cases := []struct {
		name    string
		factory func() *schema.Resource
	}{
		// db_user/db_role update in place; CustomizeDiff is for command preview (PREVIEW-016..019).
		{"DANGER-T01 DANGER-001 db_user", resourceDatabaseUser},
		{"DANGER-T02 DANGER-002 db_role", resourceDatabaseRole},
		{"DANGER-T03 DANGER-003 original_user blocks all updates", resourceOriginalUser},
		// danger_mode's type is asserted in TestFCVSchema.
		{"DANGER-T06 FCV-T04 DANGER-007 fcv danger_mode unaffected", resourceFCV},
		{"DANGER-T13 DANGER-013 server_parameter blocks parameter changes", resourceServerParameter},
		{"DANGER-T14 DANGER-014 collection_balancing blocks namespace changes", resourceCollectionBalancing},
		{"DANGER-T15 DANGER-015 profiler blocks database changes", resourceProfiler},
		{"DANGER-T16 DANGER-016/017 shard blocks hosts and shard_name changes", resourceShard},
		{"BAL-T09 BAL-010 balancer_config requires both window bounds", resourceBalancerConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.factory().CustomizeDiff == nil {
				t.Error("CustomizeDiff should be set")
			}
		})
	}
}

// DANGER-T04, DANGER-T05: DANGER-003/008 — original_user has no
// allow_dangerous_update escape hatch and its Delete is a no-op that must
// still be wired.
func TestDangerousOps_OriginalUser(t *testing.T) {
	res := resourceOriginalUser()
	if _, ok := res.Schema["allow_dangerous_update"]; ok {
		t.Error("original_user should not have allow_dangerous_update (updates unconditionally refused)")
	}
	if res.DeleteContext == nil {
		t.Fatal("original_user should have DeleteContext set")
	}
}

// DANGER-T11: DANGER-010, DANGER-012 — No ForceNew in any resource schema
// except explicitly allowlisted fields.
func TestDangerousOps_NoForceNewExceptAllowlist(t *testing.T) {
	// DANGER-018: Exactly one allowed entry
	allowlist := map[string]map[string]bool{
		"mongodb_shard": {
			"shard_name": true,
		},
	}

	for _, reg := range AllResources() {
		res := reg.Factory()
		for fieldName, field := range res.Schema {
			if field.ForceNew {
				if allowlist[reg.Name][fieldName] {
					continue
				}
				t.Errorf("resource %q field %q has ForceNew: true (banned by DANGER-010); "+
					"use CustomizeDiff to block changes instead", reg.Name, fieldName)
			}
		}
	}
}

// DANGER-T12: DANGER-018 — Allowlisted ForceNew entries MUST actually exist
// and have ForceNew set (prevents stale allowlist entries).
func TestDangerousOps_AllowlistEntriesExist(t *testing.T) {
	allowlist := map[string]string{
		"mongodb_shard": "shard_name",
	}

	registry := make(map[string]func() *schema.Resource)
	for _, reg := range AllResources() {
		registry[reg.Name] = reg.Factory
	}

	for resName, fieldName := range allowlist {
		factory, ok := registry[resName]
		if !ok {
			t.Errorf("allowlist entry %q: resource not found in AllResources()", resName)
			continue
		}
		res := factory()
		field, ok := res.Schema[fieldName]
		if !ok {
			t.Errorf("allowlist entry %q.%q: field not found in schema", resName, fieldName)
			continue
		}
		if !field.ForceNew {
			t.Errorf("allowlist entry %q.%q: field does not have ForceNew (stale allowlist?)",
				resName, fieldName)
		}
	}
}

// DANGER-T18: DANGER-023 — db_user password schema rejects empty values
func TestDangerousOps_DbUserPasswordNotEmpty(t *testing.T) {
	s := resourceDatabaseUser().Schema["password"]
	if s.ValidateDiagFunc == nil {
		t.Fatal("password must have a ValidateDiagFunc")
	}
	if diags := s.ValidateDiagFunc("", cty.Path{}); !diags.HasError() {
		t.Error("empty password must fail validation")
	}
	if diags := s.ValidateDiagFunc("nonempty", cty.Path{}); diags.HasError() {
		t.Errorf("non-empty password must pass validation: %+v", diags)
	}
}

// DANGER-T19: DANGER-021, DANGER-023 — Update sends pwd only on a real,
// non-empty password change
func TestDangerousOps_IncludePasswordForUpdate(t *testing.T) {
	cases := []struct {
		name      string
		hasChange bool
		password  string
		want      bool
		wantErr   bool
	}{
		{"unchanged non-empty", false, "keep", false, false},
		{"unchanged empty (imported)", false, "", false, false},
		{"changed non-empty (rotation)", true, "newpass", true, false},
		{"changed to empty (must error)", true, "", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := includePasswordForUpdate(fakeChangeChecker{t: t, changed: tc.hasChange}, tc.password)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("includePassword = %v, want %v", got, tc.want)
			}
		})
	}
}

type fakeChangeChecker struct {
	t       *testing.T
	changed bool
}

func (f fakeChangeChecker) HasChange(key string) bool {
	if key != "password" {
		f.t.Fatalf("HasChange called with unexpected key %q", key)
	}
	return f.changed
}
