//go:build integration

package mongodb

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/mongo"
)

// memberAddCluster is three mongods on their own network without auth. Only
// the first is initiated, as a one-member set; the other two wait to be
// added. Started lazily; tests skip if it cannot start.
var memberAddCluster struct {
	once     sync.Once
	err      error
	network  *testcontainers.DockerNetwork
	nodes    []*testcontainers.DockerContainer
	seedHost string
	seedPort string
}

const (
	memberAddRSName = "rsgrow"
	memberAddPort   = "27017"
)

// memberAddAliases are the members' names on the test network, and so the
// hosts in the replica set configuration.
var memberAddAliases = []string{"rsgrow-a", "rsgrow-b", "rsgrow-c"}

func memberAddHost(i int) string { return memberAddAliases[i] + ":" + memberAddPort }

func setupMemberAddCluster() error {
	ctx := context.Background()

	image := testMongoImage
	if env := os.Getenv("MONGO_TEST_IMAGE"); env != "" {
		image = env
	}

	nw, err := network.New(ctx)
	if err != nil {
		return fmt.Errorf("create network: %w", err)
	}
	memberAddCluster.network = nw

	natPort := nat.Port(memberAddPort + "/tcp")
	type result struct {
		i   int
		c   *testcontainers.DockerContainer
		err error
	}
	results := make(chan result, len(memberAddAliases))
	for i, alias := range memberAddAliases {
		go func(i int, alias string) {
			c, err := testcontainers.Run(ctx, image,
				network.WithNetwork([]string{alias}, nw),
				testcontainers.WithCmd("mongod", "--replSet", memberAddRSName, "--port", memberAddPort, "--bind_ip_all"),
				testcontainers.WithExposedPorts(string(natPort)),
				testcontainers.WithWaitStrategy(wait.ForListeningPort(natPort).WithStartupTimeout(120*time.Second)),
			)
			results <- result{i: i, c: c, err: err}
		}(i, alias)
	}
	memberAddCluster.nodes = make([]*testcontainers.DockerContainer, len(memberAddAliases))
	for range memberAddAliases {
		r := <-results
		if r.err != nil {
			return fmt.Errorf("start %s: %w", memberAddAliases[r.i], r.err)
		}
		memberAddCluster.nodes[r.i] = r.c
	}

	seed := memberAddCluster.nodes[0]
	if err := initRS(ctx, seed, memberAddRSName, memberAddAliases[0], memberAddPort); err != nil {
		return fmt.Errorf("init %s: %w", memberAddRSName, err)
	}
	if err := waitForPrimaryExec(ctx, seed, memberAddPort); err != nil {
		return fmt.Errorf("wait %s primary: %w", memberAddRSName, err)
	}

	host, err := seed.Host(ctx)
	if err != nil {
		return fmt.Errorf("seed host: %w", err)
	}
	mapped, err := seed.MappedPort(ctx, natPort)
	if err != nil {
		return fmt.Errorf("seed port: %w", err)
	}
	memberAddCluster.seedHost = host
	memberAddCluster.seedPort = mapped.Port()
	return nil
}

func ensureMemberAddCluster(t *testing.T) {
	t.Helper()
	memberAddCluster.once.Do(func() {
		memberAddCluster.err = setupMemberAddCluster()
	})
	if memberAddCluster.err != nil {
		t.Skipf("member-add replica set unavailable: %v", memberAddCluster.err)
	}
}

// teardownMemberAddCluster is called from TestMain.
func teardownMemberAddCluster() {
	ctx := context.Background()
	for _, c := range memberAddCluster.nodes {
		if c != nil {
			_ = c.Terminate(ctx)
		}
	}
	if memberAddCluster.network != nil {
		_ = memberAddCluster.network.Remove(ctx)
	}
}

func memberAddProviderConf() *MongoDatabaseConfiguration {
	return &MongoDatabaseConfiguration{
		Config: &ClientConfig{
			Host:        memberAddCluster.seedHost,
			Port:        memberAddCluster.seedPort,
			DB:          "admin",
			Direct:      true,
			RetryWrites: false,
		},
		MaxConnLifetime: 10,
	}
}

