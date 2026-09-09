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

// HoldPromotions separates the blocks that raise a live member's votes from
// the rest. Every block comes back in held, but a promoting block has its
// votes and priority pinned to the live values so the settings reconfig
// leaves them alone; the block itself is returned in promote for
// PromoteMembersSequentially, which applies the new values only once the
// member is SECONDARY. Before 5.0 a member counts toward majority as soon as
// it votes, synced or not. SHARD-023
func HoldPromotions(rsMembers ConfigMembers, overrides []MemberOverride) (held, promote []MemberOverride) {
	live := make(map[string]ConfigMember, len(rsMembers))
	for _, m := range rsMembers {
		live[m.Host] = m
	}
	held = make([]MemberOverride, 0, len(overrides))
	for _, o := range overrides {
		m, ok := live[o.Host]
		if !ok || o.Votes <= derefInt(m.Votes) {
			held = append(held, o)
			continue
		}
		pinned := o
		pinned.Votes = derefInt(m.Votes)
		pinned.Priority = m.Priority
		held = append(held, pinned)
		promote = append(promote, o)
	}
	return held, promote
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

// stagedMember is the form a new member is first added in: every configured
// field except votes and priority, which start at 0 so the member does not
// count toward majority until it has finished initial sync. Arbiters must
// vote, so they are added in their final form. SHARD-022
func stagedMember(o MemberOverride, id int) ConfigMember {
	if o.ArbiterOnly {
		return BuildConfigMember(o, id)
	}
	staged := o
	staged.Votes = 0
	staged.Priority = 0
	return BuildConfigMember(staged, id)
}

// needsPromotion reports whether the block's votes or priority differ from
// the staged 0/0 and so need the second reconfig.
func needsPromotion(o MemberOverride) bool {
	return !o.ArbiterOnly && (o.Votes != 0 || o.Priority != 0)
}

// addTargetVerdict is what a candidate host's own replSetGetStatus says about
// adding it. SHARD-027
type addTargetVerdict int

const (
	// addTargetOK is a fresh mongod started with --replSet, or one removed
	// from a set earlier: adding it is what the block asks for.
	addTargetOK addTargetVerdict = iota
	// addTargetRefused is a host that is already an active member of this set
	// under another name, belongs to a different set, or does not run
	// replication at all.
	addTargetRefused
	// addTargetUnknown is a host that could not be inspected, because it is
	// unreachable from here or accepts none of the credentials tried. The add
	// proceeds and the wait after it decides (SHARD-016, SHARD-017).
	addTargetUnknown
)

// CheckAddTarget decides from replSetGetStatus run against a candidate host
// whether it may be added to set. The case this exists for is a host that
// is already an active member under another name, such as an FQDN for a
// member configured by its short name: the reconfig would give that node two
// entries, the node would remove itself until an acceptable configuration
// arrives, and in a two-voter set the primary would lose its majority with no
// rollback able to run. A fresh node has no users yet, so an authenticated
// probe fails there and reads as unknown; a live member has replicated users,
// answers, and is refused. SHARD-027
func CheckAddTarget(set string, status *ReplSetStatus, statusErr error) (addTargetVerdict, string) {
	if statusErr != nil {
		switch {
		case IsNotYetInitialized(statusErr):
			return addTargetOK, "not yet initialized"
		case IsInvalidReplicaSetConfig(statusErr):
			return addTargetOK, "removed from a replica set earlier"
		case IsNoReplicationEnabled(statusErr):
			return addTargetRefused, "mongod is not running with --replSet"
		default:
			return addTargetUnknown, statusErr.Error()
		}
	}
	if status.Set != set {
		return addTargetRefused, fmt.Sprintf("it is a member of replica set %q", status.Set)
	}
	if status.MyState == MemberStateRemoved {
		return addTargetOK, "removed from this replica set earlier"
	}
	as := "under another name"
	if self := status.GetSelf(); self != nil {
		as = "as " + self.Name
	}
	return addTargetRefused, fmt.Sprintf("it is already a member of this replica set %s (state %s); use that host in the member block",
		as, MemberStateStrings[status.MyState])
}

// memberProbe runs replSetGetStatus against a host over a direct connection.
type memberProbe func(ctx context.Context, host string) (*ReplSetStatus, error)

// PreflightAddTargets probes every host about to be added and refuses the
// first one that must not be, before any reconfig is sent. Hosts that cannot
// be inspected are logged and allowed through. SHARD-027
func PreflightAddTargets(ctx context.Context, set string, overrides []MemberOverride, probe memberProbe) error {
	for _, o := range overrides {
		status, err := probe(ctx, o.Host)
		verdict, reason := CheckAddTarget(set, status, err)
		fields := map[string]interface{}{"host": o.Host, "set": set, "reason": reason}
		switch verdict {
		case addTargetRefused:
			return fmt.Errorf("member %s cannot be added to replica set %s: %s", o.Host, set, reason)
		case addTargetUnknown:
			tflog.Warn(ctx, "could not inspect member before adding it; relying on the wait after the add", fields)
		default:
			tflog.Debug(ctx, "member inspected before add", fields)
		}
	}
	return nil
}

// memberWaitTarget is the state a member must report before the resource
// moves on. SHARD-016
type memberWaitTarget int

const (
	// waitReachable is health 1 in any state: enough for a member that stays
	// non-voting, since it never counts toward majority.
	waitReachable memberWaitTarget = iota
	// waitSecondary is health 1 and SECONDARY (or PRIMARY): required before a
	// member is given votes.
	waitSecondary
	// waitArbiter is health 1 and ARBITER.
	waitArbiter
)

func (t memberWaitTarget) String() string {
	switch t {
	case waitSecondary:
		return "SECONDARY"
	case waitArbiter:
		return "ARBITER"
	default:
		return "reachable"
	}
}

func waitTargetFor(o MemberOverride) memberWaitTarget {
	switch {
	case o.ArbiterOnly:
		return waitArbiter
	case needsPromotion(o):
		return waitSecondary
	default:
		return waitReachable
	}
}

// memberObservation is what one replSetGetStatus says about a host.
type memberObservation struct {
	Reachable bool   // health 1
	Met       bool   // target reached
	Seen      string // for logs and errors
}

// observeMember reads host's row of status against target. SHARD-016
func observeMember(status *ReplSetStatus, host string, target memberWaitTarget) memberObservation {
	for _, m := range status.Members {
		if m.Name != host {
			continue
		}
		state := m.StateStr
		if state == "" {
			state = MemberStateStrings[m.State]
		}
		obs := memberObservation{
			Reachable: m.Health == MemberHealthUp,
			Seen:      fmt.Sprintf("health=%d state=%s", m.Health, state),
		}
		if !obs.Reachable {
			return obs
		}
		switch target {
		case waitSecondary:
			obs.Met = m.State == MemberStateSecondary || m.State == MemberStatePrimary
		case waitArbiter:
			obs.Met = m.State == MemberStateArbiter
		default:
			obs.Met = true
		}
		return obs
	}
	return memberObservation{Seen: "not listed in replSetGetStatus"}
}

// memberWaitResult is how a wait ended. A timeout is not an error here: the
// caller decides what to do with a member that was never reachable versus one
// that is reachable but still syncing. Observed distinguishes a member the
// primary reported down from one whose status could not be read at all: only
// the former justifies removing it (SHARD-017).
type memberWaitResult struct {
	Met           bool
	EverReachable bool
	Observed      bool // at least one replSetGetStatus succeeded
	Last          string
}

// WaitForMemberState polls getStatus until host meets target or timeout
// elapses. The returned error is only ever the context's. SHARD-016
func WaitForMemberState(ctx context.Context, getStatus func(context.Context) (*ReplSetStatus, error), host string, target memberWaitTarget, timeout, poll time.Duration) (memberWaitResult, error) {
	deadline := time.Now().Add(timeout)
	var res memberWaitResult
	for {
		if status, err := getStatus(ctx); err != nil {
			res.Last = "replSetGetStatus: " + err.Error()
		} else {
			obs := observeMember(status, host, target)
			res.Observed = true
			res.Last = obs.Seen
			res.EverReachable = res.EverReachable || obs.Reachable
			if obs.Met {
				res.Met = true
				tflog.Info(ctx, "replica set member reached target state", map[string]interface{}{
					"host": host, "target": target.String(), "status": obs.Seen,
				})
				return res, nil
			}
		}
		if time.Now().After(deadline) {
			return res, nil
		}
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-time.After(poll):
		}
	}
}

