package cdktn

import "fmt"

// ResourceNames holds the naming convention for generated Terraform resource identifiers.
const (
	ResourceTypeDBRole      = "mongodb_db_role"
	ResourceTypeDBUser      = "mongodb_db_user"
	ResourceTypeShardConfig = "mongodb_shard_config"
)

// BuildRoles adds mongodb_db_role resources for each role on each alias.
// Returns the list of role resource references per alias (for depends_on). // CDKTN-012
func BuildRoles(stack *TerraformStack, aliases []string, roles []RoleConfig) map[string][]string {
	roleDeps := make(map[string][]string, len(aliases))
	for _, alias := range aliases {
		var deps []string
		for _, role := range roles {
			resName := fmt.Sprintf("%s_role_%s", alias, sanitizeName(role.Name))
			config := buildRoleConfig(role)
			ref := fmt.Sprintf("%s.%s", ResourceTypeDBRole, resName)
			stack.AddResource(ResourceTypeDBRole, resName, config, ProviderRef(alias), nil)
			deps = append(deps, ref)
		}
		roleDeps[alias] = deps
	}
	return roleDeps
}

// BuildUsers adds mongodb_db_user resources for each user on each alias,
// with depends_on referencing all roles on that alias. // CDKTN-011, CDKTN-013, CDKTN-014
func BuildUsers(stack *TerraformStack, aliases []string, users []UserConfig, roleDeps map[string][]string) {
	for _, alias := range aliases {
		deps := roleDeps[alias]
		for _, user := range users {
			resName := fmt.Sprintf("%s_user_%s", alias, sanitizeName(user.Username))
			config := buildUserConfig(user)
			stack.AddResource(ResourceTypeDBUser, resName, config, ProviderRef(alias), deps)
		}
	}
}

// BuildShardConfig adds a mongodb_shard_config resource targeting the first member's alias.
// CDKTN-015, CDKTN-016, CDKTN-051
func BuildShardConfig(stack *TerraformStack, rsName string, primaryAlias string, settings *ShardConfigSettings) {
	if settings == nil {
		settings = DefaultShardConfigSettings()
	}

	resName := fmt.Sprintf("%s_config", sanitizeName(rsName))
	config := map[string]interface{}{
		"shard_name":                rsName,
		"chaining_allowed":          settings.ChainingAllowed,
		"heartbeat_interval_millis": settings.HeartbeatIntervalMillis,
		"heartbeat_timeout_secs":    settings.HeartbeatTimeoutSecs,
		"election_timeout_millis":   settings.ElectionTimeoutMillis,
	}

	// CDKTN-051: Per-member overrides
	if len(settings.Members) > 0 {
		members := make([]map[string]interface{}, 0, len(settings.Members))
		for _, mo := range settings.Members {
			entry := map[string]interface{}{
				"host":                 mo.Host,
				"priority":             mo.Priority,
				"votes":                mo.Votes,
				"hidden":               mo.Hidden,
				"arbiter_only":         mo.ArbiterOnly,
				"build_indexes":        mo.BuildIndexes,
				"secondary_delay_secs": mo.SecondaryDelaySecs,
			}
			if mo.Tags != nil {
				entry["tags"] = mo.Tags
			}
			members = append(members, entry)
		}
		config["member"] = members
	}

	stack.AddResource(ResourceTypeShardConfig, resName, config, ProviderRef(primaryAlias), nil)
}

// BuildOriginalUsers adds mongodb_original_user resources for bootstrap admin users.
// Each resource has inline connection params (no provider alias ref). // CDKTN-052
func BuildOriginalUsers(stack *TerraformStack, prefix string, users []OriginalUserConfig) {
	for _, user := range users {
		resName := fmt.Sprintf("%s_origuser_%s", prefix, sanitizeName(user.Username))

		port := user.Port
		if port == 0 {
			port = DefaultPort
		}

		db := user.AuthDatabase
		if db == "" {
			db = DefaultAuthDatabase
		}

		config := map[string]interface{}{
			"host":          user.Host,
			"port":          fmt.Sprintf("%d", port),
			"username":      user.Username,
			"password":      user.Password,
			"auth_database": db,
		}

		if len(user.Roles) > 0 {
			roles := make([]map[string]interface{}, 0, len(user.Roles))
			for _, r := range user.Roles {
				roles = append(roles, map[string]interface{}{
					"role": r.Role,
					"db":   r.DB,
				})
			}
			config["role"] = roles
		}

		if user.ReplicaSet != "" {
			config["replica_set"] = user.ReplicaSet
		}

		if user.SSL != nil {
			config["ssl"] = user.SSL.Enabled
			if user.SSL.Certificate != "" {
				config["certificate"] = user.SSL.Certificate
			}
			if user.SSL.InsecureSkipVerify {
				config["insecure_skip_verify"] = true
			}
		}

		// No provider ref — original_user uses inline connection params
		stack.AddResource(ResourceTypeOriginalUser, resName, config, "", nil)
	}
}

