package mongodb

import (
	"encoding/base64"
	"testing"
)

// TEST-005: Valid base64 ID returns (roleName, database, nil)
func TestResourceDatabaseRoleParseId_Valid(t *testing.T) {
	id := base64.StdEncoding.EncodeToString([]byte("admin.myRole"))
	roleName, database, err := resourceDatabaseRoleParseId(id)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if roleName != "myRole" {
		t.Errorf("expected roleName 'myRole', got '%s'", roleName)
	}
	if database != "admin" {
		t.Errorf("expected database 'admin', got '%s'", database)
	}
}

// TEST-006: Invalid inputs return errors
func TestResourceDatabaseRoleParseId_InvalidInputs(t *testing.T) {
	cases := []struct {
		name  string
		id    string
		raw   bool // if true, use id directly; if false, base64 encode it
	}{
		{"invalid base64", "not-valid!@#", true},
		{"no separator", "nodotshere", false},
		{"empty database", ".roleName", false},
		{"empty roleName", "database.", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := tc.id
			if !tc.raw {
				id = base64.StdEncoding.EncodeToString([]byte(tc.id))
			}
			_, _, err := resourceDatabaseRoleParseId(id)
			if err == nil {
				t.Fatalf("expected error for case %q, got nil", tc.name)
			}
		})
	}
}
