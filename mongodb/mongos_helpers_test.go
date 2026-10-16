package mongodb

import (
	"testing"
)

// BAL-012, CBAL-010: requireMongos is an integration-only function that needs
// a live mongo.Client. Unit tests verify the underlying classifyConnectionType
// logic instead. Also covers DISC-T18..T20.
func TestClassifyConnectionType(t *testing.T) {
	cases := []struct {
		name string
		resp *IsMasterResp
		want ConnectionType
	}{
		{"DISC-T18 mongos from msg", &IsMasterResp{Msg: "isdbgrid"}, ConnTypeMongos},
		{"DISC-T19 replica set from setName", &IsMasterResp{SetName: "rs0"}, ConnTypeReplicaSet},
		{"DISC-T20 standalone", &IsMasterResp{}, ConnTypeStandalone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyConnectionType(tc.resp); got != tc.want {
				t.Errorf("want %s, got %s", tc.want, got)
			}
		})
	}
}
