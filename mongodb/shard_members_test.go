package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type waitCall struct {
	host   string
	target memberWaitTarget
}

// fakeMemberAddServer stands in for the primary in the add and promote
// sequences: GetConfig returns a copy of the current config, SetConfig must
// carry version+1 and is recorded, Wait records the host and target and
// reports what the test scripted (target met, by default).
type fakeMemberAddServer struct {
	current   RSConfig
	reconfigs []RSConfig
	waited    []waitCall
	getCalls  int
	setErr    func(cfg *RSConfig) error
	wait      func(host string, target memberWaitTarget) memberWaitResult
	waitErr   error
}

func newFakeMemberAddServer(members ...ConfigMember) *fakeMemberAddServer {
	return &fakeMemberAddServer{current: RSConfig{ID: "rs0", Version: 5, Members: members}}
}

func copyRSConfig(c RSConfig) RSConfig {
	out := c
	out.Members = make(ConfigMembers, len(c.Members))
	copy(out.Members, c.Members)
	return out
}

func (f *fakeMemberAddServer) ops() memberAddOps {
	return memberAddOps{
		GetConfig: func(context.Context) (*RSConfig, error) {
			f.getCalls++
			c := copyRSConfig(f.current)
			return &c, nil
		},
		SetConfig: func(_ context.Context, cfg *RSConfig) error {
			if f.setErr != nil {
				if err := f.setErr(cfg); err != nil {
					return err
				}
			}
			if cfg.Version != f.current.Version+1 {
				return fmt.Errorf("version %d is not current+1 (%d)", cfg.Version, f.current.Version+1)
			}
			f.current = copyRSConfig(*cfg)
			f.reconfigs = append(f.reconfigs, copyRSConfig(*cfg))
			return nil
		},
		Wait: func(_ context.Context, host string, target memberWaitTarget) (memberWaitResult, error) {
			f.waited = append(f.waited, waitCall{host, target})
			if f.waitErr != nil {
				return memberWaitResult{}, f.waitErr
			}
			if f.wait != nil {
				return f.wait(host, target), nil
			}
			return memberWaitResult{Met: true, EverReachable: true, Last: "health=1 state=" + target.String()}, nil
		},
		Timeout: time.Minute,
	}
}

func (f *fakeMemberAddServer) member(host string) ConfigMember {
	for _, m := range f.current.Members {
		if m.Host == host {
			return m
		}
	}
	return ConfigMember{}
}

func assertHosts(t *testing.T, label string, members ConfigMembers, want ...string) {
	t.Helper()
	got := memberHosts(members)
	if len(got) != len(want) {
		t.Fatalf("%s: want hosts %v, got %v", label, want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: index %d want %s, got %s", label, i, want[i], got[i])
		}
	}
}

func assertWaits(t *testing.T, got []waitCall, want ...waitCall) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("waits: want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("wait %d: want %v, got %v", i, want[i], got[i])
		}
	}
}

// SHARD-T14: SHARD-012 — PartitionMemberOverrides splits by live host and keeps block order
func TestPartitionMemberOverrides_SplitsAndKeepsOrder(t *testing.T) {
	live := ConfigMembers{{ID: 0, Host: "mongo1:27017"}, {ID: 3, Host: "mongo3:27017"}}
	overrides := []MemberOverride{
		{Host: "mongo3:27017"}, {Host: "mongo4:27017"}, {Host: "mongo1:27017"}, {Host: "mongo2:27017"},
	}
	existing, missing := PartitionMemberOverrides(live, overrides)

	if len(existing) != 2 || existing[0].Host != "mongo3:27017" || existing[1].Host != "mongo1:27017" {
		t.Errorf("existing: want [mongo3 mongo1] in block order, got %v", existing)
	}
	if len(missing) != 2 || missing[0].Host != "mongo4:27017" || missing[1].Host != "mongo2:27017" {
		t.Errorf("missing: want [mongo4 mongo2] in block order, got %v", missing)
	}
}

