package mongodb

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"go.mongodb.org/mongo-driver/mongo"
)

// PartitionMemberOverrides splits the Terraform member blocks into the ones
// whose host is already in the replica set configuration and the ones that
// are not. Order within each group follows the member blocks.
// SHARD-003, SHARD-012
func PartitionMemberOverrides(rsMembers ConfigMembers, overrides []MemberOverride) (existing, missing []MemberOverride) {
	hosts := make(map[string]bool, len(rsMembers))
	for _, m := range rsMembers {
		hosts[m.Host] = true
	}
	for _, o := range overrides {
		if hosts[o.Host] {
			existing = append(existing, o)
		} else {
			missing = append(missing, o)
		}
	}
	return existing, missing
}

// NextMemberID returns the _id for a member appended to members: one more
// than the highest _id in use, or 0 for an empty configuration. The position
// of the block in the Terraform list is never used, so a member removed by
// hand does not shift the ids of the others, and a host that is re-added
// does not reuse an id the server may still associate with the old member.
// SHARD-014
func NextMemberID(members ConfigMembers) int {
	next := 0
	for _, m := range members {
		if m.ID >= next {
			next = m.ID + 1
		}
	}
	return next
}

// BuildConfigMember converts a Terraform member block into a ConfigMember
// with the given _id, carrying over every per-member field.
// SHARD-015
func BuildConfigMember(o MemberOverride, id int) ConfigMember {
	m := ConfigMember{
		ID:           id,
		Host:         o.Host,
		Priority:     o.Priority,
		Votes:        intPtr(o.Votes),
		Hidden:       boolPtr(o.Hidden),
		ArbiterOnly:  boolPtr(o.ArbiterOnly),
		BuildIndexes: boolPtr(o.BuildIndexes),
	}
	if o.Tags != nil {
		m.Tags = ReplsetTags(o.Tags)
	}
	return m
}

// memberAddOps are the server interactions AddMembersSequentially needs,
// injected so the sequencing can be tested without a replica set.
type memberAddOps struct {
	GetConfig     func(ctx context.Context) (*RSConfig, error)
	SetConfig     func(ctx context.Context, cfg *RSConfig) error
	WaitReachable func(ctx context.Context, host string) error
}

// memberAddOpsForClient wires AddMembersSequentially to a live primary.
func memberAddOpsForClient(client *mongo.Client, timeout time.Duration) memberAddOps {
	return memberAddOps{
		GetConfig: func(ctx context.Context) (*RSConfig, error) {
			return GetReplSetConfig(ctx, client)
		},
		SetConfig: func(ctx context.Context, cfg *RSConfig) error {
			return SetReplSetConfigWithRetry(ctx, client, cfg, timeout)
		},
		WaitReachable: func(ctx context.Context, host string) error {
			return WaitForMemberReachable(ctx, client, host, timeout)
		},
	}
}

// AddMembersSequentially adds each override to the replica set with its own
// replSetReconfig, in block order. The configuration is re-read before every
// add so the version and the next _id reflect anything the server changed in
// between, such as the automatic reconfiguration that clears newlyAdded on
// MongoDB 5.0 and later. MongoDB 4.4 rejects a non-force reconfig that adds
// more than one voting member, and one member per reconfig is correct on
// every version.
//
// After each reconfig it waits for the primary to report the new member as
// reachable. If that does not happen within the timeout, the member is
// removed again (best effort) so a mistyped host does not stay in the
// configuration as a permanently unreachable member, and the add fails.
// SHARD-013, SHARD-016, SHARD-017
func AddMembersSequentially(ctx context.Context, overrides []MemberOverride, ops memberAddOps) error {
	for _, o := range overrides {
		cfg, err := ops.GetConfig(ctx)
		if err != nil {
			return fmt.Errorf("reading replica set config before adding member %s: %w", o.Host, err)
		}
		member := BuildConfigMember(o, NextMemberID(cfg.Members))
		cfg.Members = append(cfg.Members, member)
		cfg.Version++

		tflog.Info(ctx, "adding replica set member", map[string]interface{}{
			"host":      member.Host,
			"member_id": member.ID,
			"version":   cfg.Version,
		})
		if err := ops.SetConfig(ctx, cfg); err != nil {
			return fmt.Errorf("adding member %s (_id %d) to replica set: %w", member.Host, member.ID, err)
		}

		if err := ops.WaitReachable(ctx, member.Host); err != nil {
			if rbErr := removeMemberByHost(ctx, member.Host, ops); rbErr != nil {
				return fmt.Errorf("member %s was added but did not become reachable: %w; removing it again also failed: %v",
					member.Host, err, rbErr)
			}
			return fmt.Errorf("member %s was added but did not become reachable, so it was removed again: %w",
				member.Host, err)
		}
	}
	return nil
}

// removeMemberByHost reconfigures the set without host. It is only used to
// undo an add whose member never became reachable (SHARD-017); the resource
// otherwise never removes members (SHARD-019).
func removeMemberByHost(ctx context.Context, host string, ops memberAddOps) error {
	cfg, err := ops.GetConfig(ctx)
	if err != nil {
		return err
	}
	kept := make(ConfigMembers, 0, len(cfg.Members))
	for _, m := range cfg.Members {
		if m.Host != host {
			kept = append(kept, m)
		}
	}
	if len(kept) == len(cfg.Members) {
		return nil
	}
	cfg.Members = kept
	cfg.Version++
	tflog.Warn(ctx, "removing replica set member that did not become reachable", map[string]interface{}{
		"host":    host,
		"version": cfg.Version,
	})
	return ops.SetConfig(ctx, cfg)
}

// memberReachable reports whether status lists host with health up, plus a
// description of what it saw for the timeout message. A member in STARTUP2
// or RECOVERING counts: the check is reachability, not initial sync, which
// can take hours on a data-bearing set and is not something an apply should
// block on. SHARD-016
func memberReachable(status *ReplSetStatus, host string) (bool, string) {
	for _, m := range status.Members {
		if m.Name != host {
			continue
		}
		state := m.StateStr
		if state == "" {
			state = MemberStateStrings[m.State]
		}
		return m.Health == MemberHealthUp, fmt.Sprintf("health=%d state=%s", m.Health, state)
	}
	return false, "not listed in replSetGetStatus"
}

// WaitForMemberReachable polls replSetGetStatus until host reports health 1
// or the timeout is reached. SHARD-016
func WaitForMemberReachable(ctx context.Context, client *mongo.Client, host string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for {
		status, err := GetReplSetStatus(ctx, client)
		if err == nil {
			ok, seen := memberReachable(status, host)
			if ok {
				tflog.Info(ctx, "replica set member is reachable", map[string]interface{}{
					"host":   host,
					"status": seen,
				})
				return nil
			}
			last = seen
		} else {
			last = "replSetGetStatus: " + err.Error()
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("member %s did not become reachable within %s (last observed: %s)", host, timeout, last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(initPollInterval):
		}
	}
}
