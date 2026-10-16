package mongodb

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestGetSelf(t *testing.T) {
	cases := []struct {
		name    string
		members []*Member
		wantID  int
		wantNil bool
	}{
		// TEST-017: returns the member with Self=true
		{"found", []*Member{{ID: 0, Name: "node0"}, {ID: 1, Name: "node1", Self: true}, {ID: 2, Name: "node2"}}, 1, false},
		// TEST-018: nil when no member has Self=true
		{"not found", []*Member{{ID: 0}, {ID: 1}}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			self := (&ReplSetStatus{Members: tc.members}).GetSelf()
			if tc.wantNil {
				if self != nil {
					t.Errorf("expected nil, got member ID %d", self.ID)
				}
				return
			}
			if self == nil {
				t.Fatal("expected non-nil member")
			}
			if self.ID != tc.wantID {
				t.Errorf("expected member ID %d, got %d", tc.wantID, self.ID)
			}
		})
	}
}

func TestGetMembersByState(t *testing.T) {
	cases := []struct {
		name    string
		members []*Member
		state   MemberState
		limit   int
		wantLen int
	}{
		// TEST-019: limit=0 returns all matching
		{"no limit", []*Member{{ID: 0, State: MemberStatePrimary}, {ID: 1, State: MemberStateSecondary}, {ID: 2, State: MemberStateSecondary}}, MemberStateSecondary, 0, 2},
		// TEST-020: positive limit caps results
		{"with limit", []*Member{{ID: 0, State: MemberStateSecondary}, {ID: 1, State: MemberStateSecondary}, {ID: 2, State: MemberStateSecondary}}, MemberStateSecondary, 1, 1},
		// TEST-021: no matches returns empty slice
		{"no match", []*Member{{ID: 0, State: MemberStateSecondary}, {ID: 1, State: MemberStateSecondary}}, MemberStatePrimary, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			members := (&ReplSetStatus{Members: tc.members}).GetMembersByState(tc.state, tc.limit)
			if len(members) != tc.wantLen {
				t.Errorf("expected %d members, got %d", tc.wantLen, len(members))
			}
		})
	}
}

func TestPrimary(t *testing.T) {
	cases := []struct {
		name    string
		members []*Member
		wantNil bool
	}{
		// TEST-022: returns the primary member when one exists
		{"found", []*Member{{ID: 0, State: MemberStatePrimary}, {ID: 1, State: MemberStateSecondary}, {ID: 2, State: MemberStateSecondary}}, false},
		// TEST-023: nil when no primary exists
		{"not found", []*Member{{ID: 0, State: MemberStateSecondary}, {ID: 1, State: MemberStateSecondary}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			primary := (&ReplSetStatus{Members: tc.members}).Primary()
			if tc.wantNil {
				if primary != nil {
					t.Errorf("expected nil, got member ID %d", primary.ID)
				}
				return
			}
			if primary == nil {
				t.Fatal("expected non-nil primary")
			}
			if primary.State != MemberStatePrimary {
				t.Errorf("expected PRIMARY state, got %d", primary.State)
			}
		})
	}
}

// TEST-024: MemberStateStrings covers all defined MemberState constants
func TestMemberStateStrings_Coverage(t *testing.T) {
	states := []MemberState{
		MemberStateStartup,
		MemberStatePrimary,
		MemberStateSecondary,
		MemberStateRecovering,
		MemberStateStartup2,
		MemberStateUnknown,
		MemberStateArbiter,
		MemberStateDown,
		MemberStateRollback,
		MemberStateRemoved,
	}
	for _, state := range states {
		if _, ok := MemberStateStrings[state]; !ok {
			t.Errorf("MemberStateStrings missing entry for state %d", state)
		}
	}
}

// TEST-025: Replica set constants match expected values
func TestReplicaSetConstants(t *testing.T) {
	cases := []struct {
		name     string
		got      int
		expected int
	}{
		{"MinVotingMembers", MinVotingMembers, 1},
		{"MaxVotingMembers", MaxVotingMembers, 7},
		{"MaxMembers", MaxMembers, 50},
		{"DefaultPriority", DefaultPriority, 2},
		{"DefaultVotes", DefaultVotes, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.expected {
				t.Errorf("%s: expected %d, got %d", tc.name, tc.expected, tc.got)
			}
		})
	}
}

