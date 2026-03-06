package mongodb

import (
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// ResourceMaturity classifies a resource's stability level.
type ResourceMaturity int

const (
	// ResourceMature indicates a stable, production-ready resource.
	ResourceMature ResourceMaturity = iota
	// ResourceExperimental indicates a resource that requires explicit opt-in.
	ResourceExperimental
)

// String returns the human-readable maturity label.
func (m ResourceMaturity) String() string {
	switch m {
	case ResourceMature:
		return "mature"
	case ResourceExperimental:
		return "experimental"
	default:
		return "unknown"
	}
}

// EnableEnvVar is the environment variable that opts in experimental resources.
// Value: comma-separated resource names (e.g. "mongodb_shard_config,mongodb_shard").
const EnableEnvVar = "TERRAFORM_PROVIDER_MONGODB_ENABLE"

// ResourceRegistration pairs a schema.Resource with its maturity classification.
type ResourceRegistration struct {
	Name     string
	Factory  func() *schema.Resource
	Maturity ResourceMaturity
}

// AllResources returns the full registry of resources with their maturity.
// Mature resources are always available. Experimental resources require
// opt-in via the TERRAFORM_PROVIDER_MONGODB_ENABLE env var.
func AllResources() []ResourceRegistration {
	return []ResourceRegistration{
		{Name: "mongodb_db_user", Factory: resourceDatabaseUser, Maturity: ResourceMature},
		{Name: "mongodb_db_role", Factory: resourceDatabaseRole, Maturity: ResourceMature},
		{Name: "mongodb_original_user", Factory: resourceOriginalUser, Maturity: ResourceMature},
		{Name: "mongodb_shard_config", Factory: resourceShardConfig, Maturity: ResourceExperimental},
		{Name: "mongodb_shard", Factory: resourceShard, Maturity: ResourceExperimental},
		// PROF-011
		{Name: "mongodb_profiler", Factory: resourceProfiler, Maturity: ResourceExperimental},
		// PARAM-011
		{Name: "mongodb_server_parameter", Factory: resourceServerParameter, Maturity: ResourceExperimental},
	}
}

// parseEnableList reads the EnableEnvVar and returns the set of resource names
// that have been explicitly opted in.
func parseEnableList() map[string]bool {
	raw := os.Getenv(EnableEnvVar)
	if raw == "" {
		return nil
	}
	enabled := make(map[string]bool)
	for _, name := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			enabled[trimmed] = true
		}
	}
	return enabled
}

// BuildResourceMap constructs the ResourcesMap for the provider by including
// all mature resources and only those experimental resources that appear in
// the enable list.
func BuildResourceMap(registry []ResourceRegistration, enableList map[string]bool) map[string]*schema.Resource {
	resources := make(map[string]*schema.Resource)
	for _, reg := range registry {
		switch reg.Maturity {
		case ResourceMature:
			resources[reg.Name] = reg.Factory()
		case ResourceExperimental:
			if enableList[reg.Name] {
				resources[reg.Name] = reg.Factory()
			}
		}
	}
	return resources
}
