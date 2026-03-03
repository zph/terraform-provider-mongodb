package mongodb

import "testing"

// IDFORMAT-002: WHEN a resource ID is parsed, the provider SHALL split on
// the first "." separator and return (name, database, nil).
func TestParseResourceId_Valid(t *testing.T) {
	name, database, err := parseResourceId("admin.testuser")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if name != "testuser" {
		t.Errorf("expected name 'testuser', got %q", name)
	}
	if database != "admin" {
		t.Errorf("expected database 'admin', got %q", database)
	}
}

// IDFORMAT-002: Names containing dots are preserved via SplitN.
func TestParseResourceId_NameWithDots(t *testing.T) {
	name, database, err := parseResourceId("admin.my.dotted.role")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if name != "my.dotted.role" {
		t.Errorf("expected name 'my.dotted.role', got %q", name)
	}
	if database != "admin" {
		t.Errorf("expected database 'admin', got %q", database)
	}
}

// IDFORMAT-003: If a resource ID is missing a "." separator, the provider
// SHALL return an error.
func TestParseResourceId_NoSeparator(t *testing.T) {
	_, _, err := parseResourceId("nodots")
	if err == nil {
		t.Fatal("expected error for missing separator, got nil")
	}
}

// IDFORMAT-003: If database or name component is empty, the provider SHALL
// return an error.
func TestParseResourceId_EmptyParts(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty database", ".username"},
		{"empty name", "database."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseResourceId(tc.input)
			if err == nil {
				t.Fatalf("expected error for input %q, got nil", tc.input)
			}
		})
	}
}

// IDFORMAT-004: WHEN a resource ID is formatted, the provider SHALL return
// "database.name".
func TestFormatResourceId(t *testing.T) {
	got := formatResourceId("admin", "testuser")
	if got != "admin.testuser" {
		t.Errorf("expected 'admin.testuser', got %q", got)
	}
}

// IDFORMAT-004: Round-trip: formatResourceId then parseResourceId.
func TestFormatParseRoundTrip(t *testing.T) {
	id := formatResourceId("mydb", "myuser")
	name, database, err := parseResourceId(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if database != "mydb" {
		t.Errorf("expected database 'mydb', got %q", database)
	}
	if name != "myuser" {
		t.Errorf("expected name 'myuser', got %q", name)
	}
}
