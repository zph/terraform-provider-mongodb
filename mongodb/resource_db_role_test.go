package mongodb

import "testing"

// TEST-005: Valid plain text ID returns (roleName, database, nil)
func TestResourceDatabaseRoleParseId_Valid(t *testing.T) {
	roleName, database, err := resourceDatabaseRoleParseId("admin.myRole")
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
		input string
	}{
		{"no separator", "nodotshere"},
		{"empty database", ".roleName"},
		{"empty roleName", "database."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := resourceDatabaseRoleParseId(tc.input)
			if err == nil {
				t.Fatalf("expected error for case %q, got nil", tc.name)
			}
		})
	}
}
