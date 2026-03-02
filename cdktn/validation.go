package cdktn

import (
	"fmt"
	"log"
)

// ValidateReplicaSetMembers checks member count bounds for a component type.
// CDKTN-021, CDKTN-022, CDKTN-026
func ValidateReplicaSetMembers(ct ComponentType, members []MemberConfig) error {
	count := len(members)

	// Mongos has no minimum
	if ct != ComponentTypeMongos && count < MinReplicaSetMembers {
		return fmt.Errorf("%s requires minimum %d members, got %d", ct, MinReplicaSetMembers, count)
	}

	if count > MaxMembers {
		return fmt.Errorf("%s exceeds maximum %d members, got %d", ct, MaxMembers, count)
	}

	return nil
}

// ValidateDuplicateHostPort returns an error if any two members share host:port. // CDKTN-025
func ValidateDuplicateHostPort(members []MemberConfig) error {
	seen := make(map[string]struct{}, len(members))
	for _, m := range members {
		hp := m.HostPort()
		if _, ok := seen[hp]; ok {
			return fmt.Errorf("duplicate member host:port %s", hp)
		}
		seen[hp] = struct{}{}
	}
	return nil
}

// ValidateDuplicateRSNames returns an error if any replica set names repeat. // CDKTN-028
func ValidateDuplicateRSNames(names []string) error {
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		if _, ok := seen[n]; ok {
			return fmt.Errorf("duplicate replica set name %q", n)
		}
		seen[n] = struct{}{}
	}
	return nil
}

// WarnVotingMemberCount logs a warning if member count exceeds MaxVotingMembers.
// MongoDB allows non-voting members beyond 7, so this is a warning not an error. // CDKTN-027
func WarnVotingMemberCount(logger *log.Logger, members []MemberConfig) {
	if len(members) > MaxVotingMembers {
		logger.Printf("WARNING: %d members exceeds MongoDB voting member limit of %d; additional members MUST be non-voting",
			len(members), MaxVotingMembers)
	}
}

// ValidateClusterMongos returns an error if no mongos instances are defined. // CDKTN-023
func ValidateClusterMongos(mongos []MongosConfig) error {
	if len(mongos) == 0 {
		return fmt.Errorf("at least one mongos instance is required")
	}
	return nil
}

// ValidateClusterShards returns an error if no shards are defined. // CDKTN-024
func ValidateClusterShards(shards []ShardConfig) error {
	if len(shards) == 0 {
		return fmt.Errorf("at least one shard is required")
	}
	return nil
}

// ValidateReplicaSetName returns an error if the RS name is empty. // CDKTN-033
func ValidateReplicaSetName(name string) error {
	if name == "" {
		return fmt.Errorf("replica set name MUST not be empty")
	}
	return nil
}
