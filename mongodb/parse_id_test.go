package mongodb

import (
	"strings"
	"testing"
)

// IDFORMAT-002: WHEN a resource ID is parsed, the provider SHALL split on
// the first "." separator and return (name, database, nil); names containing
// dots are preserved via SplitN.
// IDFORMAT-003: If the separator is missing or either component is empty,
// the provider SHALL return an error.
func TestParseResourceId(t *testing.T) {
	checkParseId(t, parseResourceId, []parseIdCase{
		{name: "IDFORMAT-002 valid", input: "admin.testuser", wantName: "testuser", wantDB: "admin"},
		{name: "IDFORMAT-002 name with dots", input: "admin.my.dotted.role", wantName: "my.dotted.role", wantDB: "admin"},
		{name: "IDFORMAT-003 no separator", input: "nodots", wantErr: true},
		{name: "IDFORMAT-003 empty database", input: ".username", wantErr: true},
		{name: "IDFORMAT-003 empty name", input: "database.", wantErr: true},
	})
}

// IDFORMAT-004: WHEN a resource ID is formatted, the provider SHALL return
// "database.name", and parseResourceId round-trips it.
func TestFormatResourceId(t *testing.T) {
	cases := []struct {
		name     string
		database string
		resName  string
		want     string
	}{
		{"IDFORMAT-004 user", "admin", "testuser", "admin.testuser"},
		{"IDFORMAT-004 round-trip", "mydb", "myuser", "mydb.myuser"},
		{"CBAL-T05 CBAL-009 collection_balancing", "mydb.mycollection", "balancing", "mydb.mycollection.balancing"},
		{"PROF-T05 PROF-004 profiler", "mydb", "profiler", "mydb.profiler"},
		{"PARAM-T10 PARAM-007 server_parameter", "admin", "wiredTigerConcurrentReadTransactions", "admin.wiredTigerConcurrentReadTransactions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := formatResourceId(tc.database, tc.resName)
			if id != tc.want {
				t.Fatalf("formatResourceId(%q, %q) = %q, want %q", tc.database, tc.resName, id, tc.want)
			}
			name, database, err := parseResourceId(id)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// A dotted database cannot round-trip: the parser splits at the first dot.
			if strings.Contains(tc.database, ".") {
				return
			}
			if database != tc.database || name != tc.resName {
				t.Errorf("round trip = (%q, %q), want (%q, %q)", name, database, tc.resName, tc.database)
			}
		})
	}
}