// SHARD-T15: SHARD-012 — every block is missing against an empty set, none against a full match
func TestPartitionMemberOverrides_Edges(t *testing.T) {
	overrides := []MemberOverride{{Host: "a:1"}, {Host: "b:1"}}

	existing, missing := PartitionMemberOverrides(nil, overrides)
	if len(existing) != 0 || len(missing) != 2 {
		t.Errorf("empty set: want 0 existing / 2 missing, got %d / %d", len(existing), len(missing))
	}

	existing, missing = PartitionMemberOverrides(ConfigMembers{{Host: "a:1"}, {Host: "b:1"}}, overrides)
	if len(existing) != 2 || len(missing) != 0 {
		t.Errorf("full match: want 2 existing / 0 missing, got %d / %d", len(existing), len(missing))
	}

	existing, missing = PartitionMemberOverrides(ConfigMembers{{Host: "a:1"}}, nil)
	if len(existing) != 0 || len(missing) != 0 {
		t.Errorf("no blocks: want nothing in either group, got %d / %d", len(existing), len(missing))
	}
}

// SHARD-T16: SHARD-014 — NextMemberID is highest+1, never the position or the count
func TestNextMemberID(t *testing.T) {
	cases := []struct {
		name    string
		members ConfigMembers
		want    int
	}{
		{"empty", nil, 0},
		{"contiguous", ConfigMembers{{ID: 0}, {ID: 1}, {ID: 2}}, 3},
		{"gap after manual removal", ConfigMembers{{ID: 0}, {ID: 2}}, 3},
		{"single high id", ConfigMembers{{ID: 7}}, 8},
		{"unordered", ConfigMembers{{ID: 5}, {ID: 0}, {ID: 2}}, 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NextMemberID(tc.members); got != tc.want {
				t.Errorf("want %d, got %d", tc.want, got)
			}
		})
	}
}

// SHARD-T17: SHARD-015 — BuildConfigMember carries every field, including priority 0
func TestBuildConfigMember_AllFields(t *testing.T) {
	m := BuildConfigMember(MemberOverride{
		Host:         "mongo3:27017",
		Priority:     0,
		Votes:        0,
		Hidden:       true,
		ArbiterOnly:  false,
		BuildIndexes: false,
		Tags:         map[string]string{"dc": "east", "rack": "r1"},
	}, 7)

	if m.ID != 7 {
		t.Errorf("_id: want 7, got %d", m.ID)
	}
	if m.Host != "mongo3:27017" {
		t.Errorf("host: want mongo3:27017, got %s", m.Host)
	}
	if m.Priority != 0 {
		t.Errorf("priority: want 0, got %v", m.Priority)
	}
	if derefInt(m.Votes) != 0 || m.Votes == nil {
		t.Errorf("votes: want explicit 0, got %v", m.Votes)
	}
	if !derefBool(m.Hidden) {
		t.Error("hidden: want true")
	}
	if derefBool(m.ArbiterOnly) {
		t.Error("arbiterOnly: want false")
	}
	if m.BuildIndexes == nil || *m.BuildIndexes {
		t.Error("buildIndexes: want explicit false")
	}
	if m.Tags["dc"] != "east" || m.Tags["rack"] != "r1" {
		t.Errorf("tags: want {dc:east, rack:r1}, got %v", m.Tags)
	}
}

// SHARD-T18: SHARD-015 — a block without tags yields no tags document
func TestBuildConfigMember_NoTags(t *testing.T) {
	m := BuildConfigMember(MemberOverride{Host: "mongo1:27017", Priority: 1, Votes: 1, BuildIndexes: true}, 0)
	if m.Tags != nil {
		t.Errorf("tags: want nil, got %v", m.Tags)
	}
	if m.ID != 0 || m.Priority != 1 || derefInt(m.Votes) != 1 || !derefBool(m.BuildIndexes) {
		t.Errorf("unexpected member: %+v", m)
	}
}