// BuildShardRegistrations adds mongodb_shard resources via the mongos provider alias.
// Each shard is registered with its RS name and member hosts. // CLUS-002
func BuildShardRegistrations(stack *TerraformStack, mongosAlias string, shards []shardRegistrationEntry, deps []string) []string {
	var refs []string
	for _, s := range shards {
		resName := fmt.Sprintf("shard_%s", sanitizeName(s.RSName))
		hosts := make([]string, len(s.Hosts))
		copy(hosts, s.Hosts)
		config := map[string]interface{}{
			"shard_name":          s.RSName,
			"hosts":               hosts,
			"remove_timeout_secs": DefaultRemoveTimeoutSecs,
		}
		ref := fmt.Sprintf("%s.%s", ResourceTypeShard, resName)
		stack.AddResource(ResourceTypeShard, resName, config, ProviderRef(mongosAlias), deps)
		refs = append(refs, ref)
	}
	return refs
}

// shardRegistrationEntry holds the info needed to register a shard.
type shardRegistrationEntry struct {
	RSName string
	Hosts  []string
}

// BuildBalancerConfig adds a mongodb_balancer_config resource via the mongos provider. // BAL-001
func BuildBalancerConfig(stack *TerraformStack, mongosAlias string, cfg *BalancerConfig, deps []string) {
	resName := "balancer"
	config := map[string]interface{}{
		"enabled": cfg.Enabled,
	}
	if cfg.ActiveWindowStart != "" {
		config["active_window_start"] = cfg.ActiveWindowStart
	}
	if cfg.ActiveWindowStop != "" {
		config["active_window_stop"] = cfg.ActiveWindowStop
	}
	if cfg.ChunkSizeMB > 0 {
		config["chunk_size_mb"] = cfg.ChunkSizeMB
	}
	if cfg.SecondaryThrottle != "" {
		config["secondary_throttle"] = cfg.SecondaryThrottle
	}
	if cfg.WaitForDelete != nil {
		config["wait_for_delete"] = *cfg.WaitForDelete
	}
	stack.AddResource(ResourceTypeBalancerConfig, resName, config, ProviderRef(mongosAlias), deps)
}

// BuildShardZones adds mongodb_shard_zone resources via the mongos provider. // ZONE-002
func BuildShardZones(stack *TerraformStack, mongosAlias string, zones []ShardZoneConfig, deps []string) []string {
	var refs []string
	for _, z := range zones {
		resName := fmt.Sprintf("%s_%s", sanitizeName(z.ShardName), sanitizeName(z.Zone))
		config := map[string]interface{}{
			"shard_name": z.ShardName,
			"zone":       z.Zone,
		}
		ref := fmt.Sprintf("%s.%s", ResourceTypeShardZone, resName)
		stack.AddResource(ResourceTypeShardZone, resName, config, ProviderRef(mongosAlias), deps)
		refs = append(refs, ref)
	}
	return refs
}

// BuildZoneKeyRanges adds mongodb_zone_key_range resources via the mongos provider. // ZONE-017
func BuildZoneKeyRanges(stack *TerraformStack, mongosAlias string, ranges []ZoneKeyRangeConfig, deps []string) {
	for i, r := range ranges {
		resName := fmt.Sprintf("%s_%s_%d", sanitizeName(r.Namespace), sanitizeName(r.Zone), i)
		config := map[string]interface{}{
			"namespace": r.Namespace,
			"zone":      r.Zone,
			"min":       r.Min,
			"max":       r.Max,
		}
		stack.AddResource(ResourceTypeZoneKeyRange, resName, config, ProviderRef(mongosAlias), deps)
	}
}