// rollbackGrace bounds the removal of a never-reachable member when the apply
// itself has been cancelled. SHARD-017
const rollbackGrace = 30 * time.Second

// memberPending is a member that is in the set, reachable, and due the votes
// and priority its block asks for, but had not reached the required state
// when the wait ended. It stays a non-voter with priority 0; the apply
// succeeds with a warning and the next apply performs the promotion once the
// member is SECONDARY. Initial sync of a data-bearing set routinely outlasts
// init_timeout_secs, so this is the expected outcome, not a failure.
// SHARD-017, SHARD-023
type memberPending struct {
	Host     string
	Target   memberWaitTarget
	Last     string
	Votes    int
	Priority float64
	Timeout  time.Duration
}

// Summary is the one-line warning shown in the apply output.
func (p memberPending) Summary() string {
	return fmt.Sprintf("Replica set member %s is not yet %s", p.Host, p.Target)
}

// Detail explains what happened and what the next apply does.
func (p memberPending) Detail() string {
	return fmt.Sprintf("%s is reachable but was not %s within %s (last observed: %s). "+
		"It stays in the replica set as a non-voter with priority 0, which the plan now shows as a pending change. "+
		"Apply again once it is %s to give it votes %d and priority %v, or raise init_timeout_secs.",
		p.Host, p.Target, p.Timeout, p.Last, p.Target, p.Votes, p.Priority)
}

