package mongodb

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"go.mongodb.org/mongo-driver/mongo"
)

// PartitionMemberOverrides splits member blocks into those whose host is in
// the replica set and those that are not, preserving block order. SHARD-012
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

// NextMemberID returns the highest _id in use plus one, or 0 for an empty
// set. Ids never come from list position, so a member removed by hand does
// not shift the others and a re-added host gets a fresh id. SHARD-014
func NextMemberID(members ConfigMembers) int {
	next := 0
	for _, m := range members {
		if m.ID >= next {
			next = m.ID + 1
		}
	}
	return next
}

// BuildConfigMember converts a member block into a ConfigMember. SHARD-015
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

// memberAddOps abstracts the server calls AddMembersSequentially makes so
// the sequencing can be unit tested.
type memberAddOps struct {
	GetConfig     func(ctx context.Context) (*RSConfig, error)
	SetConfig     func(ctx context.Context, cfg *RSConfig) error
	WaitReachable func(ctx context.Context, host string) error
}

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

// AddMembersSequentially adds one member per replSetReconfig, re-reading the
// config before each so the version and next _id reflect any server-side
// reconfiguration in between. MongoDB 4.4 rejects adding more than one voting
// member at a time. A member that does not become reachable is removed again
// so a mistyped host cannot linger in the config. SHARD-013, SHARD-016, SHARD-017
func AddMembersSequentially(ctx context.Context, overrides []MemberOverride, ops memberAddOps) error {
	for _, o := range overrides {
		cfg, err := ops.GetConfig(ctx)
		if err != nil {
			return fmt.Errorf("reading config before adding member %s: %w", o.Host, err)
		}
		member := BuildConfigMember(o, NextMemberID(cfg.Members))
		cfg.Members = append(cfg.Members, member)
		cfg.Version++

		tflog.Info(ctx, "adding replica set member", map[string]interface{}{
			"host": member.Host, "member_id": member.ID, "version": cfg.Version,
		})
		if err := ops.SetConfig(ctx, cfg); err != nil {
			return fmt.Errorf("adding member %s (_id %d): %w", member.Host, member.ID, err)
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

// removeMemberByHost undoes an add whose member never became reachable. The
// resource otherwise never removes members. SHARD-017, SHARD-019
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
	tflog.Warn(ctx, "removing unreachable replica set member", map[string]interface{}{
		"host": host, "version": cfg.Version,
	})
	return ops.SetConfig(ctx, cfg)
}

// memberReachable reports whether host has health 1 in any state, and what
// was seen. Reachability rather than initial sync is the bar: sync can take
// hours on a data-bearing set. SHARD-016
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
// or the timeout elapses. SHARD-016
func WaitForMemberReachable(ctx context.Context, client *mongo.Client, host string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var last string
		if status, err := GetReplSetStatus(ctx, client); err != nil {
			last = "replSetGetStatus: " + err.Error()
		} else if ok, seen := memberReachable(status, host); ok {
			tflog.Info(ctx, "replica set member is reachable", map[string]interface{}{
				"host": host, "status": seen,
			})
			return nil
		} else {
			last = seen
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