// SHARD-T19: SHARD-013, SHARD-014, SHARD-022 — one reconfig per step, in block order, ids from the re-read config
func TestAddMembersSequentially_OneReconfigPerStep(t *testing.T) {
	srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "mongo1:27017"})
	overrides := []MemberOverride{
		{Host: "mongo2:27017", Priority: 1, Votes: 1, BuildIndexes: true},
		{Host: "mongo3:27017", Priority: 0, Votes: 0, Hidden: true, BuildIndexes: true},
	}

	if err := AddMembersSequentially(context.Background(), overrides, srv.ops()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// add mongo2 staged, promote mongo2, add mongo3 (already a non-voter)
	if len(srv.reconfigs) != 3 {
		t.Fatalf("want 3 reconfigs, got %d", len(srv.reconfigs))
	}
	assertHosts(t, "first reconfig", srv.reconfigs[0].Members, "mongo1:27017", "mongo2:27017")
	assertHosts(t, "second reconfig", srv.reconfigs[1].Members, "mongo1:27017", "mongo2:27017")
	assertHosts(t, "third reconfig", srv.reconfigs[2].Members, "mongo1:27017", "mongo2:27017", "mongo3:27017")
	for i, want := range []int{6, 7, 8} {
		if srv.reconfigs[i].Version != want {
			t.Errorf("reconfig %d version: want %d, got %d", i, want, srv.reconfigs[i].Version)
		}
	}

	staged := srv.reconfigs[0].Members[1]
	if staged.ID != 1 || derefInt(staged.Votes) != 0 || staged.Priority != 0 || !derefBool(staged.BuildIndexes) {
		t.Errorf("mongo2 should be added as _id 1 with votes 0 and priority 0, got %+v", staged)
	}
	promoted := srv.reconfigs[1].Members[1]
	if derefInt(promoted.Votes) != 1 || promoted.Priority != 1 {
		t.Errorf("mongo2 should be promoted to votes 1 priority 1, got %+v", promoted)
	}
	added := srv.reconfigs[2].Members[2]
	if added.ID != 2 || !derefBool(added.Hidden) || added.Priority != 0 || derefInt(added.Votes) != 0 {
		t.Errorf("mongo3 should be added as _id 2 hidden non-voter, got %+v", added)
	}

	assertWaits(t, srv.waited,
		waitCall{"mongo2:27017", waitSecondary},
		waitCall{"mongo3:27017", waitReachable},
	)
}

// SHARD-T20: SHARD-014 — ids and versions follow the live config, including changes made between steps
func TestAddMembersSequentially_FollowsLiveConfig(t *testing.T) {
	srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "a:1"}, ConfigMember{ID: 4, Host: "b:1"})
	// Simulate the server reconfiguring on its own (e.g. clearing newlyAdded)
	// while we wait for the member.
	srv.wait = func(string, memberWaitTarget) memberWaitResult {
		srv.current.Version += 3
		return memberWaitResult{Met: true, EverReachable: true}
	}

	err := AddMembersSequentially(context.Background(), []MemberOverride{{Host: "c:1"}, {Host: "d:1"}}, srv.ops())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(srv.reconfigs) != 2 {
		t.Fatalf("want 2 reconfigs, got %d", len(srv.reconfigs))
	}
	if id := srv.reconfigs[0].Members[2].ID; id != 5 {
		t.Errorf("c _id: want 5 (highest 4 + 1), got %d", id)
	}
	if id := srv.reconfigs[1].Members[3].ID; id != 6 {
		t.Errorf("d _id: want 6, got %d", id)
	}
	if srv.reconfigs[0].Version != 6 {
		t.Errorf("first version: want 6, got %d", srv.reconfigs[0].Version)
	}
	if srv.reconfigs[1].Version != 10 {
		t.Errorf("second version: want 10 (6, +3 by the server, +1), got %d", srv.reconfigs[1].Version)
	}
}

// SHARD-T21: SHARD-013 — a failed reconfig stops the loop and names the member
func TestAddMembersSequentially_StopsOnReconfigError(t *testing.T) {
	srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "mongo1:27017"})
	srv.setErr = func(cfg *RSConfig) error {
		for _, m := range cfg.Members {
			if m.Host == "mongo3:27017" {
				return errors.New("Non force replica set reconfig can only add or remove at most 1 voting member")
			}
		}
		return nil
	}

	err := AddMembersSequentially(context.Background(), []MemberOverride{
		{Host: "mongo2:27017"}, {Host: "mongo3:27017"}, {Host: "mongo4:27017"},
	}, srv.ops())

	if err == nil || !strings.Contains(err.Error(), "mongo3:27017") {
		t.Fatalf("want error naming mongo3, got %v", err)
	}
	if strings.Contains(err.Error(), "removed again") {
		t.Errorf("nothing was installed, so nothing should be reported removed: %v", err)
	}
	if len(srv.reconfigs) != 1 {
		t.Errorf("want the loop to stop after the failed add, got %d reconfigs", len(srv.reconfigs))
	}
	assertHosts(t, "live config", srv.current.Members, "mongo1:27017", "mongo2:27017")
}