// newMemberAddSeedClient connects directly to the initiated member without
// auth, as ConnectForInit's fallback would on a fresh set.
func newMemberAddSeedClient(t *testing.T) *mongo.Client {
	t.Helper()
	client, err := MongoClientInitNoAuth(context.Background(), memberAddProviderConf())
	if err != nil {
		t.Fatalf("MongoClientInitNoAuth failed: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	return client
}

// memberAddResourceData is the three-member configuration the tests apply:
// the seed member with a changed priority, a tagged voter and a hidden
// priority-0 member whose votes the caller picks.
func memberAddResourceData(t *testing.T, thirdVotes int) *schema.ResourceData {
	return schema.TestResourceDataRaw(t, resourceShardConfig().Schema, map[string]interface{}{
		"shard_name":        memberAddRSName,
		"init_timeout_secs": 120,
		"member": []interface{}{
			map[string]interface{}{"host": memberAddHost(0), "priority": 2.0, "votes": 1},
			map[string]interface{}{"host": memberAddHost(1), "priority": 1.0, "votes": 1,
				"tags": map[string]interface{}{"role": "online"}},
			map[string]interface{}{"host": memberAddHost(2), "priority": 0.0, "votes": thirdVotes, "hidden": true},
		},
	})
}

func waitForAllMembersHealthy(ctx context.Context, client *mongo.Client, want int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	last := "no status yet"
	for time.Now().Before(deadline) {
		status, err := GetReplSetStatus(ctx, client)
		if err != nil {
			last = err.Error()
		} else {
			healthy := 0
			var states []string
			for _, m := range status.Members {
				states = append(states, fmt.Sprintf("%s=%s", m.Name, m.StateStr))
				if m.Health == MemberHealthUp && (m.State == MemberStatePrimary || m.State == MemberStateSecondary) {
					healthy++
				}
			}
			if healthy == want {
				return nil
			}
			last = fmt.Sprint(states)
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("not all %d members became PRIMARY/SECONDARY within %s (last: %s)", want, timeout, last)
}

// INTEG-022: SHARD-012 through SHARD-018, SHARD-020, SHARD-022 — a one-member
// set grows to three: the voter is staged then promoted, the non-voter is
// added in one step, and every field is applied.
func TestIntegration_ShardConfigUpdate_AddsMembersOneAtATime(t *testing.T) {
	ensureMemberAddCluster(t)
	client := newMemberAddSeedClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	before, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig before: %v", err)
	}
	if len(before.Members) != 1 {
		t.Fatalf("precondition: want a one-member set, got %d members", len(before.Members))
	}

	data := memberAddResourceData(t, 0)
	if diags := RShardConfig.updateWithClient(ctx, data, client, memberAddProviderConf(), true); diags.HasError() {
		t.Fatalf("updateWithClient: %v", diags)
	}

	cfg, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after: %v", err)
	}
	if len(cfg.Members) != 3 {
		t.Fatalf("want 3 members, got %d: %v", len(cfg.Members), memberHosts(cfg.Members))
	}
	for i := 0; i < 3; i++ {
		if cfg.Members[i].ID != i || cfg.Members[i].Host != memberAddHost(i) {
			t.Errorf("member %d: want _id %d host %s, got _id %d host %s",
				i, i, memberAddHost(i), cfg.Members[i].ID, cfg.Members[i].Host)
		}
	}
	if cfg.Members[0].Priority != 2 {
		t.Errorf("seed member priority: want 2, got %v", cfg.Members[0].Priority)
	}
	if second := cfg.Members[1]; second.Priority != 1 || derefInt(second.Votes) != 1 || second.Tags["role"] != "online" {
		t.Errorf("second member: want priority 1, votes 1 and tag role=online after promotion, got %+v", second)
	}
	if third := cfg.Members[2]; !derefBool(third.Hidden) || third.Priority != 0 || derefInt(third.Votes) != 0 {
		t.Errorf("third member: want hidden, priority 0, votes 0, got %+v", third)
	}
	// Settings reconfig, staged add and promotion of the voter, one add for
	// the non-voter; 5.0+ may add newlyAdded reconfigs on top.
	if cfg.Version < before.Version+4 {
		t.Errorf("version: want at least %d, got %d", before.Version+4, cfg.Version)
	}

	stateMembers, ok := data.Get("member").([]interface{})
	if !ok || len(stateMembers) != 3 {
		t.Fatalf("state member list: want 3 entries, got %v", data.Get("member"))
	}
	for i, raw := range stateMembers {
		if host := raw.(map[string]interface{})["host"]; host != memberAddHost(i) {
			t.Errorf("state member %d: want host %s, got %v", i, memberAddHost(i), host)
		}
	}
	if data.Id() != memberAddRSName {
		t.Errorf("resource id: want %s, got %s", memberAddRSName, data.Id())
	}

	if err := waitForAllMembersHealthy(ctx, client, 3, 3*time.Minute); err != nil {
		t.Fatal(err)
	}
}

// INTEG-023: SHARD-012, SHARD-019 — a second apply adds nothing and keeps ids.
func TestIntegration_ShardConfigUpdate_SecondApplyAddsNothing(t *testing.T) {
	ensureMemberAddCluster(t)
	client := newMemberAddSeedClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	before, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig before: %v", err)
	}
	if len(before.Members) != 3 {
		t.Skipf("precondition: runs after the set has grown to 3 members, got %d", len(before.Members))
	}

	data := memberAddResourceData(t, 0)
	if diags := RShardConfig.updateWithClient(ctx, data, client, memberAddProviderConf(), true); diags.HasError() {
		t.Fatalf("updateWithClient: %v", diags)
	}

	after, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after: %v", err)
	}
	if len(after.Members) != 3 {
		t.Fatalf("want 3 members after second apply, got %d", len(after.Members))
	}
	for i := range after.Members {
		if after.Members[i].ID != before.Members[i].ID || after.Members[i].Host != before.Members[i].Host {
			t.Errorf("member %d changed: before %+v after %+v", i, before.Members[i], after.Members[i])
		}
	}
}

