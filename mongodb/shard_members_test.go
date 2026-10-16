package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// SHARD-T41: SHARD-027 — CheckAddTarget from the candidate's own isMaster answer
func TestCheckAddTarget(t *testing.T) {
	const set = "rs0"
	fresh := &IsMasterResp{IsReplicaSet: true, Info: "Does not have a valid replica set config"}
	member := func(me string, primary, secondary, arbiter bool) *IsMasterResp {
		return &IsMasterResp{SetName: set, Me: me, IsMaster: primary, Secondary: secondary, IsArbiter: arbiter}
	}
	cases := []struct {
		name    string
		resp    *IsMasterResp
		err     error
		verdict addTargetVerdict
		reason  string
	}{
		{"fresh node", fresh, nil, addTargetOK, "not yet initialized"},
		{"removed earlier answers like a fresh node", fresh, nil, addTargetOK, "removed from a replica set earlier"},
		{"no --replSet", &IsMasterResp{IsMaster: true}, nil, addTargetRefused, "--replSet"},
		{"mongos", &IsMasterResp{IsMaster: true, Msg: "isdbgrid"}, nil, addTargetRefused, "mongos"},
		{"unreachable", nil, errors.New("server selection timeout"), addTargetUnknown, "server selection timeout"},
		{"other set", &IsMasterResp{SetName: "rs1", Secondary: true}, nil, addTargetRefused, `replica set "rs1"`},
		{"live secondary under another name", member("mongo2.internal:27017", false, true, false), nil, addTargetRefused, "as mongo2.internal:27017 (SECONDARY)"},
		{"live primary under another name", member("mongo1.internal:27017", true, false, false), nil, addTargetRefused, "as mongo1.internal:27017 (PRIMARY)"},
		{"live arbiter under another name", member("arb.internal:27017", false, false, true), nil, addTargetRefused, "as arb.internal:27017 (ARBITER)"},
		{"syncing under another name", member("mongo3.internal:27017", false, false, false), nil, addTargetRefused, "(neither PRIMARY nor SECONDARY)"},
		{"member with no me", &IsMasterResp{SetName: set}, nil, addTargetRefused, "under another name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict, reason := CheckAddTarget(set, tc.resp, tc.err)
			if verdict != tc.verdict || !strings.Contains(reason, tc.reason) {
				t.Errorf("want verdict %d reason~%q, got %d %q", tc.verdict, tc.reason, verdict, reason)
			}
		})
	}
}

// SHARD-T43: SHARD-027 — the verdict is read from documents shaped the way
// servers shape them, pinning the isMaster field names the check relies on
func TestCheckAddTarget_ServerShapes(t *testing.T) {
	cases := []struct {
		name    string
		doc     bson.D
		verdict addTargetVerdict
		reason  string
	}{
		{"mongod with --replSet and no config", bson.D{
			{Key: "ismaster", Value: false}, {Key: "secondary", Value: false},
			{Key: "info", Value: "Does not have a valid replica set config"}, {Key: "isreplicaset", Value: true},
			{Key: "ok", Value: 1},
		}, addTargetOK, "not yet initialized"},
		{"secondary of this set", bson.D{
			{Key: "hosts", Value: bson.A{"mongo1:27017", "mongo2:27017"}}, {Key: "setName", Value: "rs0"},
			{Key: "ismaster", Value: false}, {Key: "secondary", Value: true},
			{Key: "primary", Value: "mongo1:27017"}, {Key: "me", Value: "mongo2:27017"}, {Key: "ok", Value: 1},
		}, addTargetRefused, "as mongo2:27017 (SECONDARY)"},
		{"arbiter of this set", bson.D{
			{Key: "setName", Value: "rs0"}, {Key: "ismaster", Value: false}, {Key: "secondary", Value: false},
			{Key: "arbiterOnly", Value: true}, {Key: "me", Value: "arb:27017"}, {Key: "ok", Value: 1},
		}, addTargetRefused, "as arb:27017 (ARBITER)"},
		{"standalone", bson.D{{Key: "ismaster", Value: true}, {Key: "ok", Value: 1}}, addTargetRefused, "--replSet"},
		{"mongos", bson.D{{Key: "ismaster", Value: true}, {Key: "msg", Value: "isdbgrid"}, {Key: "ok", Value: 1}}, addTargetRefused, "mongos"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := bson.Marshal(tc.doc)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var resp IsMasterResp
			if err := bson.Unmarshal(raw, &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			verdict, reason := CheckAddTarget("rs0", &resp, nil)
			if verdict != tc.verdict || !strings.Contains(reason, tc.reason) {
				t.Errorf("want verdict %d reason~%q, got %d %q", tc.verdict, tc.reason, verdict, reason)
			}
		})
	}
}

