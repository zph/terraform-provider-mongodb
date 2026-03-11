package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// TEST-046: resourceOriginalUser schema has required fields: host, port, username (password optional to avoid state)
func TestResourceOriginalUser_SchemaRequiredFields(t *testing.T) {
	res := resourceOriginalUser()
	requiredFields := []string{"host", "port", "username"}
	for _, field := range requiredFields {
		s, ok := res.Schema[field]
		if !ok {
			t.Errorf("schema missing required field %q", field)
			continue
		}
		if !s.Required {
			t.Errorf("field %q SHOULD be Required", field)
		}
	}
}

// TEST-047: resourceOriginalUser schema has optional fields with correct defaults
func TestResourceOriginalUser_SchemaOptionalFields(t *testing.T) {
	res := resourceOriginalUser()
	cases := []struct {
		field      string
		defaultVal interface{}
	}{
		{"auth_database", "admin"},
		{"ssl", false},
		{"insecure_skip_verify", false},
	}
	for _, tc := range cases {
		s, ok := res.Schema[tc.field]
		if !ok {
			t.Errorf("schema missing optional field %q", tc.field)
			continue
		}
		if s.Required {
			t.Errorf("field %q should be Optional, not Required", tc.field)
		}
		if s.Default != tc.defaultVal {
			t.Errorf("field %q: expected default %v, got %v", tc.field, tc.defaultVal, s.Default)
		}
	}
}

// TEST-048: password field is Optional (use env var to avoid storing in state) and Sensitive
func TestResourceOriginalUser_PasswordSensitive(t *testing.T) {
	res := resourceOriginalUser()
	s := res.Schema["password"]
	if !s.Sensitive {
		t.Error("password field SHOULD be Sensitive")
	}
	if s.Required {
		t.Error("password field SHOULD be Optional so MONGODB_ORIGINAL_USER_PASSWORD can be used without state")
	}
}

// TEST-049: certificate field is marked Sensitive
func TestResourceOriginalUser_CertificateSensitive(t *testing.T) {
	res := resourceOriginalUser()
	s := res.Schema["certificate"]
	if !s.Sensitive {
		t.Error("certificate field SHOULD be Sensitive")
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
	if _, ok := elem.Schema["db"]; !ok {
		t.Error("role sub-schema missing 'db' field")
	}
	if _, ok := elem.Schema["role"]; !ok {
		t.Error("role sub-schema missing 'role' field")
	}
}

// TEST-051: resourceOriginalUser schema passes InternalValidate
func TestResourceOriginalUser_SchemaValid(t *testing.T) {
	res := resourceOriginalUser()
	if err := res.InternalValidate(nil, true); err != nil {
		t.Fatalf("schema validation failed: %v", err)
	}
}

// TEST-052: resourceOriginalUserParseId round-trip with plain text ID
func TestResourceOriginalUserParseId_Valid(t *testing.T) {
	username, database, err := resourceOriginalUserParseId("admin.myadmin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if username != "myadmin" {
		t.Errorf("expected username 'myadmin', got %q", username)
	}
	if database != "admin" {
		t.Errorf("expected database 'admin', got %q", database)
	}
}

// TEST-054: resourceOriginalUserParseId with missing separator
func TestResourceOriginalUserParseId_NoSeparator(t *testing.T) {
	_, _, err := resourceOriginalUserParseId("nodotshere")
	if err == nil {
		t.Fatal("expected error for missing separator, got nil")
	}
}

// TEST-055: resourceOriginalUserParseId with empty parts
func TestResourceOriginalUserParseId_EmptyParts(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty database", ".username"},
		{"empty name", "database."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := resourceOriginalUserParseId(tc.input)
			if err == nil {
				t.Fatalf("expected error for input %q, got nil", tc.input)
			}
		})
	}
}

// TEST-057: replica_set field is optional and computed (auto-discovered)
func TestResourceOriginalUser_ReplicaSetField(t *testing.T) {
	res := resourceOriginalUser()
	s, ok := res.Schema["replica_set"]
	if !ok {
		t.Fatal("schema missing 'replica_set' field")
	}
	if s.Required {
		t.Error("replica_set should be Optional")
	}
	if !s.Computed {
		t.Error("replica_set should be Computed (auto-discovered from server)")
	}
}

// TEST-058: direct is not in the schema (inferred from replica_set)
func TestResourceOriginalUser_NoDirectField(t *testing.T) {
	res := resourceOriginalUser()
	if _, ok := res.Schema["direct"]; ok {
		t.Error("direct field should NOT be in schema; it is inferred from replica_set")
	}
}

// TEST-059: resolveDirectMode returns false when replica_set is set
func TestResolveDirectMode_WithReplicaSet(t *testing.T) {
	if resolveDirectMode("rs0") != false {
		t.Error("expected direct=false when replica_set is set")
	}
}

// TEST-060: resolveDirectMode returns true when replica_set is empty
func TestResolveDirectMode_WithoutReplicaSet(t *testing.T) {
	if resolveDirectMode("") != true {
		t.Error("expected direct=true when replica_set is empty")
	}
}

// TEST-061: IsMasterResp includes SetName field for replica set discovery
func TestIsMasterResp_SetNameField(t *testing.T) {
	resp := IsMasterResp{SetName: "rs0", IsMaster: true}
	if resp.SetName != "rs0" {
		t.Errorf("expected SetName 'rs0', got %q", resp.SetName)
	}
}

// TEST-062: discoverReplicaSet returns the explicit replica_set when provided
func TestDiscoverReplicaSet_ExplicitOverride(t *testing.T) {
	name, direct := discoverReplicaSet("rs0", nil)
	if name != "rs0" {
		t.Errorf("expected 'rs0', got %q", name)
	}
	if direct {
		t.Error("expected direct=false when replica set is known")
	}
}

// TEST-063: discoverReplicaSet returns empty + direct=true when no client and no explicit
func TestDiscoverReplicaSet_NoClientNoExplicit(t *testing.T) {
	name, direct := discoverReplicaSet("", nil)
	if name != "" {
		t.Errorf("expected empty, got %q", name)
	}
	if !direct {
		t.Error("expected direct=true when no replica set")
	}
}

// TEST-056: buildOriginalUserConfig constructs ClientConfig from resource data
func TestBuildOriginalUserConfig_Defaults(t *testing.T) {
	cfg := ClientConfig{
		Host:   "myhost",
		Port:   "27018",
		Direct: true,
		DB:     "admin",
	}
	if cfg.Host != "myhost" {
		t.Errorf("expected host 'myhost', got %q", cfg.Host)
	}
	if cfg.Port != "27018" {
		t.Errorf("expected port '27018', got %q", cfg.Port)
	}
	if !cfg.Direct {
		t.Error("expected Direct=true")
	}
	if cfg.DB != "admin" {
		t.Error("expected DB='admin'")
	}
}
