package mongodb

import (
	"context"
	"fmt"
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
// GATE-006
const EnableEnvVar = "TERRAFORM_PROVIDER_MONGODB_ENABLE"

// ResourceRegistration pairs a schema.Resource with its maturity classification.
type ResourceRegistration struct {
	Name     string
	Factory  func() *schema.Resource
	Maturity ResourceMaturity
}

// AllResources returns the full registry of resources with their maturity.
// All resources are registered unconditionally. Experimental resources are
// gated at plan time via requireFeature in CustomizeDiff.
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
		// BAL-014
		{Name: "mongodb_balancer_config", Factory: resourceBalancerConfig, Maturity: ResourceExperimental},
		// CBAL-011
		{Name: "mongodb_collection_balancing", Factory: resourceCollectionBalancing, Maturity: ResourceExperimental},
		// FCV-013
		{Name: "mongodb_feature_compatibility_version", Factory: resourceFCV, Maturity: ResourceExperimental},
		// ZONE-012
		{Name: "mongodb_shard_zone", Factory: resourceShardZone, Maturity: ResourceExperimental},
		// ZONE-029
		{Name: "mongodb_zone_key_range", Factory: resourceZoneKeyRange, Maturity: ResourceExperimental},
	}
}

// parseEnableList reads the EnableEnvVar and returns the set of resource names
// that have been explicitly opted in.
func parseEnableList() map[string]bool {
	raw := os.Getenv(EnableEnvVar)
	if raw == "" {
		return nil
	}
	return parseCommaSeparated(raw)
}

// parseCommaSeparated splits a comma-separated string into a set of trimmed names.
func parseCommaSeparated(raw string) map[string]bool {
	enabled := make(map[string]bool)
	for _, name := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			enabled[trimmed] = true
		}
	}
	return enabled
}

// mergeEnableLists combines the env var enable list with the HCL features_enabled set.
func mergeEnableLists(envList, hclList map[string]bool) map[string]bool {
	merged := make(map[string]bool)
	for k := range envList {
		merged[k] = true
	}
	for k := range hclList {
		merged[k] = true
	}
	return merged
}

// BuildResourceMap constructs the ResourcesMap for the provider.
// All resources are registered unconditionally so that HCL references are
// valid regardless of env var state. Experimental resources are gated at
// plan time via requireFeature in CustomizeDiff.
func BuildResourceMap(registry []ResourceRegistration) map[string]*schema.Resource {
	resources := make(map[string]*schema.Resource)
	for _, reg := range registry {
		resources[reg.Name] = reg.Factory()
	}
	return resources
}

// requireFeature returns a CustomizeDiffFunc that blocks plan if the
// resource is not in the provider's enabled features set.
// GATE-005: blocks experimental resources not explicitly opted in.
func requireFeature(resourceName string) schema.CustomizeDiffFunc {
	return func(_ context.Context, _ *schema.ResourceDiff, meta interface{}) error {
		if meta == nil {
			// Provider not yet configured (e.g. terraform validate); allow.
			return nil
		}
		conf, ok := meta.(*MongoDatabaseConfiguration)
		if !ok || conf == nil {
			return nil
		}
		if conf.FeaturesEnabled[resourceName] {
			return nil
		}
		return fmt.Errorf(
			"resource %q is experimental and not enabled; add it to features_enabled "+
				"in the provider block or set %s=%s",
			resourceName, EnableEnvVar, resourceName,
		)
	}
}