// SHARD-T42: SHARD-027, SHARD-028 — PreflightAddTargets probes in block
// order, lets uninspectable data-bearing hosts through, stops at the first
// refusal, and refuses an arbiter it could not inspect
func TestPreflightAddTargets(t *testing.T) {
	type answer struct {
		resp *IsMasterResp
		err  error
	}
	fresh := &IsMasterResp{IsReplicaSet: true}
	answers := map[string]answer{
		"fresh:27017":   {resp: fresh},
		"dark:27017":    {err: errors.New("dial tcp: no route to host")},
		"alias:27017":   {resp: &IsMasterResp{SetName: "rs0", Me: "real:27017", Secondary: true}},
		"unseen:27017":  {resp: fresh},
		"arb:27017":     {resp: fresh},
		"darkarb:27017": {err: errors.New("dial tcp: no route to host")},
	}
	var probed []string
	probe := func(_ context.Context, host string) (*IsMasterResp, error) {
		probed = append(probed, host)
		a, ok := answers[host]
		if !ok {
			t.Fatalf("unexpected probe of %s", host)
		}
		return a.resp, a.err
	}
	blocks := []MemberOverride{{Host: "fresh:27017"}, {Host: "dark:27017"}, {Host: "alias:27017"}, {Host: "unseen:27017"}}

	err := PreflightAddTargets(context.Background(), "rs0", blocks, probe)

	if err == nil {
		t.Fatal("want a refusal for the aliased member")
	}
	assertErrorContains(t, err, "alias:27017", "rs0", "already a member", "as real:27017")
	if len(probed) != 3 || probed[0] != "fresh:27017" || probed[1] != "dark:27017" || probed[2] != "alias:27017" {
		t.Errorf("want probes in block order stopping at the refusal, got %v", probed)
	}

	probed = nil
	if err := PreflightAddTargets(context.Background(), "rs0", blocks[:2], probe); err != nil {
		t.Errorf("a fresh host and an uninspectable data-bearing host must both pass, got %v", err)
	}
	if err := PreflightAddTargets(context.Background(), "rs0", nil, probe); err != nil || len(probed) != 2 {
		t.Errorf("no blocks means no probes, got err %v probes %v", err, probed)
	}

	t.Run("arbiter", func(t *testing.T) {
		arb := MemberOverride{Host: "arb:27017", ArbiterOnly: true, Votes: 1}
		if err := PreflightAddTargets(context.Background(), "rs0", []MemberOverride{arb}, probe); err != nil {
			t.Errorf("an arbiter host inspected as fresh must pass, got %v", err)
		}

		darkArb := MemberOverride{Host: "darkarb:27017", ArbiterOnly: true, Votes: 1}
		probed = nil
		err := PreflightAddTargets(context.Background(), "rs0", []MemberOverride{{Host: "fresh:27017"}, darkArb, {Host: "unseen:27017"}}, probe)
		if err == nil {
			t.Fatal("want a refusal for the arbiter that could not be inspected")
		}
		assertErrorContains(t, err, "arbiter darkarb:27017", "rs0", "could not be inspected", "no route to host", "majority")
		if len(probed) != 2 {
			t.Errorf("the refusal must stop the pre-flight, got probes %v", probed)
		}
	})

	t.Run("probe skipped under host_override", func(t *testing.T) {
		data := []MemberOverride{{Host: "a:27017", Votes: 1, Priority: 1}, {Host: "b:27017"}}
		if err := PreflightAddTargets(context.Background(), "rs0", data, memberProbeSkipped); err != nil {
			t.Errorf("data-bearing blocks must pass when the probe is skipped, got %v", err)
		}
		withArb := append(data, MemberOverride{Host: "arb:27017", ArbiterOnly: true, Votes: 1})
		err := PreflightAddTargets(context.Background(), "rs0", withArb, memberProbeSkipped)
		if err == nil {
			t.Fatal("want a refusal for an arbiter when the probe is skipped")
		}
		assertErrorContains(t, err, "arbiter arb:27017", "host_override")
	})
}

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
			var res memberWaitResult
			switch {
			case f.wait != nil:
				res = f.wait(host, target)
			case f.waitErr == nil:
				res = memberWaitResult{Met: true, EverReachable: true, Observed: true, Last: "health=1 state=" + target.String()}
			}
			return res, f.waitErr
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

// Wait scripts shared by the add and promote cases.
var (
	waitUnreachable = func(string, memberWaitTarget) memberWaitResult {
		return memberWaitResult{Observed: true, Last: "health=0 state=(not reachable/healthy)"}
	}
	waitStatusUnreadable = func(string, memberWaitTarget) memberWaitResult {
		return memberWaitResult{Last: "replSetGetStatus: connection reset by peer"}
	}
	waitNotListed = func(string, memberWaitTarget) memberWaitResult {
		return memberWaitResult{Observed: true, Last: "not listed in replSetGetStatus"}
	}
)

func assertStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: want %v, got %v", label, want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: index %d want %s, got %s", label, i, want[i], got[i])
		}
	}
}

func assertHosts(t *testing.T, label string, members ConfigMembers, want ...string) {
	t.Helper()
	assertStrings(t, label, memberHosts(members), want)
}

func assertOverrideHosts(t *testing.T, label string, overrides []MemberOverride, want ...string) {
	t.Helper()
	got := make([]string, len(overrides))
	for i, o := range overrides {
		got[i] = o.Host
	}
	assertStrings(t, label, got, want)
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

func assertErrorContains(t *testing.T, err error, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should contain %q, got: %v", want, err)
		}
	}
}

// assertRun checks what every add and promote case asserts: wantErr nil means
// the run must succeed, otherwise the error must carry each substring; live
// nil leaves the final config unchecked.
func assertRun(t *testing.T, srv *fakeMemberAddServer, err error, wantErr []string, reconfigs int, live []string) {
	t.Helper()
	switch {
	case wantErr == nil && err != nil:
		t.Fatalf("unexpected error: %v", err)
	case wantErr != nil && err == nil:
		t.Fatal("want an error")
	case err != nil:
		assertErrorContains(t, err, wantErr...)
	}
	if len(srv.reconfigs) != reconfigs {
		t.Fatalf("want %d reconfigs, got %d", reconfigs, len(srv.reconfigs))
	}
	if live != nil {
		assertHosts(t, "live config", srv.current.Members, live...)
	}
}

