package mongodb

import (
	"testing"
)

// TEST-030: Provider schema passes InternalValidate
func TestProviderSchemaValid(t *testing.T) {
	p := Provider()
	if err := p.InternalValidate(); err != nil {
		t.Fatalf("provider schema validation failed: %v", err)
	}
}

// TEST-031: All resources (mature + experimental) are registered unconditionally
func TestProviderResourceMap_AllRegistered(t *testing.T) {
	p := Provider()
	for _, reg := range AllResources() {
		if _, ok := p.ResourcesMap[reg.Name]; !ok {
			t.Errorf("resource %q not registered in ResourcesMap", reg.Name)
		}
	}
}

// TEST-032: Each resource schema passes InternalValidate
func TestProviderResourceSchemasValid(t *testing.T) {
	p := Provider()
	for name, resource := range p.ResourcesMap {
		t.Run(name, func(t *testing.T) {
			if err := resource.InternalValidate(nil, true); err != nil {
				t.Errorf("resource %q schema validation failed: %v", name, err)
			}
		})
	}
}

// TEST-033: Provider schema includes features_enabled field
func TestProviderSchema_FeaturesEnabled(t *testing.T) {
	p := Provider()
	field, ok := p.Schema["features_enabled"]
	if !ok {
		t.Fatal("provider schema missing features_enabled field")
	}
	if !field.Optional {
		t.Error("features_enabled should be Optional")
	}
}