func TestConfigMemberBSON(t *testing.T) {
	cases := []struct {
		name   string
		member ConfigMember
		check  func(t *testing.T, original ConfigMember, raw bson.Raw)
	}{
		{
			// TEST-035: round-trip preserves values
			name:   "round trip",
			member: ConfigMember{ID: 0, Host: "localhost:27017", ArbiterOnly: boolPtr(false), Priority: 2.0, Votes: intPtr(1)},
			check: func(t *testing.T, original ConfigMember, raw bson.Raw) {
				var decoded ConfigMember
				if err := bson.Unmarshal(raw, &decoded); err != nil {
					t.Fatalf("unmarshal error: %v", err)
				}
				if decoded.ID != original.ID {
					t.Errorf("ID mismatch: got %d, want %d", decoded.ID, original.ID)
				}
				if decoded.Host != original.Host {
					t.Errorf("Host mismatch: got %s, want %s", decoded.Host, original.Host)
				}
				if decoded.Priority != original.Priority {
					t.Errorf("Priority mismatch: got %v, want %v", decoded.Priority, original.Priority)
				}
				if *decoded.ArbiterOnly != *original.ArbiterOnly {
					t.Errorf("ArbiterOnly mismatch: got %v, want %v", *decoded.ArbiterOnly, *original.ArbiterOnly)
				}
				if *decoded.Votes != *original.Votes {
					t.Errorf("Votes mismatch: got %d, want %d", *decoded.Votes, *original.Votes)
				}
			},
		},
		{
			// SHARD-020: priority 0 must reach the server. With omitempty the field was
			// dropped and MongoDB defaulted it to 1, which rejects hidden members and
			// silently makes a priority-0 member electable.
			name:   "priority zero is sent",
			member: ConfigMember{ID: 2, Host: "mongo3:27017", Hidden: boolPtr(true), Priority: 0, Votes: intPtr(0)},
			check: func(t *testing.T, _ ConfigMember, raw bson.Raw) {
				val, err := raw.LookupErr("priority")
				if err != nil {
					t.Fatalf("priority missing from marshaled member: %s", raw.String())
				}
				if got := val.Double(); got != 0 {
					t.Errorf("priority: want 0, got %v", got)
				}
				if v, err := raw.LookupErr("votes"); err != nil || v.Int32() != 0 {
					t.Errorf("votes: want explicit 0, got %v (err %v)", v, err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := bson.Marshal(tc.member)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			tc.check(t, tc.member, bson.Raw(data))
		})
	}
}

// TEST-036: RSConfig BSON round-trip with Settings and Members
func TestRSConfigBSONRoundTrip(t *testing.T) {
	votes := 1
	original := RSConfig{
		ID:      "rs0",
		Version: 3,
		Members: ConfigMembers{
			{ID: 0, Host: "localhost:27017", Priority: 2.0, Votes: &votes},
			{ID: 1, Host: "localhost:27018", Priority: 1.0, Votes: &votes},
		},
		Settings: Settings{
			ChainingAllowed:         true,
			HeartbeatIntervalMillis: 2000,
			HeartbeatTimeoutSecs:    10,
			ElectionTimeoutMillis:   10000,
		},
	}
	data, err := bson.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded RSConfig
	if err := bson.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID, original.ID)
	}
	if decoded.Version != original.Version {
		t.Errorf("Version mismatch: got %d, want %d", decoded.Version, original.Version)
	}
	if len(decoded.Members) != len(original.Members) {
		t.Fatalf("Members count mismatch: got %d, want %d", len(decoded.Members), len(original.Members))
	}
	for i, m := range decoded.Members {
		if m.Host != original.Members[i].Host {
			t.Errorf("Member[%d] Host mismatch: got %s, want %s", i, m.Host, original.Members[i].Host)
		}
	}
	if decoded.Settings.ElectionTimeoutMillis != original.Settings.ElectionTimeoutMillis {
		t.Errorf("ElectionTimeoutMillis mismatch: got %d, want %d",
			decoded.Settings.ElectionTimeoutMillis, original.Settings.ElectionTimeoutMillis)
	}
	if decoded.Settings.HeartbeatIntervalMillis != original.Settings.HeartbeatIntervalMillis {
		t.Errorf("HeartbeatIntervalMillis mismatch: got %d, want %d",
			decoded.Settings.HeartbeatIntervalMillis, original.Settings.HeartbeatIntervalMillis)
	}
}
