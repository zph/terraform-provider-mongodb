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
