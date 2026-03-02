package cdktn

import (
	"encoding/json"
	"fmt"
	"sort"
)

// TerraformStack accumulates provider aliases and resources, then synthesizes
// deterministic Terraform JSON. This is a lightweight synthesis engine that
// mirrors CDKTN's construct tree → JSON pipeline. // CDKTN-029, CDKTN-030, CDKTN-031
type TerraformStack struct {
	terraformVersion string
	providerVersion  string
	providers        []ProviderBlock
	resources        []ResourceBlock
}

// ProviderBlock represents a single Terraform provider alias configuration.
type ProviderBlock struct {
	Alias    string
	Config   map[string]interface{}
	sortKey  string // deterministic ordering key
}

// ResourceBlock represents a single Terraform resource instance.
type ResourceBlock struct {
	Type      string
	Name      string
	Config    map[string]interface{}
	Provider  string
	DependsOn []string
	sortKey   string
}

// NewTerraformStack creates a stack with the given Terraform and provider version constraints.
func NewTerraformStack(terraformVersion, providerVersion string) *TerraformStack {
	if terraformVersion == "" {
		terraformVersion = DefaultTerraformVersion
	}
	return &TerraformStack{
		terraformVersion: terraformVersion,
		providerVersion:  providerVersion,
	}
}

// AddProvider appends a provider alias block.
func (s *TerraformStack) AddProvider(alias string, config map[string]interface{}) {
	s.providers = append(s.providers, ProviderBlock{
		Alias:   alias,
		Config:  config,
		sortKey: alias,
	})
}

// AddResource appends a resource block.
func (s *TerraformStack) AddResource(resType, name string, config map[string]interface{}, provider string, dependsOn []string) {
	s.resources = append(s.resources, ResourceBlock{
		Type:      resType,
		Name:      name,
		Config:    config,
		Provider:  provider,
		DependsOn: dependsOn,
		sortKey:   fmt.Sprintf("%s.%s", resType, name),
	})
}

// Synth produces deterministic Terraform JSON output. // CDKTN-029
func (s *TerraformStack) Synth() ([]byte, error) {
	out := make(map[string]interface{})

	// terraform block // CDKTN-030, CDKTN-031
	out["terraform"] = map[string]interface{}{
		"required_version": s.terraformVersion,
		"required_providers": map[string]interface{}{
			ProviderType: map[string]interface{}{
				"source":  ProviderSource,
				"version": s.providerVersion,
			},
		},
	}

	// provider block — sorted for determinism // CDKTN-029
	sortedProviders := make([]ProviderBlock, len(s.providers))
	copy(sortedProviders, s.providers)
	sort.Slice(sortedProviders, func(i, j int) bool {
		return sortedProviders[i].sortKey < sortedProviders[j].sortKey
	})

	providerList := make([]map[string]interface{}, 0, len(sortedProviders))
	for _, p := range sortedProviders {
		block := make(map[string]interface{}, len(p.Config)+1)
		block["alias"] = p.Alias
		for k, v := range p.Config {
			block[k] = v
		}
		providerList = append(providerList, block)
	}
	out["provider"] = map[string]interface{}{
		ProviderType: providerList,
	}

	// resource blocks — grouped by type, sorted within each type // CDKTN-029
	sortedResources := make([]ResourceBlock, len(s.resources))
	copy(sortedResources, s.resources)
	sort.Slice(sortedResources, func(i, j int) bool {
		return sortedResources[i].sortKey < sortedResources[j].sortKey
	})

	resourceMap := make(map[string]map[string]interface{})
	for _, r := range sortedResources {
		if _, ok := resourceMap[r.Type]; !ok {
			resourceMap[r.Type] = make(map[string]interface{})
		}
		block := make(map[string]interface{}, len(r.Config)+2)
		for k, v := range r.Config {
			block[k] = v
		}
		if r.Provider != "" {
			block["provider"] = r.Provider
		}
		if len(r.DependsOn) > 0 {
			sorted := make([]string, len(r.DependsOn))
			copy(sorted, r.DependsOn)
			sort.Strings(sorted)
			block["depends_on"] = sorted
		}
		resourceMap[r.Type][r.Name] = block
	}
	if len(resourceMap) > 0 {
		out["resource"] = resourceMap
	}

	return json.MarshalIndent(out, "", "  ")
}

// SynthToMap produces the same structure as Synth but as a Go map (useful for test assertions).
func (s *TerraformStack) SynthToMap() (map[string]interface{}, error) {
	data, err := s.Synth()
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
