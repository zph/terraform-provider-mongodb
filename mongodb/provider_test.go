package mongodb

import (
	"sort"
	"strings"
	"testing"
)

// TEST-030: Provider schema passes InternalValidate
func TestProviderSchemaValid(t *testing.T) {
	p := Provider()
	if err := p.InternalValidate(); err != nil {
		t.Fatalf("provider schema validation failed: %v", err)
	}
}

// TEST-031: Provider with no enable env var registers only mature resources
func TestProviderResourceMap_DefaultMatureOnly(t *testing.T) {
	t.Setenv(EnableEnvVar, "")
	p := Provider()
	expected := []string{"mongodb_db_user", "mongodb_db_role", "mongodb_original_user"}
	sort.Strings(expected)

	got := make([]string, 0, len(p.ResourcesMap))
	for k := range p.ResourcesMap {
		got = append(got, k)
	}
	sort.Strings(got)

	if len(got) != len(expected) {
		t.Fatalf("expected %d resources, got %d: %v", len(expected), len(got), got)
	}
	if strings.Join(got, ",") != strings.Join(expected, ",") {
		t.Errorf("resource mismatch: got %v, want %v", got, expected)
	}
}

// TEST-033: Provider with enable env var registers mature + opted-in experimental
func TestProviderResourceMap_WithExperimentalEnabled(t *testing.T) {
	t.Setenv(EnableEnvVar, "mongodb_shard_config,mongodb_shard")
	p := Provider()
	expected := []string{
		"mongodb_db_user", "mongodb_db_role", "mongodb_original_user",
		"mongodb_shard_config", "mongodb_shard",
	}
	sort.Strings(expected)

	got := make([]string, 0, len(p.ResourcesMap))
	for k := range p.ResourcesMap {
		got = append(got, k)
	}
	sort.Strings(got)

	if len(got) != len(expected) {
		t.Fatalf("expected %d resources, got %d: %v", len(expected), len(got), got)
	}
	if strings.Join(got, ",") != strings.Join(expected, ",") {
		t.Errorf("resource mismatch: got %v, want %v", got, expected)
	}
}

// TEST-032: Each resource schema passes InternalValidate (with all resources enabled)
func TestProviderResourceSchemasValid(t *testing.T) {
	t.Setenv(EnableEnvVar, "mongodb_shard_config,mongodb_shard")
	p := Provider()
	for name, resource := range p.ResourcesMap {
		t.Run(name, func(t *testing.T) {
			if err := resource.InternalValidate(nil, true); err != nil {
				t.Errorf("resource %q schema validation failed: %v", name, err)
			}
		})
	}
}
