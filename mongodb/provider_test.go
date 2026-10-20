package mongodb

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// TEST-030: Provider schema passes InternalValidate (also DANGER-T10)
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

// TEST-032: Each resource schema passes InternalValidate. With TEST-031 this
// covers the per-resource checks DANGER-T07, T08, T09, T17 and TEST-051.
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

// TEST-033: provider schema fields. Both PEM inputs are Sensitive so they
// stay out of plan output and logs.
func TestProviderSchema_Fields(t *testing.T) {
	p := Provider()
	checkSchemaFields(t, &schema.Resource{Schema: p.Schema}, []schemaField{
		{name: "certificate", typ: schema.TypeString, optional: true, sensitive: true},
		{name: "client_certificate_key", typ: schema.TypeString, optional: true, sensitive: true},
		{name: "insecure_skip_verify", typ: schema.TypeBool, optional: true, def: false},
		{name: "features_enabled", typ: schema.TypeSet, optional: true},
	})
}