// SHARD-T22: SHARD-017 — a member that never becomes reachable is removed again and the add fails
func TestAddMembersSequentially_RollsBackUnreachableMember(t *testing.T) {
	srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "mongo1:27017"})
	srv.wait = func(host string, _ memberWaitTarget) memberWaitResult {
		if host == "mongo3:27017" {
			return memberWaitResult{Last: "not listed in replSetGetStatus"}
		}
		return memberWaitResult{Met: true, EverReachable: true}
	}

	err := AddMembersSequentially(context.Background(), []MemberOverride{
		{Host: "mongo2:27017"}, {Host: "mongo3:27017"}, {Host: "mongo4:27017"},
	}, srv.ops())

	if err == nil {
		t.Fatal("want an error for the unreachable member")
	}
	for _, want := range []string{"mongo3:27017", "removed again", "not listed", "1m0s"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should contain %q, got: %v", want, err)
		}
	}
	if len(srv.reconfigs) != 3 {
		t.Fatalf("want add, add, remove = 3 reconfigs, got %d", len(srv.reconfigs))
	}
	assertHosts(t, "rollback reconfig", srv.reconfigs[2].Members, "mongo1:27017", "mongo2:27017")
	assertHosts(t, "live config", srv.current.Members, "mongo1:27017", "mongo2:27017")
	if srv.reconfigs[2].Version != 8 {
		t.Errorf("rollback version: want 8, got %d", srv.reconfigs[2].Version)
	}
}

// SHARD-T23: SHARD-017 — when the rollback also fails, both errors are reported
func TestAddMembersSequentially_ReportsRollbackFailure(t *testing.T) {
	srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "mongo1:27017"})
	srv.wait = func(string, memberWaitTarget) memberWaitResult {
		return memberWaitResult{Last: "health=0 state=(not reachable/healthy)"}
	}
	srv.setErr = func(cfg *RSConfig) error {
		if len(cfg.Members) < len(srv.current.Members) {
			return errors.New("not primary")
		}
		return nil
	}

	err := AddMembersSequentially(context.Background(), []MemberOverride{{Host: "mongo2:27017"}}, srv.ops())

	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"did not become reachable", "health=0", "removing it again also failed", "not primary"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should contain %q, got: %v", want, err)
		}
	}
	assertHosts(t, "live config keeps the member", srv.current.Members, "mongo1:27017", "mongo2:27017")
}

// SHARD-T24: SHARD-013 — no missing members means no server interaction at all
func TestAddMembersSequentially_NoMembers(t *testing.T) {
	srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "mongo1:27017"})
	if err := AddMembersSequentially(context.Background(), nil, srv.ops()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv.getCalls != 0 || len(srv.reconfigs) != 0 || len(srv.waited) != 0 {
		t.Errorf("want no reads, reconfigs or waits, got %d/%d/%d", srv.getCalls, len(srv.reconfigs), len(srv.waited))
	}
}

// SHARD-T26: SHARD-022 — a voter is staged with votes 0 / priority 0 and every other field, then promoted
func TestAddMembersSequentially_StagesVoterThenPromotes(t *testing.T) {
	srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "mongo1:27017"})
	override := MemberOverride{
		Host: "mongo2:27017", Priority: 2, Votes: 1, BuildIndexes: true,
		Tags: map[string]string{"dc": "east"},
	}

	if err := AddMembersSequentially(context.Background(), []MemberOverride{override}, srv.ops()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(srv.reconfigs) != 2 {
		t.Fatalf("want add then promote = 2 reconfigs, got %d", len(srv.reconfigs))
	}
	staged := srv.reconfigs[0].Members[1]
	if derefInt(staged.Votes) != 0 || staged.Priority != 0 {
		t.Errorf("staged add must be non-voting with priority 0, got votes=%d priority=%v", derefInt(staged.Votes), staged.Priority)
	}
	if staged.Tags["dc"] != "east" || !derefBool(staged.BuildIndexes) || derefBool(staged.Hidden) {
		t.Errorf("staged add must carry the other fields, got %+v", staged)
	}
	promoted := srv.reconfigs[1].Members[1]
	if derefInt(promoted.Votes) != 1 || promoted.Priority != 2 || promoted.ID != 1 || promoted.Tags["dc"] != "east" {
		t.Errorf("promotion must set votes 1 priority 2 and change nothing else, got %+v", promoted)
	}
	assertWaits(t, srv.waited, waitCall{"mongo2:27017", waitSecondary})
}

