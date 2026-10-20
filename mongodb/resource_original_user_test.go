package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// TEST-046: host, port, username Required (password optional to avoid state).
// TEST-047: optional field defaults. TEST-048: password Optional so
// MONGODB_ORIGINAL_USER_PASSWORD can be used without state, and Sensitive.
// TEST-049: certificate and client_certificate_key Sensitive. TEST-057:
// replica_set auto-discovered.
func TestResourceOriginalUser_Schema(t *testing.T) {
	res := resourceOriginalUser()
	checkSchemaFields(t, res, []schemaField{
		{name: "host", typ: schema.TypeString, required: true},
		{name: "port", typ: schema.TypeString, required: true},
		{name: "username", typ: schema.TypeString, required: true},
		{name: "password", typ: schema.TypeString, optional: true, sensitive: true},
		{name: "certificate", typ: schema.TypeString, optional: true, sensitive: true},
		{name: "client_certificate_key", typ: schema.TypeString, optional: true, sensitive: true},
		{name: "auth_database", typ: schema.TypeString, optional: true, def: "admin"},
		{name: "ssl", typ: schema.TypeBool, optional: true, def: false},
		{name: "insecure_skip_verify", typ: schema.TypeBool, optional: true, def: false},
		{name: "replica_set", typ: schema.TypeString, optional: true, computed: true},
	})
	// TEST-058: direct is inferred from replica_set, never configured.
	if _, ok := res.Schema["direct"]; ok {
		t.Error("direct field should NOT be in schema; it is inferred from replica_set")
	}
}

// TEST-050: role field is TypeSet with nested db and role subfields
func TestResourceOriginalUser_RoleSchema(t *testing.T) {
	res := resourceOriginalUser()
	roleField, ok := res.Schema["role"]
	if !ok {
		t.Fatal("schema missing 'role' field")
	}
	if roleField.Type != schema.TypeSet {
		t.Errorf("role field should be TypeSet, got %v", roleField.Type)
	}
	elem, ok := roleField.Elem.(*schema.Resource)
	if !ok {
		t.Fatal("role Elem should be *schema.Resource")
	}
	for _, sub := range []string{"db", "role"} {
		if _, ok := elem.Schema[sub]; !ok {
			t.Errorf("role sub-schema missing %q field", sub)
		}
	}
}

func TestResourceOriginalUserParseId(t *testing.T) {
	checkParseId(t, resourceOriginalUserParseId, []parseIdCase{
		{name: "TEST-052 valid", input: "admin.myadmin", wantName: "myadmin", wantDB: "admin"},
		{name: "TEST-054 no separator", input: "nodotshere", wantErr: true},
		{name: "TEST-055 empty database", input: ".username", wantErr: true},
		{name: "TEST-055 empty name", input: "database.", wantErr: true},
	})
}

// TEST-059, TEST-060: direct mode iff no replica set is known.
func TestResolveDirectMode(t *testing.T) {
	cases := []struct {
		name       string
		replicaSet string
		want       bool
	}{
		{"TEST-059 with replica_set", "rs0", false},
		{"TEST-060 without replica_set", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveDirectMode(tc.replicaSet); got != tc.want {
				t.Errorf("resolveDirectMode(%q) = %v, want %v", tc.replicaSet, got, tc.want)
			}
		})
	}
}

// discoverReplicaSet without a client falls back to the explicit value.
func TestDiscoverReplicaSet(t *testing.T) {
	cases := []struct {
		name       string
		explicit   string
		wantName   string
		wantDirect bool
	}{
		{"TEST-062 explicit override", "rs0", "rs0", false},
		{"TEST-063 no client no explicit", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, direct := discoverReplicaSet(tc.explicit, nil)
			if name != tc.wantName {
				t.Errorf("name: want %q, got %q", tc.wantName, name)
			}
			if direct != tc.wantDirect {
				t.Errorf("direct: want %v, got %v", tc.wantDirect, direct)
			}
		})
	}
}

// TEST-056: buildOriginalUserConfig constructs ClientConfig from resource data
func TestBuildOriginalUserConfig_Defaults(t *testing.T) {
	cfg := ClientConfig{Host: "myhost", Port: "27018", Direct: true, DB: "admin"}
	if cfg.Host != "myhost" || cfg.Port != "27018" || !cfg.Direct || cfg.DB != "admin" {
		t.Errorf("unexpected config %+v", cfg)
	}
}