// BuildCollectionBalancing adds mongodb_collection_balancing resources via the mongos provider. // CBAL-001
func BuildCollectionBalancing(stack *TerraformStack, mongosAlias string, configs []CollectionBalancingConfig, deps []string) {
	for _, c := range configs {
		resName := fmt.Sprintf("colbal_%s", sanitizeName(c.Namespace))
		config := map[string]interface{}{
			"namespace": c.Namespace,
			"enabled":   c.Enabled,
		}
		if c.ChunkSizeMB > 0 {
			config["chunk_size_mb"] = c.ChunkSizeMB
		}
		stack.AddResource(ResourceTypeCollBalancing, resName, config, ProviderRef(mongosAlias), deps)
	}
}

// BuildProfilers adds mongodb_profiler resources for each database on each alias. // PROF-001
func BuildProfilers(stack *TerraformStack, aliases []string, profilers []ProfilerConfig) {
	for _, alias := range aliases {
		for _, p := range profilers {
			resName := fmt.Sprintf("%s_profiler_%s", alias, sanitizeName(p.Database))
			config := map[string]interface{}{
				"database":  p.Database,
				"level":     p.Level,
				"slowms":    p.SlowMs,
				"ratelimit": p.RateLimit,
			}
			stack.AddResource(ResourceTypeProfiler, resName, config, ProviderRef(alias), nil)
		}
	}
}

// BuildServerParameters adds mongodb_server_parameter resources for each param on each alias. // PARAM-001
func BuildServerParameters(stack *TerraformStack, aliases []string, params []ServerParameterConfig) {
	for _, alias := range aliases {
		for _, p := range params {
			resName := fmt.Sprintf("%s_param_%s", alias, sanitizeName(p.Parameter))
			config := map[string]interface{}{
				"parameter": p.Parameter,
				"value":     p.Value,
			}
			stack.AddResource(ResourceTypeServerParameter, resName, config, ProviderRef(alias), nil)
		}
	}
}

// BuildFCV adds a mongodb_feature_compatibility_version resource on the given alias. // FCV-001
func BuildFCV(stack *TerraformStack, alias string, fcv *FCVConfig) {
	resName := fmt.Sprintf("%s_fcv", alias)
	config := map[string]interface{}{
		"version":     fcv.Version,
		"danger_mode": fcv.DangerMode,
	}
	stack.AddResource(ResourceTypeFCV, resName, config, ProviderRef(alias), nil)
}

func buildRoleConfig(role RoleConfig) map[string]interface{} {
	config := map[string]interface{}{
		"name":     role.Name,
		"database": role.Database,
	}

	if len(role.Privileges) > 0 {
		privs := make([]map[string]interface{}, 0, len(role.Privileges))
		for _, p := range role.Privileges {
			priv := map[string]interface{}{
				"actions": p.Actions,
			}
			if p.Cluster {
				priv["cluster"] = true
			}
			if p.DB != "" {
				priv["db"] = p.DB
			}
			if p.Collection != "" {
				priv["collection"] = p.Collection
			}
			privs = append(privs, priv)
		}
		config["privilege"] = privs
	}

	if len(role.InheritedRoles) > 0 {
		inherited := make([]map[string]interface{}, 0, len(role.InheritedRoles))
		for _, ir := range role.InheritedRoles {
			inherited = append(inherited, map[string]interface{}{
				"role": ir.Role,
				"db":   ir.DB,
			})
		}
		config["inherited_role"] = inherited
	}

	return config
}

// CDKTN-014
func buildUserConfig(user UserConfig) map[string]interface{} {
	db := user.Database
	if db == "" {
		db = DefaultAuthDatabase
	}

	config := map[string]interface{}{
		"name":          user.Username,
		"password":      user.Password,
		"auth_database": db,
	}

	if len(user.Roles) > 0 {
		roles := make([]map[string]interface{}, 0, len(user.Roles))
		for _, r := range user.Roles {
			roles = append(roles, map[string]interface{}{
				"role": r.Role,
				"db":   r.DB,
			})
		}
		config["role"] = roles
	}

	return config
}

// sanitizeName converts names for use as Terraform resource identifiers.
func sanitizeName(name string) string {
	result := make([]byte, 0, len(name))
	for _, c := range name {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'):
			result = append(result, byte(c))
		default:
			result = append(result, '_')
		}
	}
	return string(result)
}
