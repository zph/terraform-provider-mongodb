package mongodb

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

// --- IsNotYetInitialized tests ---

// INIT-T01: IsNotYetInitialized true for mongo.CommandError{Code: 94}
func TestIsNotYetInitialized_Code94(t *testing.T) {
	err := mongo.CommandError{Code: MongoErrNotYetInitialized, Message: "no replset config"}
	if !IsNotYetInitialized(err) {
		t.Error("expected true for CommandError code 94")
	}
}

// INIT-T02: IsNotYetInitialized false for mongo.CommandError{Code: 99}
func TestIsNotYetInitialized_Code99(t *testing.T) {
	err := mongo.CommandError{Code: 99, Message: "other error"}
	if IsNotYetInitialized(err) {
		t.Error("expected false for CommandError code 99")
	}
}

// INIT-T03: IsNotYetInitialized false for plain fmt.Errorf
func TestIsNotYetInitialized_PlainError(t *testing.T) {
	err := fmt.Errorf("some plain error")
	if IsNotYetInitialized(err) {
		t.Error("expected false for plain error")
	}
}

// INIT-T04: IsNotYetInitialized true for pkgerrors.Wrap(CommandError{94})
func TestIsNotYetInitialized_WrappedCode94(t *testing.T) {
	inner := mongo.CommandError{Code: MongoErrNotYetInitialized, Message: "no replset config"}
	wrapped := errors.Wrap(inner, "replSetGetConfig")
	if !IsNotYetInitialized(wrapped) {
		t.Error("expected true for wrapped CommandError code 94")
	}
}

// --- IsAlreadyInitialized tests ---

// INIT-T05: IsAlreadyInitialized true for mongo.CommandError{Code: 23}
func TestIsAlreadyInitialized_Code23(t *testing.T) {
	err := mongo.CommandError{Code: MongoErrAlreadyInitialized, Message: "already initialized"}
	if !IsAlreadyInitialized(err) {
		t.Error("expected true for CommandError code 23")
	}
}

// INIT-T06: IsAlreadyInitialized false for code 94
func TestIsAlreadyInitialized_Code94(t *testing.T) {
	err := mongo.CommandError{Code: MongoErrNotYetInitialized, Message: "not yet initialized"}
	if IsAlreadyInitialized(err) {
		t.Error("expected false for CommandError code 94")
	}
}

// INIT-T07: IsAlreadyInitialized false for plain error
func TestIsAlreadyInitialized_PlainError(t *testing.T) {
	err := fmt.Errorf("some plain error")
	if IsAlreadyInitialized(err) {
		t.Error("expected false for plain error")
	}
}

// --- BuildInitialMembers tests ---

// INIT-T08: BuildInitialMembers assigns sequential IDs 0,1,2
func TestBuildInitialMembers_SequentialIDs(t *testing.T) {
	overrides := []MemberOverride{
		{Host: "mongo1:27017", Priority: 1, Votes: 1, BuildIndexes: true},
		{Host: "mongo2:27017", Priority: 1, Votes: 1, BuildIndexes: true},
		{Host: "mongo3:27017", Priority: 1, Votes: 1, BuildIndexes: true},
	}
	members := BuildInitialMembers(overrides)
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}
	for i, m := range members {
		if m.ID != i {
			t.Errorf("member %d: expected _id=%d, got %d", i, i, m.ID)
		}
		if m.Host != overrides[i].Host {
			t.Errorf("member %d: expected host=%s, got %s", i, overrides[i].Host, m.Host)
		}
	}
}

// INIT-T09: BuildInitialMembers applies all fields (priority, votes, hidden, arbiter_only, build_indexes, tags)
func TestBuildInitialMembers_AllFields(t *testing.T) {
	overrides := []MemberOverride{
		{
			Host:         "mongo1:27017",
			Priority:     10,
			Votes:        1,
			Hidden:       true,
			ArbiterOnly:  false,
			BuildIndexes: false,
			Tags:         map[string]string{"dc": "east", "rack": "r1"},
		},
	}
	members := BuildInitialMembers(overrides)
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	m := members[0]
	if m.ID != 0 {
		t.Errorf("_id: want 0, got %d", m.ID)
	}
	if m.Host != "mongo1:27017" {
		t.Errorf("host: want mongo1:27017, got %s", m.Host)
	}
	if m.Priority != 10 {
		t.Errorf("priority: want 10, got %d", m.Priority)
	}
	if derefInt(m.Votes) != 1 {
		t.Errorf("votes: want 1, got %d", derefInt(m.Votes))
	}
	if derefBool(m.Hidden) != true {
		t.Errorf("hidden: want true, got %v", derefBool(m.Hidden))
	}
	if derefBool(m.ArbiterOnly) != false {
		t.Errorf("arbiterOnly: want false, got %v", derefBool(m.ArbiterOnly))
	}
	if derefBool(m.BuildIndexes) != false {
		t.Errorf("buildIndexes: want false, got %v", derefBool(m.BuildIndexes))
	}
	if m.Tags["dc"] != "east" || m.Tags["rack"] != "r1" {
		t.Errorf("tags: want {dc:east, rack:r1}, got %v", m.Tags)
	}
}

// INIT-T10: BuildInitialMembers empty input returns empty ConfigMembers
func TestBuildInitialMembers_EmptyInput(t *testing.T) {
	members := BuildInitialMembers(nil)
	if len(members) != 0 {
		t.Errorf("expected 0 members for nil input, got %d", len(members))
	}
	members = BuildInitialMembers([]MemberOverride{})
	if len(members) != 0 {
		t.Errorf("expected 0 members for empty input, got %d", len(members))
	}
}

// INIT-T11: BuildInitialMembers single member gets _id: 0
func TestBuildInitialMembers_SingleMember(t *testing.T) {
	overrides := []MemberOverride{
		{Host: "mongo1:27017", Priority: 1, Votes: 1, BuildIndexes: true},
	}
	members := BuildInitialMembers(overrides)
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0].ID != 0 {
		t.Errorf("expected _id=0, got %d", members[0].ID)
	}
}

// INIT-T12: Schema: init_timeout_secs exists, Optional, Default 60
func TestShardConfigSchema_InitTimeoutSecs(t *testing.T) {
	res := resourceShardConfig()
	field, ok := res.Schema["init_timeout_secs"]
	if !ok {
		t.Fatal("schema missing 'init_timeout_secs' field")
	}
	if field.Required {
		t.Error("init_timeout_secs should be Optional, not Required")
	}
	if field.Default != DefaultInitTimeoutSecs {
		t.Errorf("init_timeout_secs default: want %d, got %v", DefaultInitTimeoutSecs, field.Default)
	}
}