// SHARD-T14, SHARD-T15: SHARD-012 — PartitionMemberOverrides splits by live
// host and keeps block order; every block is missing against an empty set,
// none against a full match
func TestPartitionMemberOverrides(t *testing.T) {
	cases := []struct {
		name         string
		live         ConfigMembers
		overrides    []MemberOverride
		wantExisting []string
		wantMissing  []string
	}{
		{name: "splits and keeps block order", // SHARD-T14
			live:         ConfigMembers{{ID: 0, Host: "mongo1:27017"}, {ID: 3, Host: "mongo3:27017"}},
			overrides:    []MemberOverride{{Host: "mongo3:27017"}, {Host: "mongo4:27017"}, {Host: "mongo1:27017"}, {Host: "mongo2:27017"}},
			wantExisting: []string{"mongo3:27017", "mongo1:27017"},
			wantMissing:  []string{"mongo4:27017", "mongo2:27017"}},
		{name: "empty set", // SHARD-T15
			overrides:   []MemberOverride{{Host: "a:1"}, {Host: "b:1"}},
			wantMissing: []string{"a:1", "b:1"}},
		{name: "full match", // SHARD-T15
			live:         ConfigMembers{{Host: "a:1"}, {Host: "b:1"}},
			overrides:    []MemberOverride{{Host: "a:1"}, {Host: "b:1"}},
			wantExisting: []string{"a:1", "b:1"}},
		{name: "no blocks", // SHARD-T15
			live: ConfigMembers{{Host: "a:1"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			existing, missing := PartitionMemberOverrides(tc.live, tc.overrides)
			assertOverrideHosts(t, "existing", existing, tc.wantExisting...)
			assertOverrideHosts(t, "missing", missing, tc.wantMissing...)
		})
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

// SHARD-T17, SHARD-T18: SHARD-015 — BuildConfigMember carries every field,
// including priority 0; a block without tags yields no tags document
func TestBuildConfigMember(t *testing.T) {
	cases := []struct {
		name     string
		override MemberOverride
		id       int
		check    func(t *testing.T, m ConfigMember)
	}{
		{name: "all fields", // SHARD-T17
			override: MemberOverride{
				Host: "mongo3:27017", Priority: 0, Votes: 0, Hidden: true, ArbiterOnly: false, BuildIndexes: false,
				Tags: map[string]string{"dc": "east", "rack": "r1"},
			},
			id: 7,
			check: func(t *testing.T, m ConfigMember) {
				if m.ID != 7 || m.Host != "mongo3:27017" || m.Priority != 0 {
					t.Errorf("want _id 7 mongo3:27017 priority 0, got %+v", m)
				}
				if derefInt(m.Votes) != 0 || m.Votes == nil {
					t.Errorf("votes: want explicit 0, got %v", m.Votes)
				}
				if !derefBool(m.Hidden) || derefBool(m.ArbiterOnly) {
					t.Errorf("want hidden and not arbiterOnly, got %+v", m)
				}
				if m.BuildIndexes == nil || *m.BuildIndexes {
					t.Error("buildIndexes: want explicit false")
				}
				if m.Tags["dc"] != "east" || m.Tags["rack"] != "r1" {
					t.Errorf("tags: want {dc:east, rack:r1}, got %v", m.Tags)
				}
			}},
		{name: "no tags", // SHARD-T18
			override: MemberOverride{Host: "mongo1:27017", Priority: 1, Votes: 1, BuildIndexes: true},
			check: func(t *testing.T, m ConfigMember) {
				if m.Tags != nil {
					t.Errorf("tags: want nil, got %v", m.Tags)
				}
				if m.ID != 0 || m.Priority != 1 || derefInt(m.Votes) != 1 || !derefBool(m.BuildIndexes) {
					t.Errorf("unexpected member: %+v", m)
				}
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, BuildConfigMember(tc.override, tc.id))
		})
	}
}

// AddMembersSequentially against a scripted primary. Each case starts from a
// set of mongo1 alone unless it names its own members; see assertRun for the
// outcome fields.
func TestAddMembersSequentially(t *testing.T) {
	canceled := []string{context.Canceled.Error()}
	mongo2Voter := []MemberOverride{{Host: "mongo2:27017", Votes: 1, Priority: 1}}
	three := []MemberOverride{{Host: "mongo2:27017"}, {Host: "mongo3:27017"}, {Host: "mongo4:27017"}}
	noRemoval := func(t *testing.T, _ *fakeMemberAddServer, _ []memberPending, err error) {
		if strings.Contains(err.Error(), "removed") {
			t.Errorf("nothing was installed or observed, so nothing may be reported removed: %v", err)
		}
	}
	canceledNoRemoval := func(t *testing.T, srv *fakeMemberAddServer, pending []memberPending, err error) {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want the bare cancellation, got %v", err)
		}
		noRemoval(t, srv, pending, err)
	}

	cases := []struct {
		name      string
		members   []ConfigMember
		setup     func(srv *fakeMemberAddServer)
		overrides []MemberOverride
		wantErr   []string
		reconfigs int
		live      []string
		check     func(t *testing.T, srv *fakeMemberAddServer, pending []memberPending, err error)
	}{
		{name: "one reconfig per step", // SHARD-T19: SHARD-013, SHARD-014, SHARD-022 — in block order, ids from the re-read config
			overrides: []MemberOverride{
				{Host: "mongo2:27017", Priority: 1, Votes: 1, BuildIndexes: true},
				{Host: "mongo3:27017", Priority: 0, Votes: 0, Hidden: true, BuildIndexes: true},
			},
			reconfigs: 3, // add mongo2 staged, promote mongo2, add mongo3 (already a non-voter)
			check: func(t *testing.T, srv *fakeMemberAddServer, _ []memberPending, _ error) {
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
				assertWaits(t, srv.waited, waitCall{"mongo2:27017", waitSecondary}, waitCall{"mongo3:27017", waitReachable})
			}},
		{name: "follows live config", // SHARD-T20: SHARD-014 — ids and versions follow the live config, including changes made between steps
			members: []ConfigMember{{ID: 0, Host: "a:1"}, {ID: 4, Host: "b:1"}},
			setup: func(srv *fakeMemberAddServer) {
				// Simulate the server reconfiguring on its own (e.g. clearing
				// newlyAdded) while we wait for the member.
				srv.wait = func(string, memberWaitTarget) memberWaitResult {
					srv.current.Version += 3
					return memberWaitResult{Met: true, EverReachable: true}
				}
			},
			overrides: []MemberOverride{{Host: "c:1"}, {Host: "d:1"}},
			reconfigs: 2,
			check: func(t *testing.T, srv *fakeMemberAddServer, _ []memberPending, _ error) {
				if c, d := srv.reconfigs[0].Members[2].ID, srv.reconfigs[1].Members[3].ID; c != 5 || d != 6 {
					t.Errorf("_ids: want c 5 (highest 4 + 1) then d 6, got %d then %d", c, d)
				}
				if v1, v2 := srv.reconfigs[0].Version, srv.reconfigs[1].Version; v1 != 6 || v2 != 10 {
					t.Errorf("versions: want 6 then 10 (6, +3 by the server, +1), got %d then %d", v1, v2)
				}
			}},
		{name: "stops on reconfig error", // SHARD-T21: SHARD-013 — a failed reconfig stops the loop and names the member
			setup: func(srv *fakeMemberAddServer) {
				srv.setErr = func(cfg *RSConfig) error {
					for _, m := range cfg.Members {
						if m.Host == "mongo3:27017" {
							return errors.New("Non force replica set reconfig can only add or remove at most 1 voting member")
						}
					}
					return nil
				}
			},
			overrides: three,
			wantErr:   []string{"mongo3:27017"},
			reconfigs: 1, // the loop stops after the failed add
			live:      []string{"mongo1:27017", "mongo2:27017"},
			check:     noRemoval},
		{name: "rolls back unreachable member", // SHARD-T22: SHARD-017 — a member that never becomes reachable is removed again and the add fails
			setup: func(srv *fakeMemberAddServer) {
				srv.wait = func(host string, target memberWaitTarget) memberWaitResult {
					if host == "mongo3:27017" {
						return waitNotListed(host, target)
					}
					return memberWaitResult{Met: true, EverReachable: true}
				}
			},
			overrides: three,
			wantErr:   []string{"mongo3:27017", "removed again", "not listed", "1m0s"},
			reconfigs: 3, // add, add, remove
			live:      []string{"mongo1:27017", "mongo2:27017"},
			check: func(t *testing.T, srv *fakeMemberAddServer, _ []memberPending, _ error) {
				assertHosts(t, "rollback reconfig", srv.reconfigs[2].Members, "mongo1:27017", "mongo2:27017")
				if srv.reconfigs[2].Version != 8 {
					t.Errorf("rollback version: want 8, got %d", srv.reconfigs[2].Version)
				}
			}},
		{name: "reports rollback failure", // SHARD-T23: SHARD-017 — when the rollback also fails, both errors are reported
			setup: func(srv *fakeMemberAddServer) {
				srv.wait = waitUnreachable
				srv.setErr = func(cfg *RSConfig) error {
					if len(cfg.Members) < len(srv.current.Members) {
						return errors.New("not primary")
					}
					return nil
				}
			},
			overrides: []MemberOverride{{Host: "mongo2:27017"}},
			wantErr:   []string{"did not become reachable", "health=0", "removing it again also failed", "not primary"},
			reconfigs: 1,
			live:      []string{"mongo1:27017", "mongo2:27017"}},
		{name: "no members", // SHARD-T24: SHARD-013 — no missing members means no server interaction at all
			check: func(t *testing.T, srv *fakeMemberAddServer, _ []memberPending, _ error) {
				if srv.getCalls != 0 || len(srv.waited) != 0 {
					t.Errorf("want no reads or waits, got %d/%d", srv.getCalls, len(srv.waited))
				}
			}},
		{name: "stages voter then promotes", // SHARD-T26: SHARD-022 — a voter is staged with votes 0 / priority 0 and every other field, then promoted
			overrides: []MemberOverride{{Host: "mongo2:27017", Priority: 2, Votes: 1, BuildIndexes: true, Tags: map[string]string{"dc": "east"}}},
			reconfigs: 2, // add then promote
			check: func(t *testing.T, srv *fakeMemberAddServer, _ []memberPending, _ error) {
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
			}},
		{name: "arbiter added directly", // SHARD-T27: SHARD-022 — arbiters cannot be staged: added in final form, waited on for ARBITER, never promoted
			overrides: []MemberOverride{{Host: "arb:27017", Priority: 0, Votes: 1, ArbiterOnly: true, BuildIndexes: true}},
			reconfigs: 1,
			check: func(t *testing.T, srv *fakeMemberAddServer, _ []memberPending, _ error) {
				arb := srv.reconfigs[0].Members[1]
				if !derefBool(arb.ArbiterOnly) || derefInt(arb.Votes) != 1 || arb.Priority != 0 {
					t.Errorf("arbiter must be added with arbiterOnly, votes 1, priority 0, got %+v", arb)
				}
				assertWaits(t, srv.waited, waitCall{"arb:27017", waitArbiter})
			}},
		{name: "keeps syncing member", // SHARD-T28: SHARD-016, SHARD-017 — a reachable member still syncing at the timeout is kept as a non-voter
			setup: func(srv *fakeMemberAddServer) {
				srv.wait = func(host string, _ memberWaitTarget) memberWaitResult {
					if host == "mongo2:27017" {
						return memberWaitResult{EverReachable: true, Observed: true, Last: "health=1 state=STARTUP2"}
					}
					return memberWaitResult{Met: true, EverReachable: true, Observed: true}
				}
			},
			overrides: []MemberOverride{{Host: "mongo2:27017", Priority: 2, Votes: 1}, {Host: "mongo3:27017", Priority: 1, Votes: 1}},
			reconfigs: 3, // staged add of mongo2, no promotion; staged add of mongo3 and its promotion
			check: func(t *testing.T, srv *fakeMemberAddServer, pending []memberPending, _ error) {
				if len(pending) != 1 || pending[0].Host != "mongo2:27017" || pending[0].Votes != 1 || pending[0].Priority != 2 || pending[0].Target != waitSecondary {
					t.Fatalf("want mongo2 pending with its configured votes and priority, got %+v", pending)
				}
				for _, want := range []string{"mongo2:27017", "STARTUP2", "non-voter", "votes 1 and priority 2", "1m0s"} {
					if !strings.Contains(pending[0].Detail(), want) {
						t.Errorf("warning detail should contain %q, got: %s", want, pending[0].Detail())
					}
				}
				kept := srv.member("mongo2:27017")
				if kept.Host == "" || derefInt(kept.Votes) != 0 || kept.Priority != 0 {
					t.Errorf("syncing member should stay in the set as a non-voter, got %+v", kept)
				}
				next := srv.member("mongo3:27017")
				if next.Host == "" || derefInt(next.Votes) != 1 || next.Priority != 1 {
					t.Errorf("the following member should be added and promoted, got %+v", next)
				}
			}},
		// SHARD-T39: SHARD-017 — when replSetGetStatus never answered, nothing is
		// known about the member, so it is left in place and the wait is reported
		// as inconclusive rather than rolled back as unreachable
		{name: "status unreadable at timeout leaves member",
			setup:     func(srv *fakeMemberAddServer) { srv.wait = waitStatusUnreadable },
			overrides: mongo2Voter,
			wantErr:   []string{"mongo2:27017", "could not be read", "connection reset", "left in the replica set"},
			reconfigs: 1, // only the staged add
			live:      []string{"mongo1:27017", "mongo2:27017"},
			check:     noRemoval},
		{name: "status unreadable when interrupted leaves member",
			setup:     func(srv *fakeMemberAddServer) { srv.waitErr, srv.wait = context.Canceled, waitStatusUnreadable },
			overrides: mongo2Voter,
			wantErr:   canceled,
			reconfigs: 1,
			live:      []string{"mongo1:27017", "mongo2:27017"},
			check:     canceledNoRemoval},
		{name: "removes member installed by failed reconfig", // SHARD-T33: SHARD-017 — an add whose reconfig fails after the config was installed is removed again
			setup: func(srv *fakeMemberAddServer) {
				installed := false
				srv.setErr = func(cfg *RSConfig) error {
					if !installed && len(cfg.Members) == 2 {
						installed = true
						srv.current = copyRSConfig(*cfg)
						return errors.New("Reconfig finished but failed to propagate to a majority")
					}
					return nil
				}
			},
			overrides: []MemberOverride{{Host: "mongo2:27017"}},
			wantErr:   []string{"after the config was installed, so it was removed again"},
			reconfigs: 1, // the removal; the failed add was never acknowledged
			live:      []string{"mongo1:27017"},
			check: func(t *testing.T, srv *fakeMemberAddServer, _ []memberPending, _ error) {
				if len(srv.waited) != 0 {
					t.Errorf("no wait should run after a failed reconfig, got %v", srv.waited)
				}
			}},
		// SHARD-T34: SHARD-017 — an interrupted wait removes a member that never
		// answered, keeps one that did
		{name: "interrupted wait removes never-reachable member",
			setup:     func(srv *fakeMemberAddServer) { srv.waitErr, srv.wait = context.Canceled, waitNotListed },
			overrides: mongo2Voter,
			wantErr:   append(canceled, "removed again"),
			reconfigs: 2, // add then remove
			live:      []string{"mongo1:27017"},
			check: func(t *testing.T, _ *fakeMemberAddServer, _ []memberPending, err error) {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("want the cancellation wrapped with a removal note, got %v", err)
				}
			}},
		{name: "interrupted wait keeps reachable member",
			setup: func(srv *fakeMemberAddServer) {
				srv.waitErr = context.Canceled
				srv.wait = func(string, memberWaitTarget) memberWaitResult {
					return memberWaitResult{EverReachable: true, Last: "health=1 state=STARTUP2"}
				}
			},
			overrides: mongo2Voter,
			wantErr:   canceled,
			reconfigs: 1, // only the staged add
			live:      []string{"mongo1:27017", "mongo2:27017"},
			check:     canceledNoRemoval},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			members := tc.members
			if members == nil {
				members = []ConfigMember{{ID: 0, Host: "mongo1:27017"}}
			}
			srv := newFakeMemberAddServer(members...)
			if tc.setup != nil {
				tc.setup(srv)
			}
			pending, err := AddMembersSequentially(context.Background(), tc.overrides, srv.ops())
			assertRun(t, srv, err, tc.wantErr, tc.reconfigs, tc.live)
			if tc.check != nil {
				tc.check(t, srv, pending, err)
			}
		})
	}
}

