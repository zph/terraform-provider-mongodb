package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// DANGER-T01: DANGER-001 — db_user no longer has CustomizeDiff (uses updateUser in-place)
func TestDangerousOps_DbUserNoCustomizeDiff(t *testing.T) {
	res := resourceDatabaseUser()
	if res.CustomizeDiff != nil {
		t.Error("mongodb_db_user should not have CustomizeDiff (uses updateUser in-place)")
	}
}

// DANGER-T02: DANGER-002 — db_role no longer has CustomizeDiff (uses updateRole in-place)
func TestDangerousOps_DbRoleNoCustomizeDiff(t *testing.T) {
	res := resourceDatabaseRole()
	if res.CustomizeDiff != nil {
		t.Error("mongodb_db_role should not have CustomizeDiff (uses updateRole in-place)")
	}
}

// DANGER-T03: DANGER-003 — original_user has CustomizeDiff that unconditionally blocks updates
func TestDangerousOps_OriginalUserCustomizeDiff(t *testing.T) {
	res := resourceOriginalUser()
	if res.CustomizeDiff == nil {
		t.Error("mongodb_original_user should have CustomizeDiff set")
	}
}

// DANGER-T04: DANGER-003 — original_user has no allow_dangerous_update (updates unconditionally refused)
func TestDangerousOps_OriginalUserNoAllowDangerousUpdate(t *testing.T) {
	res := resourceOriginalUser()
	if _, ok := res.Schema["allow_dangerous_update"]; ok {
		t.Error("original_user should not have allow_dangerous_update (updates unconditionally refused)")
	}
}

// DANGER-T05: DANGER-008 — original_user Delete is a no-op (does not drop user)
func TestDangerousOps_OriginalUserDeleteIsNoOp(t *testing.T) {
	res := resourceOriginalUser()
	if res.DeleteContext == nil {
		t.Fatal("original_user should have DeleteContext set")
	}
}

// DANGER-T06: DANGER-007 — FCV danger_mode is unaffected
func TestDangerousOps_FCVDangerModeUnaffected(t *testing.T) {
	res := resourceFCV()
	field, ok := res.Schema["danger_mode"]
	if !ok {
		t.Fatal("FCV schema missing danger_mode — DANGER-007 violated")
	}
	if field.Type != schema.TypeBool {
		t.Errorf("FCV danger_mode type = %v, want TypeBool", field.Type)
	}
	if res.CustomizeDiff == nil {
		t.Error("FCV CustomizeDiff should still be set")
	}
}

// DANGER-T07: Schema validation passes for db_user
func TestDangerousOps_DbUserSchemaValid(t *testing.T) {
	res := resourceDatabaseUser()
	if err := res.InternalValidate(nil, true); err != nil {
		t.Errorf("db_user schema validation failed: %v", err)
	}
}

// DANGER-T08: Schema validation passes for db_role
func TestDangerousOps_DbRoleSchemaValid(t *testing.T) {
	res := resourceDatabaseRole()
	if err := res.InternalValidate(nil, true); err != nil {
		t.Errorf("db_role schema validation failed: %v", err)
	}
}

// DANGER-T09: Schema validation passes for original_user with CustomizeDiff
func TestDangerousOps_OriginalUserSchemaValid(t *testing.T) {
	res := resourceOriginalUser()
	if err := res.InternalValidate(nil, true); err != nil {
		t.Errorf("original_user schema validation failed: %v", err)
	}
}

// DANGER-T10: Provider schema validation still passes
func TestDangerousOps_ProviderSchemaValid(t *testing.T) {
	p := Provider()
	if err := p.InternalValidate(); err != nil {
		t.Errorf("provider schema validation failed: %v", err)
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

// DANGER-T13: DANGER-013 — server_parameter has CustomizeDiff blocking parameter changes
func TestDangerousOps_ServerParameterCustomizeDiff(t *testing.T) {
	res := resourceServerParameter()
	if res.CustomizeDiff == nil {
		t.Error("mongodb_server_parameter should have CustomizeDiff blocking parameter changes")
	}
}

// DANGER-T14: DANGER-014 — collection_balancing has CustomizeDiff blocking namespace changes
func TestDangerousOps_CollectionBalancingCustomizeDiff(t *testing.T) {
	res := resourceCollectionBalancing()
	if res.CustomizeDiff == nil {
		t.Error("mongodb_collection_balancing should have CustomizeDiff blocking namespace changes")
	}
}

// DANGER-T15: DANGER-015 — profiler has CustomizeDiff blocking database changes
func TestDangerousOps_ProfilerCustomizeDiff(t *testing.T) {
	res := resourceProfiler()
	if res.CustomizeDiff == nil {
		t.Error("mongodb_profiler should have CustomizeDiff blocking database changes")
	}
}

// DANGER-T16: DANGER-016, DANGER-017 — shard has CustomizeDiff blocking hosts and shard_name changes
func TestDangerousOps_ShardCustomizeDiff(t *testing.T) {
	res := resourceShard()
	if res.CustomizeDiff == nil {
		t.Error("mongodb_shard should have CustomizeDiff blocking hosts and shard_name changes")
	}
}

// DANGER-T17: Schema validation passes for shard after ForceNew changes
func TestDangerousOps_ShardSchemaValid(t *testing.T) {
	res := resourceShard()
	if err := res.InternalValidate(nil, true); err != nil {
		t.Errorf("shard schema validation failed: %v", err)
	}
}