// SHARD-T27: SHARD-022 — arbiters cannot be staged: added in final form, waited on for ARBITER, never promoted
func TestAddMembersSequentially_ArbiterAddedDirectly(t *testing.T) {
	srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "mongo1:27017"})
	override := MemberOverride{Host: "arb:27017", Priority: 0, Votes: 1, ArbiterOnly: true, BuildIndexes: true}

	if err := AddMembersSequentially(context.Background(), []MemberOverride{override}, srv.ops()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(srv.reconfigs) != 1 {
		t.Fatalf("want a single reconfig for an arbiter, got %d", len(srv.reconfigs))
	}
	arb := srv.reconfigs[0].Members[1]
	if !derefBool(arb.ArbiterOnly) || derefInt(arb.Votes) != 1 || arb.Priority != 0 {
		t.Errorf("arbiter must be added with arbiterOnly, votes 1, priority 0, got %+v", arb)
	}
	assertWaits(t, srv.waited, waitCall{"arb:27017", waitArbiter})
}

// SHARD-T28: SHARD-016, SHARD-017 — a reachable member still syncing at the timeout is kept as a non-voter
func TestAddMembersSequentially_KeepsSyncingMember(t *testing.T) {
	srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "mongo1:27017"})
	srv.wait = func(string, memberWaitTarget) memberWaitResult {
		return memberWaitResult{EverReachable: true, Last: "health=1 state=STARTUP2"}
	}

	err := AddMembersSequentially(context.Background(), []MemberOverride{
		{Host: "mongo2:27017", Priority: 1, Votes: 1}, {Host: "mongo3:27017", Priority: 1, Votes: 1},
	}, srv.ops())

	if err == nil {
		t.Fatal("want an error when the member is not SECONDARY in time")
	}
	for _, want := range []string{"mongo2:27017", "STARTUP2", "keeps its current votes", "votes 1 and priority 1", "next apply"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should contain %q, got: %v", want, err)
		}
	}
	if len(srv.reconfigs) != 1 {
		t.Fatalf("want only the staged add, no promotion and no removal, got %d reconfigs", len(srv.reconfigs))
	}
	kept := srv.member("mongo2:27017")
	if kept.Host == "" || derefInt(kept.Votes) != 0 || kept.Priority != 0 {
		t.Errorf("member should stay in the set as a non-voter, got %+v", kept)
	}
	if srv.member("mongo3:27017").Host != "" {
		t.Error("the loop must stop before adding the next member")
	}
}

// SHARD-T29: SHARD-023 — HoldPromotions pins votes and priority of a promoting block, passes the rest through
func TestHoldPromotions(t *testing.T) {
	live := ConfigMembers{
		{ID: 0, Host: "a:1", Votes: intPtr(1), Priority: 1},
		{ID: 1, Host: "b:1", Votes: intPtr(0), Priority: 0},
		{ID: 2, Host: "c:1", Votes: intPtr(1), Priority: 1},
	}
	overrides := []MemberOverride{
		{Host: "a:1", Votes: 1, Priority: 3},               // priority only: normal merge
		{Host: "b:1", Votes: 1, Priority: 2, Hidden: true}, // promotion
		{Host: "c:1", Votes: 0, Priority: 0},               // demotion: normal merge
		{Host: "d:1", Votes: 1, Priority: 1},               // not live: passes through
	}

	held, promote := HoldPromotions(live, overrides)

	if len(held) != 4 {
		t.Fatalf("held must carry every block, got %d", len(held))
	}
	if held[0].Priority != 3 || held[0].Votes != 1 {
		t.Errorf("a: priority change must pass through, got %+v", held[0])
	}
	if held[1].Votes != 0 || held[1].Priority != 0 || !held[1].Hidden {
		t.Errorf("b: votes and priority must be pinned to live values, other fields kept, got %+v", held[1])
	}
	if held[2].Votes != 0 || held[2].Priority != 0 {
		t.Errorf("c: demotion must pass through, got %+v", held[2])
	}
	if held[3].Votes != 1 || held[3].Priority != 1 {
		t.Errorf("d: unknown host must pass through, got %+v", held[3])
	}
	if len(promote) != 1 || promote[0].Host != "b:1" || promote[0].Votes != 1 || promote[0].Priority != 2 {
		t.Errorf("promote: want only b with its configured values, got %v", promote)
	}
}

