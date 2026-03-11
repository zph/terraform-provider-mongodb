package mongodb

import (
	"testing"
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
		"mongodb_shard_config":                  true,
		"mongodb_shard":                         true,
		"mongodb_profiler":                      true,
		"mongodb_server_parameter":              true,
		"mongodb_balancer_config":               true,
		"mongodb_collection_balancing":          true,
		"mongodb_feature_compatibility_version": true,
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

// GATE-T04: BuildResourceMap registers all resources unconditionally
func TestBuildResourceMap_AllRegistered(t *testing.T) {
	resources := BuildResourceMap(AllResources())
	for _, reg := range AllResources() {
		if _, ok := resources[reg.Name]; !ok {
			t.Errorf("resource %q missing from resource map", reg.Name)
		}
	}
}

// GATE-T05: requireFeature blocks when feature not in enabled set
func TestRequireFeature_Blocks(t *testing.T) {
	fn := requireFeature("mongodb_shard")
	meta := &MongoDatabaseConfiguration{FeaturesEnabled: map[string]bool{}}
	err := fn(nil, nil, meta)
	if err == nil {
		t.Error("expected error when feature not enabled")
	}
}

// GATE-T06: requireFeature allows when feature is in enabled set
func TestRequireFeature_Allows(t *testing.T) {
	fn := requireFeature("mongodb_shard")
	meta := &MongoDatabaseConfiguration{FeaturesEnabled: map[string]bool{"mongodb_shard": true}}
	err := fn(nil, nil, meta)
	if err != nil {
		t.Errorf("expected nil error when feature enabled, got: %v", err)
	}
}

// GATE-T07: requireFeature allows when meta is nil (validate phase)
func TestRequireFeature_NilMeta(t *testing.T) {
	fn := requireFeature("mongodb_shard")
	err := fn(nil, nil, nil)
	if err != nil {
		t.Errorf("expected nil error for nil meta, got: %v", err)
	}
}

// GATE-T08: mergeEnableLists combines env and HCL sources
func TestMergeEnableLists(t *testing.T) {
	env := map[string]bool{"mongodb_shard": true}
	hcl := map[string]bool{"mongodb_profiler": true}
	got := mergeEnableLists(env, hcl)
	if !got["mongodb_shard"] || !got["mongodb_profiler"] {
		t.Errorf("expected both sources merged, got %v", got)
	}
}

// GATE-T09: mergeEnableLists handles nil inputs
func TestMergeEnableLists_Nil(t *testing.T) {
	got := mergeEnableLists(nil, nil)
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
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
func TestParseEnableList_EmptyEnv(t *testing.T) {
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

// GATE-T14: experimentalResourceNames returns names of all experimental resources
func TestExperimentalResourceNames(t *testing.T) {
	names := experimentalResourceNames()
	if len(names) < 7 {
		t.Errorf("expected at least 7 experimental resources, got %d", len(names))
	}
}
