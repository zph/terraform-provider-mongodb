package mongodb

import "testing"

// TEST-001: Valid plain text ID returns (username, database, nil)
func TestResourceDatabaseUserParseId_Valid(t *testing.T) {
	username, database, err := resourceDatabaseUserParseId("admin.testuser")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if username != "testuser" {
		t.Errorf("expected username 'testuser', got '%s'", username)
	}
	if database != "admin" {
		t.Errorf("expected database 'admin', got '%s'", database)
	}
}

// TEST-002: ID without separator returns error
func TestResourceDatabaseUserParseId_NoSeparator(t *testing.T) {
	_, _, err := resourceDatabaseUserParseId("nodotshere")
	if err == nil {
		t.Fatal("expected error for missing separator, got nil")
	}
}

// TEST-003: Empty database or username returns error
func TestResourceDatabaseUserParseId_EmptyParts(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty database", ".username"},
		{"empty username", "database."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := resourceDatabaseUserParseId(tc.input)
			if err == nil {
				t.Fatalf("expected error for input %q, got nil", tc.input)
			}
		})
	}
}