// INTEG-024: SHARD-023 — raising a live member's votes waits for SECONDARY
// and goes out in its own reconfig, leaving the member's other fields alone.
func TestIntegration_ShardConfigUpdate_PromotesLiveMember(t *testing.T) {
	ensureMemberAddCluster(t)
	client := newMemberAddSeedClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	before, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig before: %v", err)
	}
	if len(before.Members) != 3 || derefInt(before.Members[2].Votes) != 0 {
		t.Skipf("precondition: runs after the set has grown to 3 members with a non-voting third, got %v", before.Members)
	}
	if err := waitForAllMembersHealthy(ctx, client, 3, 3*time.Minute); err != nil {
		t.Fatal(err)
	}

	data := memberAddResourceData(t, 1)
	if diags := RShardConfig.updateWithClient(ctx, data, client, memberAddProviderConf(), true); diags.HasError() {
		t.Fatalf("updateWithClient: %v", diags)
	}

	after, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after: %v", err)
	}
	if len(after.Members) != 3 {
		t.Fatalf("want 3 members, got %d", len(after.Members))
	}
	third := after.Members[2]
	if derefInt(third.Votes) != 1 || third.Priority != 0 || !derefBool(third.Hidden) || third.ID != before.Members[2].ID {
		t.Errorf("third member: want votes 1 with priority 0, hidden and the same _id, got %+v", third)
	}
	// Settings reconfig plus the promotion.
	if after.Version < before.Version+2 {
		t.Errorf("version: want at least %d, got %d", before.Version+2, after.Version)
	}
	if stateMembers := data.Get("member").([]interface{}); len(stateMembers) != 3 ||
		stateMembers[2].(map[string]interface{})["votes"] != 1 {
		t.Errorf("state should show the promoted votes, got %v", data.Get("member"))
	}
}
