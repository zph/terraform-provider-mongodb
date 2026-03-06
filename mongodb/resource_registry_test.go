package mongodb

import (
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// --- Maturity classification tests ---

// GATE-T01: Mature resources are classified correctly
func TestAllResources_MatureClassification(t *testing.T) {
	matureExpected := map[string]bool{
		"mongodb_db_user":       true,
		"mongodb_db_role":       true,
		"mongodb_original_user": true,
	}
	for _, reg := range AllResources() {
		if matureExpected[reg.Name] && reg.Maturity != ResourceMature {
			t.Errorf("resource %q should be mature, got %s", reg.Name, reg.Maturity)
		}
	}
}

// GATE-T02: Experimental resources are classified correctly
func TestAllResources_ExperimentalClassification(t *testing.T) {
	experimentalExpected := map[string]bool{
		"mongodb_shard_config":         true,
		"mongodb_shard":                true,
		"mongodb_profiler":             true,
		"mongodb_server_parameter":     true,
		"mongodb_balancer_config":      true,
		"mongodb_collection_balancing": true,
	}
	for _, reg := range AllResources() {
		if experimentalExpected[reg.Name] && reg.Maturity != ResourceExperimental {
			t.Errorf("resource %q should be experimental, got %s", reg.Name, reg.Maturity)
		}
	}
}

// GATE-T03: Every resource in the registry has a non-nil factory
func TestAllResources_FactoriesNonNil(t *testing.T) {
	for _, reg := range AllResources() {
		if reg.Factory == nil {
			t.Errorf("resource %q has nil Factory", reg.Name)
		}
	}
}

// --- BuildResourceMap tests ---

// GATE-T04: Default deny — nil enableList only includes mature resources
func TestBuildResourceMap_DefaultDenyExperimental(t *testing.T) {
	resources := BuildResourceMap(AllResources(), nil)

	// Mature resources MUST be present
	for _, name := range []string{"mongodb_db_user", "mongodb_db_role", "mongodb_original_user"} {
		if _, ok := resources[name]; !ok {
			t.Errorf("mature resource %q missing from default resource map", name)
		}
	}

	// Experimental resources MUST NOT be present
	for _, name := range []string{"mongodb_shard_config", "mongodb_shard", "mongodb_profiler", "mongodb_server_parameter", "mongodb_balancer_config", "mongodb_collection_balancing"} {
		if _, ok := resources[name]; ok {
			t.Errorf("experimental resource %q should NOT be in default resource map", name)
		}
	}
}

// GATE-T05: Explicit opt-in for one experimental resource
func TestBuildResourceMap_EnableOneExperimental(t *testing.T) {
	enableList := map[string]bool{"mongodb_shard_config": true}
	resources := BuildResourceMap(AllResources(), enableList)

	if _, ok := resources["mongodb_shard_config"]; !ok {
		t.Error("mongodb_shard_config should be enabled when in enableList")
	}
	if _, ok := resources["mongodb_shard"]; ok {
		t.Error("mongodb_shard should NOT be enabled when not in enableList")
	}
	// Mature resources still present
	if _, ok := resources["mongodb_db_user"]; !ok {
		t.Error("mongodb_db_user should always be present")
	}
}

// GATE-T06: Explicit opt-in for all experimental resources
func TestBuildResourceMap_EnableAllExperimental(t *testing.T) {
	enableList := map[string]bool{
		"mongodb_shard_config":         true,
		"mongodb_shard":                true,
		"mongodb_profiler":             true,
		"mongodb_server_parameter":     true,
		"mongodb_balancer_config":      true,
		"mongodb_collection_balancing": true,
	}
	resources := BuildResourceMap(AllResources(), enableList)

	expected := []string{
		"mongodb_db_user", "mongodb_db_role", "mongodb_original_user",
		"mongodb_shard_config", "mongodb_shard", "mongodb_profiler",
		"mongodb_server_parameter", "mongodb_balancer_config",
		"mongodb_collection_balancing",
	}
	for _, name := range expected {
		if _, ok := resources[name]; !ok {
			t.Errorf("resource %q missing when all experimental enabled", name)
		}
	}
}

// GATE-T07: Enabling a nonexistent resource name is harmless
func TestBuildResourceMap_UnknownEnableName(t *testing.T) {
	enableList := map[string]bool{"mongodb_nonexistent": true}
	resources := BuildResourceMap(AllResources(), enableList)

	// Only mature resources
	if len(resources) != 3 {
		names := make([]string, 0, len(resources))
		for k := range resources {
			names = append(names, k)
		}
		sort.Strings(names)
		t.Errorf("expected 3 mature resources, got %d: %v", len(resources), names)
	}
}

// GATE-T08: Empty enableList (non-nil but empty) still blocks experimental
func TestBuildResourceMap_EmptyEnableList(t *testing.T) {
	enableList := map[string]bool{}
	resources := BuildResourceMap(AllResources(), enableList)

	if _, ok := resources["mongodb_shard_config"]; ok {
		t.Error("experimental resource should be blocked with empty enableList")
	}
	if len(resources) != 3 {
		t.Errorf("expected 3 mature resources, got %d", len(resources))
	}
}

// GATE-T09: Mature resources cannot be blocked even if not in enableList
func TestBuildResourceMap_MatureAlwaysIncluded(t *testing.T) {
	// Pass an enableList that only mentions an experimental resource;
	// mature resources must still appear.
	enableList := map[string]bool{"mongodb_shard": true}
	resources := BuildResourceMap(AllResources(), enableList)

	for _, name := range []string{"mongodb_db_user", "mongodb_db_role", "mongodb_original_user"} {
		if _, ok := resources[name]; !ok {
			t.Errorf("mature resource %q must always be present, but was missing", name)
		}
	}
}

// GATE-T10: ResourceMaturity.String() returns expected labels
func TestResourceMaturity_String(t *testing.T) {
	cases := []struct {
		m    ResourceMaturity
		want string
	}{
		{ResourceMature, "mature"},
		{ResourceExperimental, "experimental"},
		{ResourceMaturity(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.m.String(); got != tc.want {
			t.Errorf("ResourceMaturity(%d).String() = %q, want %q", tc.m, got, tc.want)
		}
	}
}

// --- parseEnableList tests (via env var) ---

// GATE-T11: parseEnableList with comma-separated values
func TestParseEnableList_CommaSeparated(t *testing.T) {
	t.Setenv(EnableEnvVar, "mongodb_shard_config,mongodb_shard")
	result := parseEnableList()
	if !result["mongodb_shard_config"] {
		t.Error("mongodb_shard_config should be in enable list")
	}
	if !result["mongodb_shard"] {
		t.Error("mongodb_shard should be in enable list")
	}
}

// GATE-T12: parseEnableList with empty env var returns nil
func TestParseEnableList_Empty(t *testing.T) {
	t.Setenv(EnableEnvVar, "")
	result := parseEnableList()
	if result != nil {
		t.Errorf("expected nil for empty env var, got %v", result)
	}
}

// GATE-T13: parseEnableList trims whitespace
func TestParseEnableList_Whitespace(t *testing.T) {
	t.Setenv(EnableEnvVar, " mongodb_shard_config , mongodb_shard ")
	result := parseEnableList()
	if !result["mongodb_shard_config"] {
		t.Error("mongodb_shard_config should be in enable list after trim")
	}
	if !result["mongodb_shard"] {
		t.Error("mongodb_shard should be in enable list after trim")
	}
}

// GATE-T14: parseEnableList single value
func TestParseEnableList_SingleValue(t *testing.T) {
	t.Setenv(EnableEnvVar, "mongodb_shard")
	result := parseEnableList()
	if !result["mongodb_shard"] {
		t.Error("mongodb_shard should be in enable list")
	}
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
}

// --- Integration: BuildResourceMap with a custom registry ---

// GATE-T15: BuildResourceMap with a custom registry (not AllResources)
func TestBuildResourceMap_CustomRegistry(t *testing.T) {
	custom := []ResourceRegistration{
		{Name: "res_a", Factory: func() *schema.Resource { return &schema.Resource{} }, Maturity: ResourceMature},
		{Name: "res_b", Factory: func() *schema.Resource { return &schema.Resource{} }, Maturity: ResourceExperimental},
	}

	// Default: only res_a
	resources := BuildResourceMap(custom, nil)
	if _, ok := resources["res_a"]; !ok {
		t.Error("mature res_a should be present")
	}
	if _, ok := resources["res_b"]; ok {
		t.Error("experimental res_b should be blocked")
	}

	// Opted in: both
	resources = BuildResourceMap(custom, map[string]bool{"res_b": true})
	if _, ok := resources["res_b"]; !ok {
		t.Error("experimental res_b should be enabled when opted in")
	}
}
