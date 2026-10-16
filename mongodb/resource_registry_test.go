package mongodb

import (
	"testing"
)

// GATE-T01, GATE-T02: maturity classification per resource; GATE-T03: every
// entry has a factory.
func TestAllResources(t *testing.T) {
	want := map[string]ResourceMaturity{
		"mongodb_db_user":                       ResourceMature,
		"mongodb_db_role":                       ResourceMature,
		"mongodb_original_user":                 ResourceMature,
		"mongodb_shard_config":                  ResourceExperimental,
		"mongodb_shard":                         ResourceExperimental,
		"mongodb_profiler":                      ResourceExperimental,
		"mongodb_server_parameter":              ResourceExperimental,
		"mongodb_balancer_config":               ResourceExperimental,
		"mongodb_collection_balancing":          ResourceExperimental,
		"mongodb_feature_compatibility_version": ResourceExperimental,
	}
	for _, reg := range AllResources() {
		t.Run(reg.Name, func(t *testing.T) {
			if m, ok := want[reg.Name]; ok && reg.Maturity != m {
				t.Errorf("maturity: want %s, got %s", m, reg.Maturity)
			}
			if reg.Factory == nil {
				t.Error("nil Factory")
			}
		})
	}
}

// GATE-T04: BuildResourceMap registers all resources unconditionally
func TestBuildResourceMap_AllRegistered(t *testing.T) {
	resources := BuildResourceMap(AllResources())
	for _, reg := range AllResources() {
		if _, ok := resources[reg.Name]; !ok {
			t.Errorf("resource %q missing from resource map", reg.Name)
		}
	}
}

func TestRequireFeature(t *testing.T) {
	cases := []struct {
		name    string
		meta    interface{}
		wantErr bool
	}{
		{"GATE-T05 blocks when not enabled", &MongoDatabaseConfiguration{FeaturesEnabled: map[string]bool{}}, true},
		{"GATE-T06 allows when enabled", &MongoDatabaseConfiguration{FeaturesEnabled: map[string]bool{"mongodb_shard": true}}, false},
		// Validate phase runs with no provider meta.
		{"GATE-T07 allows nil meta", nil, false},
	}
	fn := requireFeature("mongodb_shard")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := fn(nil, nil, tc.meta)
			if (err != nil) != tc.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestMergeEnableLists(t *testing.T) {
	cases := []struct {
		name     string
		env, hcl map[string]bool
		want     []string
	}{
		{"GATE-T08 combines env and HCL", map[string]bool{"mongodb_shard": true}, map[string]bool{"mongodb_profiler": true}, []string{"mongodb_shard", "mongodb_profiler"}},
		{"GATE-T09 nil inputs", nil, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeEnableLists(tc.env, tc.hcl)
			if len(got) != len(tc.want) {
				t.Errorf("want %v, got %v", tc.want, got)
			}
			for _, name := range tc.want {
				if !got[name] {
					t.Errorf("expected %s in merged list, got %v", name, got)
				}
			}
		})
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

// parseEnableList reads the enable list from the env var.
func TestParseEnableList(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		want    []string
		wantNil bool
	}{
		{"GATE-T11 comma-separated", "mongodb_shard_config,mongodb_shard", []string{"mongodb_shard_config", "mongodb_shard"}, false},
		{"GATE-T12 empty env returns nil", "", nil, true},
		{"GATE-T13 trims whitespace", " mongodb_shard_config , mongodb_shard ", []string{"mongodb_shard_config", "mongodb_shard"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnableEnvVar, tc.env)
			got := parseEnableList()
			if tc.wantNil && got != nil {
				t.Errorf("expected nil for empty env var, got %v", got)
			}
			for _, name := range tc.want {
				if !got[name] {
					t.Errorf("%s should be in enable list, got %v", name, got)
				}
			}
		})
	}
}

// GATE-T14: experimentalResourceNames returns names of all experimental resources
func TestExperimentalResourceNames(t *testing.T) {
	names := experimentalResourceNames()
	if len(names) < 7 {
		t.Errorf("expected at least 7 experimental resources, got %d", len(names))
	}
}
