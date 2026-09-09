package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakeMemberAddServer stands in for the primary in AddMembersSequentially
// tests: GetConfig returns a copy of the current config, SetConfig must carry
// version+1 and is recorded, WaitReachable records the host.
type fakeMemberAddServer struct {
	current   RSConfig
	reconfigs []RSConfig
	waited    []string
	getCalls  int
	setErr    func(cfg *RSConfig) error
	waitErr   func(host string) error
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
		WaitReachable: func(_ context.Context, host string) error {
			f.waited = append(f.waited, host)
			if f.waitErr != nil {
				return f.waitErr(host)
			}
			return nil
		},
	}
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

// SHARD-T19: SHARD-013, SHARD-014 — one reconfig per member, in block order, ids from the re-read config
func TestAddMembersSequentially_OneReconfigPerMember(t *testing.T) {
	srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "mongo1:27017"})
	overrides := []MemberOverride{
		{Host: "mongo2:27017", Priority: 1, Votes: 1, BuildIndexes: true},
		{Host: "mongo3:27017", Priority: 0, Votes: 0, Hidden: true, BuildIndexes: true},
	}

	if err := AddMembersSequentially(context.Background(), overrides, srv.ops()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(srv.reconfigs) != 2 {
		t.Fatalf("want 2 reconfigs, got %d", len(srv.reconfigs))
	}
	assertHosts(t, "first reconfig", srv.reconfigs[0].Members, "mongo1:27017", "mongo2:27017")
	assertHosts(t, "second reconfig", srv.reconfigs[1].Members, "mongo1:27017", "mongo2:27017", "mongo3:27017")

	if srv.reconfigs[0].Version != 6 || srv.reconfigs[1].Version != 7 {
		t.Errorf("versions: want 6 then 7, got %d then %d", srv.reconfigs[0].Version, srv.reconfigs[1].Version)
	}
	if id := srv.reconfigs[0].Members[1].ID; id != 1 {
		t.Errorf("mongo2 _id: want 1, got %d", id)
	}
	added := srv.reconfigs[1].Members[2]
	if added.ID != 2 {
		t.Errorf("mongo3 _id: want 2, got %d", added.ID)
	}
	if !derefBool(added.Hidden) || added.Priority != 0 || derefInt(added.Votes) != 0 {
		t.Errorf("mongo3 fields not carried over: %+v", added)
	}

	if len(srv.waited) != 2 || srv.waited[0] != "mongo2:27017" || srv.waited[1] != "mongo3:27017" {
		t.Errorf("want a reachability wait per added member in order, got %v", srv.waited)
	}
}

// SHARD-T20: SHARD-014 — ids and versions follow the live config, including changes made between adds
func TestAddMembersSequentially_FollowsLiveConfig(t *testing.T) {
	srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "a:1"}, ConfigMember{ID: 4, Host: "b:1"})
	// Simulate the server reconfiguring on its own (e.g. clearing newlyAdded)
	// while we wait for the member.
	srv.waitErr = func(string) error {
		srv.current.Version += 3
		return nil
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
	if len(srv.reconfigs) != 1 {
		t.Errorf("want the loop to stop after the failed add, got %d reconfigs", len(srv.reconfigs))
	}
	assertHosts(t, "live config", srv.current.Members, "mongo1:27017", "mongo2:27017")
}

// SHARD-T22: SHARD-017 — an unreachable member is removed again and the add fails
func TestAddMembersSequentially_RollsBackUnreachableMember(t *testing.T) {
	srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "mongo1:27017"})
	srv.waitErr = func(host string) error {
		if host == "mongo3:27017" {
			return errors.New("did not become reachable within 1m0s (last observed: not listed in replSetGetStatus)")
		}
		return nil
	}

	err := AddMembersSequentially(context.Background(), []MemberOverride{
		{Host: "mongo2:27017"}, {Host: "mongo3:27017"}, {Host: "mongo4:27017"},
	}, srv.ops())

	if err == nil {
		t.Fatal("want an error for the unreachable member")
	}
	for _, want := range []string{"mongo3:27017", "removed again", "not listed"} {
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
	srv.waitErr = func(string) error { return errors.New("timed out") }
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
	for _, want := range []string{"did not become reachable", "timed out", "removing it again also failed", "not primary"} {
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

// SHARD-T25: SHARD-016 — memberReachable accepts any health-1 state, rejects health 0 and unlisted hosts
func TestMemberReachable(t *testing.T) {
	status := &ReplSetStatus{Members: []*Member{
		{Name: "a:1", Health: MemberHealthUp, State: MemberStatePrimary, StateStr: "PRIMARY"},
		{Name: "b:1", Health: MemberHealthUp, State: MemberStateStartup2, StateStr: "STARTUP2"},
		{Name: "c:1", Health: MemberHealthDown, State: MemberStateUnknown, StateStr: "(not reachable/healthy)"},
		{Name: "d:1", Health: MemberHealthUp, State: MemberStateArbiter},
	}}

	if ok, _ := memberReachable(status, "a:1"); !ok {
		t.Error("PRIMARY with health 1 should be reachable")
	}
	if ok, desc := memberReachable(status, "b:1"); !ok || !strings.Contains(desc, "STARTUP2") {
		t.Errorf("STARTUP2 with health 1 should be reachable, got ok=%v desc=%q", ok, desc)
	}
	if ok, desc := memberReachable(status, "c:1"); ok || !strings.Contains(desc, "health=0") {
		t.Errorf("health 0 should not be reachable, got ok=%v desc=%q", ok, desc)
	}
	if ok, desc := memberReachable(status, "d:1"); !ok || !strings.Contains(desc, "ARBITER") {
		t.Errorf("arbiter with health 1 should be reachable and described by state code, got ok=%v desc=%q", ok, desc)
	}
	if ok, desc := memberReachable(status, "zz:1"); ok || !strings.Contains(desc, "not listed") {
		t.Errorf("unlisted host should not be reachable, got ok=%v desc=%q", ok, desc)
	}
}