// memberAddOps abstracts the server calls the add and promote sequences make
// so they can be unit tested.
type memberAddOps struct {
	GetConfig func(ctx context.Context) (*RSConfig, error)
	SetConfig func(ctx context.Context, cfg *RSConfig) error
	Wait      func(ctx context.Context, host string, target memberWaitTarget) (memberWaitResult, error)
	Timeout   time.Duration
}

func memberAddOpsForClient(client *mongo.Client, timeout time.Duration) memberAddOps {
	getStatus := func(ctx context.Context) (*ReplSetStatus, error) {
		return GetReplSetStatus(ctx, client)
	}
	return memberAddOps{
		GetConfig: func(ctx context.Context) (*RSConfig, error) {
			return GetReplSetConfig(ctx, client)
		},
		SetConfig: func(ctx context.Context, cfg *RSConfig) error {
			return SetReplSetConfigWithRetry(ctx, client, cfg, timeout)
		},
		Wait: func(ctx context.Context, host string, target memberWaitTarget) (memberWaitResult, error) {
			return WaitForMemberState(ctx, getStatus, host, target, timeout, initPollInterval)
		},
		Timeout: timeout,
	}
}

// AddMembersSequentially adds one member per replSetReconfig, in block order,
// re-reading the config before each so the version and next _id follow the
// server. A member that will vote is added with votes 0 and priority 0 and
// promoted by a second reconfig once it is SECONDARY, so it never counts
// toward majority while syncing (SHARD-022). Arbiters are added in final
// form. MongoDB 4.4 rejects adding more than one voting member at a time. A
// member that the primary reports down for the whole wait is removed again
// (SHARD-017); one that is reachable but not yet SECONDARY when the timeout
// elapses stays as a non-voter and is returned in pending, and the loop goes
// on with the next block because a non-voter does not affect the majority.
// SHARD-013, SHARD-016
func AddMembersSequentially(ctx context.Context, overrides []MemberOverride, ops memberAddOps) ([]memberPending, error) {
	var pending []memberPending
	for _, o := range overrides {
		cfg, err := ops.GetConfig(ctx)
		if err != nil {
			return pending, fmt.Errorf("reading config before adding member %s: %w", o.Host, err)
		}
		member := stagedMember(o, NextMemberID(cfg.Members))
		cfg.Members = append(cfg.Members, member)
		cfg.Version++

		tflog.Info(ctx, "adding replica set member", map[string]interface{}{
			"host": member.Host, "member_id": member.ID, "version": cfg.Version,
			"votes": derefInt(member.Votes), "priority": member.Priority,
		})
		if err := ops.SetConfig(ctx, cfg); err != nil {
			// A failed reconfig normally installs nothing; the removal covers
			// the case where the config went in but the command still failed.
			removed, rbErr := removeMemberByHost(ctx, member.Host, ops)
			switch {
			case rbErr != nil:
				return pending, fmt.Errorf("adding member %s (_id %d): %w; checking whether it was installed also failed: %v",
					member.Host, member.ID, err, rbErr)
			case removed:
				return pending, fmt.Errorf("adding member %s (_id %d) failed after the config was installed, so it was removed again: %w",
					member.Host, member.ID, err)
			default:
				return pending, fmt.Errorf("adding member %s (_id %d): %w", member.Host, member.ID, err)
			}
		}

		p, err := settleMember(ctx, o, ops, true)
		if err != nil {
			return pending, err
		}
		if p != nil {
			pending = append(pending, *p)
		}
	}
	return pending, nil
}

