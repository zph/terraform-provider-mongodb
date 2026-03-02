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
// CDKTN-015, CDKTN-016
func BuildShardConfig(stack *TerraformStack, rsName string, primaryAlias string, settings *ShardConfigSettings) {
	if settings == nil {
		settings = DefaultShardConfigSettings()
	}

	resName := fmt.Sprintf("%s_config", sanitizeName(rsName))
	config := map[string]interface{}{
		"shard_name":                rsName,
		"chaining_allowed":         settings.ChainingAllowed,
		"heartbeat_interval_millis": settings.HeartbeatIntervalMillis,
		"heartbeat_timeout_secs":    settings.HeartbeatTimeoutSecs,
		"election_timeout_millis":   settings.ElectionTimeoutMillis,
	}
	stack.AddResource(ResourceTypeShardConfig, resName, config, ProviderRef(primaryAlias), nil)
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
