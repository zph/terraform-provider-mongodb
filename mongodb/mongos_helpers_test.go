package mongodb

import (
	"testing"
)

// BAL-012, CBAL-010: requireMongos is an integration-only function that needs a live
// mongo.Client. Unit tests verify the underlying classifyConnectionType logic instead.

func TestClassifyConnectionType_Mongos(t *testing.T) {
	resp := &IsMasterResp{Msg: "isdbgrid"}
	got := classifyConnectionType(resp)
	if got != ConnTypeMongos {
		t.Errorf("expected ConnTypeMongos, got %s", got)
	}
}

func TestClassifyConnectionType_ReplicaSet(t *testing.T) {
	resp := &IsMasterResp{SetName: "rs0"}
	got := classifyConnectionType(resp)
	if got != ConnTypeReplicaSet {
		t.Errorf("expected ConnTypeReplicaSet, got %s", got)
	}
}

func TestClassifyConnectionType_Standalone(t *testing.T) {
	resp := &IsMasterResp{}
	got := classifyConnectionType(resp)
	if got != ConnTypeStandalone {
		t.Errorf("expected ConnTypeStandalone, got %s", got)
	}
}