// ReconcileMembers runs the promotions, then the adds, and returns every
// member left pending together with the first hard error. Adds do not start
// when a promotion failed hard: the set is not in the shape the plan assumed.
// SHARD-013, SHARD-023
func ReconcileMembers(ctx context.Context, promotions, newMembers []MemberOverride, ops memberAddOps) ([]memberPending, error) {
	pending, err := PromoteMembersSequentially(ctx, promotions, ops)
	if err != nil {
		return pending, err
	}
	added, err := AddMembersSequentially(ctx, newMembers, ops)
	return append(pending, added...), err
}

// PromoteMembersSequentially gives live members the votes and priority their
// blocks ask for, one reconfig each, after each has reported SECONDARY. It
// finishes adds whose promotion did not fit in an earlier apply, and is the
// path for any block that turns a non-voting member into a voter. A member
// still syncing is returned in pending and the loop continues. SHARD-023
func PromoteMembersSequentially(ctx context.Context, overrides []MemberOverride, ops memberAddOps) ([]memberPending, error) {
	var pending []memberPending
	for _, o := range overrides {
		p, err := settleMember(ctx, o, ops, false)
		if err != nil {
			return pending, err
		}
		if p != nil {
			pending = append(pending, *p)
		}
	}
	return pending, nil
}

