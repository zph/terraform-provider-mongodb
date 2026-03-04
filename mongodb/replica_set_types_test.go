package mongodb

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// TEST-017: GetSelf returns member with Self=true
func TestGetSelf_Found(t *testing.T) {
	status := &ReplSetStatus{
		Members: []*Member{
			{ID: 0, Name: "node0", Self: false},
			{ID: 1, Name: "node1", Self: true},
			{ID: 2, Name: "node2", Self: false},
		},
	}
	self := status.GetSelf()
	if self == nil {
		t.Fatal("expected non-nil member")
	}
	if self.ID != 1 {
		t.Errorf("expected member ID 1, got %d", self.ID)
	}
}

// TEST-018: GetSelf returns nil when no member has Self=true
func TestGetSelf_NotFound(t *testing.T) {
	status := &ReplSetStatus{
		Members: []*Member{
			{ID: 0, Self: false},
			{ID: 1, Self: false},
		},
	}
	self := status.GetSelf()
	if self != nil {
		t.Errorf("expected nil, got member ID %d", self.ID)
	}
}

// TEST-019: GetMembersByState with limit=0 returns all matching
func TestGetMembersByState_NoLimit(t *testing.T) {
	status := &ReplSetStatus{
		Members: []*Member{
			{ID: 0, State: MemberStatePrimary},
			{ID: 1, State: MemberStateSecondary},
			{ID: 2, State: MemberStateSecondary},
		},
	}
	members := status.GetMembersByState(MemberStateSecondary, 0)
	if len(members) != 2 {
		t.Errorf("expected 2 members, got %d", len(members))
	}
}

// TEST-020: GetMembersByState with positive limit caps results
func TestGetMembersByState_WithLimit(t *testing.T) {
	status := &ReplSetStatus{
		Members: []*Member{
			{ID: 0, State: MemberStateSecondary},
			{ID: 1, State: MemberStateSecondary},
			{ID: 2, State: MemberStateSecondary},
		},
	}
	members := status.GetMembersByState(MemberStateSecondary, 1)
	if len(members) != 1 {
		t.Errorf("expected 1 member, got %d", len(members))
	}
}

// TEST-021: GetMembersByState with no matches returns empty slice
func TestGetMembersByState_NoMatch(t *testing.T) {
	status := &ReplSetStatus{
		Members: []*Member{
			{ID: 0, State: MemberStateSecondary},
			{ID: 1, State: MemberStateSecondary},
		},
	}
	members := status.GetMembersByState(MemberStatePrimary, 0)
	if len(members) != 0 {
		t.Errorf("expected 0 members, got %d", len(members))
	}
}

// TEST-022: Primary returns the primary member when one exists
func TestPrimary_Found(t *testing.T) {
	status := &ReplSetStatus{
		Members: []*Member{
			{ID: 0, State: MemberStatePrimary},
			{ID: 1, State: MemberStateSecondary},
			{ID: 2, State: MemberStateSecondary},
		},
	}
	primary := status.Primary()
	if primary == nil {
		t.Fatal("expected non-nil primary")
	}
	if primary.State != MemberStatePrimary {
		t.Errorf("expected PRIMARY state, got %d", primary.State)
	}
}

// TEST-023: Primary returns nil when no primary exists
func TestPrimary_NotFound(t *testing.T) {
	status := &ReplSetStatus{
		Members: []*Member{
			{ID: 0, State: MemberStateSecondary},
			{ID: 1, State: MemberStateSecondary},
		},
	}
	primary := status.Primary()
	if primary != nil {
		t.Errorf("expected nil, got member ID %d", primary.ID)
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

// TEST-035: ConfigMember BSON round-trip preserves values
func TestConfigMemberBSONRoundTrip(t *testing.T) {
	arbiter := false
	votes := 1
	original := ConfigMember{
		ID:          0,
		Host:        "localhost:27017",
		ArbiterOnly: &arbiter,
		Priority:    2.0,
		Votes:       &votes,
	}
	data, err := bson.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded ConfigMember
	if err := bson.Unmarshal(data, &decoded); err != nil {
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
