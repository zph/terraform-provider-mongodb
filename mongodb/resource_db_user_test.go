package mongodb

import (
	"encoding/base64"
	"testing"
)

// TEST-001: Valid base64 ID returns (username, database, nil)
func TestResourceDatabaseUserParseId_Valid(t *testing.T) {
	id := base64.StdEncoding.EncodeToString([]byte("admin.testuser"))
	username, database, err := resourceDatabaseUserParseId(id)
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

// TEST-002: Invalid base64 returns error
func TestResourceDatabaseUserParseId_InvalidBase64(t *testing.T) {
	_, _, err := resourceDatabaseUserParseId("not-valid-base64!@#")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

// TEST-003: Valid base64 without separator returns error
func TestResourceDatabaseUserParseId_NoSeparator(t *testing.T) {
	id := base64.StdEncoding.EncodeToString([]byte("nodotshere"))
	_, _, err := resourceDatabaseUserParseId(id)
	if err == nil {
		t.Fatal("expected error for missing separator, got nil")
	}
}

// TEST-004: Empty database or username returns error
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
			id := base64.StdEncoding.EncodeToString([]byte(tc.input))
			_, _, err := resourceDatabaseUserParseId(id)
			if err == nil {
				t.Fatalf("expected error for input %q, got nil", tc.input)
			}
		})
	}
}