// SHARD-T30: SHARD-023, SHARD-019 — PromoteMembersSequentially waits for SECONDARY, reconfigs once, never removes
func TestPromoteMembersSequentially(t *testing.T) {
	t.Run("promotes after SECONDARY", func(t *testing.T) {
		srv := newFakeMemberAddServer(
			ConfigMember{ID: 0, Host: "mongo1:27017", Votes: intPtr(1), Priority: 1},
			ConfigMember{ID: 1, Host: "mongo2:27017", Votes: intPtr(0), Priority: 0},
		)
		err := PromoteMembersSequentially(context.Background(), []MemberOverride{{Host: "mongo2:27017", Votes: 1, Priority: 1}}, srv.ops())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(srv.reconfigs) != 1 || srv.reconfigs[0].Version != 6 {
			t.Fatalf("want one reconfig at version 6, got %+v", srv.reconfigs)
		}
		m := srv.member("mongo2:27017")
		if derefInt(m.Votes) != 1 || m.Priority != 1 || m.ID != 1 {
			t.Errorf("want votes 1 priority 1 on the same _id, got %+v", m)
		}
		assertWaits(t, srv.waited, waitCall{"mongo2:27017", waitSecondary})
	})

	t.Run("no reconfig when already matching", func(t *testing.T) {
		srv := newFakeMemberAddServer(ConfigMember{ID: 1, Host: "mongo2:27017", Votes: intPtr(1), Priority: 1})
		err := PromoteMembersSequentially(context.Background(), []MemberOverride{{Host: "mongo2:27017", Votes: 1, Priority: 1}}, srv.ops())
		if err != nil || len(srv.reconfigs) != 0 {
			t.Fatalf("want no reconfig and no error, got %d reconfigs, err %v", len(srv.reconfigs), err)
		}
	})

	t.Run("unreachable live member is reported, not removed", func(t *testing.T) {
		srv := newFakeMemberAddServer(ConfigMember{ID: 1, Host: "mongo2:27017", Votes: intPtr(0), Priority: 0})
		srv.wait = func(string, memberWaitTarget) memberWaitResult {
			return memberWaitResult{Last: "health=0 state=(not reachable/healthy)"}
		}
		err := PromoteMembersSequentially(context.Background(), []MemberOverride{{Host: "mongo2:27017", Votes: 1, Priority: 1}}, srv.ops())
		if err == nil || !strings.Contains(err.Error(), "did not become reachable") || strings.Contains(err.Error(), "removed") {
			t.Fatalf("want a plain unreachable error, got %v", err)
		}
		if len(srv.reconfigs) != 0 || srv.member("mongo2:27017").Host == "" {
			t.Errorf("a live member must never be removed, got %d reconfigs", len(srv.reconfigs))
		}
	})

	t.Run("member gone from the config", func(t *testing.T) {
		srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "mongo1:27017", Votes: intPtr(1), Priority: 1})
		err := PromoteMembersSequentially(context.Background(), []MemberOverride{{Host: "mongo2:27017", Votes: 1, Priority: 1}}, srv.ops())
		if err == nil || !strings.Contains(err.Error(), "no longer in the replica set configuration") {
			t.Fatalf("want an error about the missing member, got %v", err)
		}
	})
}

// SHARD-T31: SHARD-016 — observeMember per target: reachable accepts any health-1 state, SECONDARY and ARBITER are strict
func TestObserveMember(t *testing.T) {
	status := &ReplSetStatus{Members: []*Member{
		{Name: "p:1", Health: MemberHealthUp, State: MemberStatePrimary, StateStr: "PRIMARY"},
		{Name: "s:1", Health: MemberHealthUp, State: MemberStateSecondary, StateStr: "SECONDARY"},
		{Name: "b:1", Health: MemberHealthUp, State: MemberStateStartup2, StateStr: "STARTUP2"},
		{Name: "c:1", Health: MemberHealthDown, State: MemberStateUnknown, StateStr: "(not reachable/healthy)"},
		{Name: "d:1", Health: MemberHealthUp, State: MemberStateArbiter},
	}}
	cases := []struct {
		host           string
		target         memberWaitTarget
		met, reachable bool
		seen           string
	}{
		{"b:1", waitReachable, true, true, "STARTUP2"},
		{"b:1", waitSecondary, false, true, "STARTUP2"},
		{"s:1", waitSecondary, true, true, "SECONDARY"},
		{"p:1", waitSecondary, true, true, "PRIMARY"},
		{"d:1", waitArbiter, true, true, "ARBITER"},
		{"s:1", waitArbiter, false, true, "SECONDARY"},
		{"c:1", waitReachable, false, false, "health=0"},
		{"zz:1", waitReachable, false, false, "not listed"},
	}
	for _, tc := range cases {
		obs := observeMember(status, tc.host, tc.target)
		if obs.Met != tc.met || obs.Reachable != tc.reachable || !strings.Contains(obs.Seen, tc.seen) {
			t.Errorf("%s/%s: want met=%v reachable=%v seen~%q, got %+v", tc.host, tc.target, tc.met, tc.reachable, tc.seen, obs)
		}
	}
}