// settleMember waits for o's host to reach its target state, then applies the
// block's votes and priority if they differ from the staged 0/0. addedNow says
// whether this apply added the member, which decides whether a host that
// never became reachable is removed again or left alone (SHARD-019). A
// reachable member that is still syncing comes back as pending rather than
// an error. Removal needs the primary to have actually reported the member
// down at least once: if status could not be read at all, the member is left
// alone and the wait is reported as inconclusive.
func settleMember(ctx context.Context, o MemberOverride, ops memberAddOps, addedNow bool) (*memberPending, error) {
	target := waitTargetFor(o)
	res, err := ops.Wait(ctx, o.Host, target)
	neverSeenUp := res.Observed && !res.EverReachable
	if err != nil {
		// The apply is being interrupted. A member added just now that the
		// primary has reported down and never up is most likely a typo, so
		// remove it on a context that outlives the cancelled one; anything
		// else stays as a non-voter for the next apply to finish.
		if addedNow && neverSeenUp {
			rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackGrace)
			defer cancel()
			if _, rbErr := removeMemberByHost(rbCtx, o.Host, ops); rbErr != nil {
				return nil, fmt.Errorf("waiting for member %s to become %s: %w; it was never reachable and removing it again also failed: %v",
					o.Host, target, err, rbErr)
			}
			return nil, fmt.Errorf("waiting for member %s to become %s: %w; it was never reachable, so it was removed again", o.Host, target, err)
		}
		return nil, fmt.Errorf("waiting for member %s to become %s: %w", o.Host, target, err)
	}
	if !res.Met {
		switch {
		case !res.Observed && addedNow:
			return nil, fmt.Errorf("member %s was added but replSetGetStatus could not be read within %s (last: %s); it was left in the replica set as a non-voter and the next apply will check it again",
				o.Host, ops.Timeout, res.Last)
		case !res.Observed:
			return nil, fmt.Errorf("member %s: replSetGetStatus could not be read within %s (last: %s)", o.Host, ops.Timeout, res.Last)
		case neverSeenUp && addedNow:
			if _, rbErr := removeMemberByHost(ctx, o.Host, ops); rbErr != nil {
				return nil, fmt.Errorf("member %s was added but did not become reachable within %s: %s; removing it again also failed: %v",
					o.Host, ops.Timeout, res.Last, rbErr)
			}
			return nil, fmt.Errorf("member %s was added but did not become reachable within %s, so it was removed again (last observed: %s)",
				o.Host, ops.Timeout, res.Last)
		case neverSeenUp:
			return nil, fmt.Errorf("member %s did not become reachable within %s (last observed: %s)", o.Host, ops.Timeout, res.Last)
		case needsPromotion(o):
			tflog.Warn(ctx, "replica set member still syncing; leaving it as a non-voter", map[string]interface{}{
				"host": o.Host, "target": target.String(), "status": res.Last,
			})
			return &memberPending{Host: o.Host, Target: target, Last: res.Last, Votes: o.Votes, Priority: o.Priority, Timeout: ops.Timeout}, nil
		default:
			return nil, fmt.Errorf("member %s did not become %s within %s (last observed: %s)", o.Host, target, ops.Timeout, res.Last)
		}
	}
	if !needsPromotion(o) {
		return nil, nil
	}
	return nil, promoteMember(ctx, o, ops)
}

// promoteMember sets the member's votes and priority to the block's values in
// one reconfig, a no-op when they already match. SHARD-022, SHARD-023
func promoteMember(ctx context.Context, o MemberOverride, ops memberAddOps) error {
	cfg, err := ops.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("reading config before promoting member %s: %w", o.Host, err)
	}
	idx := -1
	for i, m := range cfg.Members {
		if m.Host == o.Host {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("promoting member %s: it is no longer in the replica set configuration", o.Host)
	}
	if cfg.Members[idx].Priority == o.Priority && derefInt(cfg.Members[idx].Votes) == o.Votes {
		return nil
	}
	cfg.Members[idx].Priority = o.Priority
	cfg.Members[idx].Votes = intPtr(o.Votes)
	cfg.Version++
	tflog.Info(ctx, "promoting replica set member", map[string]interface{}{
		"host": o.Host, "votes": o.Votes, "priority": o.Priority, "version": cfg.Version,
	})
	if err := ops.SetConfig(ctx, cfg); err != nil {
		return fmt.Errorf("promoting member %s to votes %d and priority %v: %w", o.Host, o.Votes, o.Priority, err)
	}
	return nil
}

// removeMemberByHost undoes an add whose member never became reachable, and
// reports whether there was anything to remove. The resource otherwise never
// removes members. SHARD-017, SHARD-019
func removeMemberByHost(ctx context.Context, host string, ops memberAddOps) (bool, error) {
	cfg, err := ops.GetConfig(ctx)
	if err != nil {
		return false, err
	}
	kept := make(ConfigMembers, 0, len(cfg.Members))
	for _, m := range cfg.Members {
		if m.Host != host {
			kept = append(kept, m)
		}
	}
	if len(kept) == len(cfg.Members) {
		return false, nil
	}
	cfg.Members = kept
	cfg.Version++
	tflog.Warn(ctx, "removing unreachable replica set member", map[string]interface{}{
		"host": host, "version": cfg.Version,
	})
	if err := ops.SetConfig(ctx, cfg); err != nil {
		return false, err
	}
	return true, nil
}
