package cdktn

import "fmt"

// ProviderAliasName generates a deterministic alias name for a provider instance.
// Pattern: <component_type>_<replica_set_name>_<member_index>
// For mongos (no RS name): mongos_<member_index> // CDKTN-004
func ProviderAliasName(ct ComponentType, rsName string, memberIdx int) string {
	prefix := ct.String()
	if rsName == "" {
		return fmt.Sprintf("%s_%d", prefix, memberIdx)
	}
	return fmt.Sprintf("%s_%s_%d", prefix, rsName, memberIdx)
}

// ProviderRef returns the Terraform provider reference string for use in
// resource "provider" attributes.
func ProviderRef(alias string) string {
	return fmt.Sprintf("%s.%s", ProviderType, alias)
}

// BuildProviderConfig assembles the provider configuration map for a single member.
// CDKTN-005, CDKTN-010, CDKTN-018-020, CDKTN-034, CDKTN-035, CDKTN-036, CDKTN-039, CDKTN-040
func BuildProviderConfig(member MemberConfig, clusterCreds CredentialSource, ssl *SSLConfig, proxy string, direct bool) map[string]interface{} {
	// CDKTN-010: member-level credentials override cluster-level
	creds := clusterCreds
	if member.Credentials != nil {
		creds = member.Credentials
	}

	port := member.Port
	if port == 0 {
		port = DefaultPort
	}

	config := map[string]interface{}{
		"host":          member.Host,
		"port":          fmt.Sprintf("%d", port), // Port is *string in provider schema
		"username":      creds.GetUsername(),
		"password":      creds.GetPassword(),
		"auth_database": DefaultAuthDatabase, // CDKTN-040
		"direct":        direct,              // CDKTN-035, CDKTN-036
		"retrywrites":   true,                // CDKTN-039
		"ssl":           false,
	}

	// CDKTN-018, CDKTN-019, CDKTN-020
	if ssl != nil {
		config["ssl"] = ssl.Enabled
		if ssl.Certificate != "" {
			config["certificate"] = ssl.Certificate
		}
		if ssl.InsecureSkipVerify {
			config["insecure_skip_verify"] = true
		}
	}

	// CDKTN-034
	if proxy != "" {
		config["proxy"] = proxy
	}

	return config
}

// BuildProviders creates provider aliases for all members of a component and adds
// them to the stack. Returns the list of alias names in member order.
func BuildProviders(stack *TerraformStack, ct ComponentType, rsName string, members []MemberConfig, creds CredentialSource, ssl *SSLConfig, proxy string) []string {
	return BuildProvidersWithOffset(stack, ct, rsName, members, creds, ssl, proxy, 0)
}

// BuildProvidersWithOffset is like BuildProviders but starts member indexing at offset.
// Used when multiple mongos groups need sequential alias numbering.
func BuildProvidersWithOffset(stack *TerraformStack, ct ComponentType, rsName string, members []MemberConfig, creds CredentialSource, ssl *SSLConfig, proxy string, offset int) []string {
	direct := ct != ComponentTypeMongos // CDKTN-035, CDKTN-036
	aliases := make([]string, 0, len(members))

	for i, member := range members {
		alias := ProviderAliasName(ct, rsName, offset+i)
		config := BuildProviderConfig(member, creds, ssl, proxy, direct)
		stack.AddProvider(alias, config)
		aliases = append(aliases, alias)
	}

	return aliases
}