// SHARD-T32: SHARD-016 — WaitForMemberState polls until the target, reports reachability seen along the way, and stops on timeout or ctx
func TestWaitForMemberState(t *testing.T) {
	script := func(states ...*ReplSetStatus) func(context.Context) (*ReplSetStatus, error) {
		i := 0
		return func(context.Context) (*ReplSetStatus, error) {
			if i >= len(states) {
				return states[len(states)-1], nil
			}
			s := states[i]
			i++
			if s == nil {
				return nil, errors.New("connection reset")
			}
			return s, nil
		}
	}
	row := func(health MemberHealth, state MemberState) *ReplSetStatus {
		return &ReplSetStatus{Members: []*Member{{Name: "m:1", Health: health, State: state, StateStr: MemberStateStrings[state]}}}
	}
	const poll = time.Millisecond

	t.Run("reaches SECONDARY through STARTUP2", func(t *testing.T) {
		get := script(nil, &ReplSetStatus{}, row(MemberHealthDown, MemberStateUnknown), row(MemberHealthUp, MemberStateStartup2), row(MemberHealthUp, MemberStateSecondary))
		res, err := WaitForMemberState(context.Background(), get, "m:1", waitSecondary, time.Second, poll)
		if err != nil || !res.Met || !res.EverReachable || !strings.Contains(res.Last, "SECONDARY") {
			t.Fatalf("want met and reachable, got %+v err %v", res, err)
		}
	})

	t.Run("times out never reachable", func(t *testing.T) {
		res, err := WaitForMemberState(context.Background(), script(row(MemberHealthDown, MemberStateUnknown)), "m:1", waitReachable, 5*poll, poll)
		if err != nil || res.Met || res.EverReachable || !strings.Contains(res.Last, "health=0") {
			t.Fatalf("want a clean timeout with no reachability, got %+v err %v", res, err)
		}
	})

	t.Run("times out still syncing", func(t *testing.T) {
		res, err := WaitForMemberState(context.Background(), script(row(MemberHealthUp, MemberStateStartup2)), "m:1", waitSecondary, 5*poll, poll)
		if err != nil || res.Met || !res.EverReachable || !strings.Contains(res.Last, "STARTUP2") {
			t.Fatalf("want a timeout that remembers the member was reachable, got %+v err %v", res, err)
		}
	})

	t.Run("context cancellation is the only error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := WaitForMemberState(ctx, script(row(MemberHealthDown, MemberStateUnknown)), "m:1", waitReachable, time.Minute, poll)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	})
}

// SHARD-T33: SHARD-017 — an add whose reconfig fails after the config was installed is removed again
func TestAddMembersSequentially_RemovesMemberInstalledByFailedReconfig(t *testing.T) {
	srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "mongo1:27017"})
	installed := false
	srv.setErr = func(cfg *RSConfig) error {
		if !installed && len(cfg.Members) == 2 {
			installed = true
			srv.current = copyRSConfig(*cfg)
			return errors.New("Reconfig finished but failed to propagate to a majority")
		}
		return nil
	}

	err := AddMembersSequentially(context.Background(), []MemberOverride{{Host: "mongo2:27017"}}, srv.ops())

	if err == nil || !strings.Contains(err.Error(), "after the config was installed, so it was removed again") {
		t.Fatalf("want an error saying the installed member was removed, got %v", err)
	}
	assertHosts(t, "live config", srv.current.Members, "mongo1:27017")
	if len(srv.waited) != 0 {
		t.Errorf("no wait should run after a failed reconfig, got %v", srv.waited)
	}
}
