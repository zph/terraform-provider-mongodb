package mongodb

import "testing"

func TestResourceDatabaseUserParseId(t *testing.T) {
	checkParseId(t, resourceDatabaseUserParseId, []parseIdCase{
		{name: "TEST-001 valid", input: "admin.testuser", wantName: "testuser", wantDB: "admin"},
		{name: "TEST-002 no separator", input: "nodotshere", wantErr: true},
		{name: "TEST-003 empty database", input: ".username", wantErr: true},
		{name: "TEST-003 empty username", input: "database.", wantErr: true},
	})
}