// SHARD-T40: SHARD-013, SHARD-023 — ReconcileMembers runs promotions before
// adds, gathers pending members from both, and skips the adds after a hard
// promotion failure
func TestReconcileMembers(t *testing.T) {
	t.Run("promotions first, pending gathered", func(t *testing.T) {
		srv := newFakeMemberAddServer(
			ConfigMember{ID: 0, Host: "mongo1:27017", Votes: intPtr(1), Priority: 1},
			ConfigMember{ID: 1, Host: "mongo2:27017", Votes: intPtr(0), Priority: 0},
		)
		srv.wait = func(host string, _ memberWaitTarget) memberWaitResult {
			if host == "mongo3:27017" {
				return memberWaitResult{EverReachable: true, Observed: true, Last: "health=1 state=STARTUP2"}
			}
			return memberWaitResult{Met: true, EverReachable: true, Observed: true}
		}

		pending, err := ReconcileMembers(context.Background(),
			[]MemberOverride{{Host: "mongo2:27017", Votes: 1, Priority: 1}},
			[]MemberOverride{{Host: "mongo3:27017", Votes: 1, Priority: 1}},
			srv.ops())

		assertRun(t, srv, err, nil, 2, nil) // promotion of mongo2, then staged add of mongo3
		if len(pending) != 1 || pending[0].Host != "mongo3:27017" {
			t.Errorf("want only mongo3 pending, got %+v", pending)
		}
		if m := srv.reconfigs[0].Members[1]; derefInt(m.Votes) != 1 {
			t.Errorf("first reconfig should promote mongo2, got %+v", m)
		}
		assertHosts(t, "second reconfig", srv.reconfigs[1].Members, "mongo1:27017", "mongo2:27017", "mongo3:27017")
	})

	t.Run("hard promotion failure stops before the adds", func(t *testing.T) {
		srv := newFakeMemberAddServer(ConfigMember{ID: 0, Host: "mongo1:27017", Votes: intPtr(1), Priority: 1})

		_, err := ReconcileMembers(context.Background(),
			[]MemberOverride{{Host: "gone:27017", Votes: 1, Priority: 1}},
			[]MemberOverride{{Host: "mongo3:27017"}},
			srv.ops())

		assertRun(t, srv, err, []string{"gone:27017"}, 0, nil) // no add may run after a failed promotion
	})
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
	mongo1Voter := ConfigMember{ID: 0, Host: "mongo1:27017", Votes: intPtr(1), Priority: 1}
	mongo2NonVoter := ConfigMember{ID: 1, Host: "mongo2:27017", Votes: intPtr(0), Priority: 0}
	promoteMongo2 := []MemberOverride{{Host: "mongo2:27017", Votes: 1, Priority: 1}}
	cases := []struct {
		name      string
		members   []ConfigMember
		wait      func(host string, target memberWaitTarget) memberWaitResult
		overrides []MemberOverride
		wantErr   []string
		reconfigs int
		check     func(t *testing.T, srv *fakeMemberAddServer, pending []memberPending, err error)
	}{
		{name: "promotes after SECONDARY",
			members:   []ConfigMember{mongo1Voter, mongo2NonVoter},
			overrides: promoteMongo2,
			reconfigs: 1,
			check: func(t *testing.T, srv *fakeMemberAddServer, _ []memberPending, _ error) {
				if srv.reconfigs[0].Version != 6 {
					t.Fatalf("want the reconfig at version 6, got %+v", srv.reconfigs)
				}
				m := srv.member("mongo2:27017")
				if derefInt(m.Votes) != 1 || m.Priority != 1 || m.ID != 1 {
					t.Errorf("want votes 1 priority 1 on the same _id, got %+v", m)
				}
				assertWaits(t, srv.waited, waitCall{"mongo2:27017", waitSecondary})
			}},
		{name: "no reconfig when already matching",
			members:   []ConfigMember{{ID: 1, Host: "mongo2:27017", Votes: intPtr(1), Priority: 1}},
			overrides: promoteMongo2},
		{name: "still syncing is pending, not an error",
			members: []ConfigMember{mongo2NonVoter},
			wait: func(string, memberWaitTarget) memberWaitResult {
				return memberWaitResult{EverReachable: true, Observed: true, Last: "health=1 state=RECOVERING"}
			},
			overrides: []MemberOverride{{Host: "mongo2:27017", Votes: 1, Priority: 3}},
			reconfigs: 0, // nothing may run before the member is SECONDARY
			check: func(t *testing.T, srv *fakeMemberAddServer, pending []memberPending, _ error) {
				if len(pending) != 1 || pending[0].Host != "mongo2:27017" || pending[0].Votes != 1 || pending[0].Priority != 3 {
					t.Fatalf("want mongo2 pending with votes 1 priority 3, got %+v", pending)
				}
				if !strings.Contains(pending[0].Summary(), "mongo2:27017") || !strings.Contains(pending[0].Detail(), "RECOVERING") {
					t.Errorf("warning should name the host and the observed state, got %q / %q", pending[0].Summary(), pending[0].Detail())
				}
				if derefInt(srv.member("mongo2:27017").Votes) != 0 {
					t.Errorf("member must still be a non-voter, got %+v", srv.member("mongo2:27017"))
				}
			}},
		{name: "unreachable live member is reported, not removed",
			members:   []ConfigMember{mongo2NonVoter},
			wait:      waitUnreachable,
			overrides: promoteMongo2,
			wantErr:   []string{"did not become reachable"},
			check: func(t *testing.T, srv *fakeMemberAddServer, _ []memberPending, err error) {
				if strings.Contains(err.Error(), "removed") || srv.member("mongo2:27017").Host == "" {
					t.Errorf("a live member must never be removed, got %v", err)
				}
			}},
		{name: "member gone from the config",
			members:   []ConfigMember{mongo1Voter},
			overrides: promoteMongo2,
			wantErr:   []string{"no longer in the replica set configuration"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newFakeMemberAddServer(tc.members...)
			srv.wait = tc.wait
			pending, err := PromoteMembersSequentially(context.Background(), tc.overrides, srv.ops())
			assertRun(t, srv, err, tc.wantErr, tc.reconfigs, nil)
			if tc.check != nil {
				tc.check(t, srv, pending, err)
			}
		})
	}
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
		t.Run(tc.host+"/"+tc.target.String(), func(t *testing.T) {
			obs := observeMember(status, tc.host, tc.target)
			if obs.Met != tc.met || obs.Reachable != tc.reachable || !strings.Contains(obs.Seen, tc.seen) {
				t.Errorf("want met=%v reachable=%v seen~%q, got %+v", tc.met, tc.reachable, tc.seen, obs)
			}
		})
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
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name    string
		ctx     context.Context
		get     func(context.Context) (*ReplSetStatus, error)
		target  memberWaitTarget
		timeout time.Duration
		check   func(t *testing.T, res memberWaitResult, err error)
	}{
		{name: "reaches SECONDARY through STARTUP2",
			get:     script(nil, &ReplSetStatus{}, row(MemberHealthDown, MemberStateUnknown), row(MemberHealthUp, MemberStateStartup2), row(MemberHealthUp, MemberStateSecondary)),
			target:  waitSecondary,
			timeout: time.Second,
			check: func(t *testing.T, res memberWaitResult, err error) {
				if err != nil || !res.Met || !res.EverReachable || !strings.Contains(res.Last, "SECONDARY") {
					t.Fatalf("want met and reachable, got %+v err %v", res, err)
				}
			}},
		{name: "times out never reachable",
			get:     script(row(MemberHealthDown, MemberStateUnknown)),
			target:  waitReachable,
			timeout: 5 * poll,
			check: func(t *testing.T, res memberWaitResult, err error) {
				if err != nil || res.Met || res.EverReachable || !res.Observed || !strings.Contains(res.Last, "health=0") {
					t.Fatalf("want a clean timeout with the member observed down, got %+v err %v", res, err)
				}
			}},
		{name: "status never readable is not an observation",
			get:     func(context.Context) (*ReplSetStatus, error) { return nil, errors.New("connection reset") },
			target:  waitReachable,
			timeout: 5 * poll,
			check: func(t *testing.T, res memberWaitResult, err error) {
				if err != nil || res.Met || res.EverReachable || res.Observed || !strings.Contains(res.Last, "replSetGetStatus: connection reset") {
					t.Fatalf("want a timeout with nothing observed, got %+v err %v", res, err)
				}
			}},
		{name: "times out still syncing",
			get:     script(row(MemberHealthUp, MemberStateStartup2)),
			target:  waitSecondary,
			timeout: 5 * poll,
			check: func(t *testing.T, res memberWaitResult, err error) {
				if err != nil || res.Met || !res.EverReachable || !strings.Contains(res.Last, "STARTUP2") {
					t.Fatalf("want a timeout that remembers the member was reachable, got %+v err %v", res, err)
				}
			}},
		{name: "context cancellation is the only error",
			ctx:     canceled,
			get:     script(row(MemberHealthDown, MemberStateUnknown)),
			target:  waitReachable,
			timeout: time.Minute,
			check: func(t *testing.T, _ memberWaitResult, err error) {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("want context.Canceled, got %v", err)
				}
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			res, err := WaitForMemberState(ctx, tc.get, "m:1", tc.target, tc.timeout, poll)
			tc.check(t, res, err)
		})
	}
}
