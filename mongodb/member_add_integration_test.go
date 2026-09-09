//go:build integration

package mongodb

import (
	"context"
	"fmt"
	"os"
	"strings"
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

// memberAddMappedHost is the address at which the test process reaches node
// i: the same mongod the set knows as memberAddHost(i), under another name.
// The network aliases resolve only inside the test network, so this is the
// only address the pre-flight probe (SHARD-027) can connect to.
func memberAddMappedHost(ctx context.Context, i int) (string, error) {
	c := memberAddCluster.nodes[i]
	host, err := c.Host(ctx)
	if err != nil {
		return "", err
	}
	port, err := c.MappedPort(ctx, nat.Port(memberAddPort+"/tcp"))
	if err != nil {
		return "", err
	}
	return host + ":" + port.Port(), nil
}

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
//
// The member hosts are container aliases that only resolve inside the test
// network, so the test process cannot reach them. host_override says so
// (DISC-008): without it the readiness wait of SHARD-029 would wait for the
// aliases until the deadline. With it, the pre-flight probe (SHARD-027) is
// skipped as well, so the adds exercise the uninspectable pass-through path.
// INTEG-026 and INTEG-028 drop host_override to run the wait and the probe
// against a host the test process can reach and one it cannot.
func memberAddResourceData(t *testing.T, thirdVotes int) *schema.ResourceData {
	return memberAddResourceDataWith(t, 120, thirdVotes)
}

func memberAddResourceDataWith(t *testing.T, timeoutSecs, thirdVotes int, extra ...map[string]interface{}) *schema.ResourceData {
	return schema.TestResourceDataRaw(t, resourceShardConfig().Schema, memberAddRaw(timeoutSecs, thirdVotes, extra...))
}

// memberAddRaw is the raw configuration behind memberAddResourceDataWith, for
// tests that adjust it before building the ResourceData.
func memberAddRaw(timeoutSecs, thirdVotes int, extra ...map[string]interface{}) map[string]interface{} {
	members := []interface{}{
		map[string]interface{}{"host": memberAddHost(0), "priority": 2.0, "votes": 1},
		map[string]interface{}{"host": memberAddHost(1), "priority": 1.0, "votes": 1,
			"tags": map[string]interface{}{"role": "online"}},
		map[string]interface{}{"host": memberAddHost(2), "priority": 0.0, "votes": thirdVotes, "hidden": true},
	}
	for _, m := range extra {
		members = append(members, m)
	}
	return map[string]interface{}{
		"shard_name":        memberAddRSName,
		"init_timeout_secs": timeoutSecs,
		"host_override":     memberAddCluster.seedHost + ":" + memberAddCluster.seedPort,
		"member":            members,
	}
}

// ensureMemberAddClusterGrown brings the set to the three-member shape
// INTEG-022 produces, so the later tests do not depend on it having run.
func ensureMemberAddClusterGrown(ctx context.Context, t *testing.T, client *mongo.Client) {
	t.Helper()
	cfg, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig: %v", err)
	}
	if len(cfg.Members) == 3 {
		return
	}
	if diags := RShardConfig.updateWithClient(ctx, memberAddResourceData(t, 0), client, memberAddProviderConf(), true); diags.HasError() {
		t.Fatalf("growing the set to three members: %v", diags)
	}
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

	// SHARD-027: before it joins, the second node answers isMaster as a mongod
	// with --replSet and no configuration, which the pre-flight reads as
	// addable. The probe reaches it through its mapped port; the add below
	// names the network alias, which the probe cannot resolve.
	freshAddr, err := memberAddMappedHost(ctx, 1)
	if err != nil {
		t.Fatalf("mapped host of node 1: %v", err)
	}
	resp, probeErr := memberProbeFor(memberAddProviderConf())(ctx, freshAddr)
	if verdict, reason := CheckAddTarget(memberAddRSName, resp, probeErr); verdict != addTargetOK {
		t.Fatalf("fresh node %s should read as addable, got verdict %d: %s", freshAddr, verdict, reason)
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

	ensureMemberAddClusterGrown(ctx, t, client)
	before, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig before: %v", err)
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

// INTEG-025: SHARD-017, SHARD-021 — a member the primary never reaches is
// removed again, the apply fails, and the member state written on the way out
// lists the live members rather than the planned blocks.
func TestIntegration_ShardConfigUpdate_UnreachableMemberRolledBack(t *testing.T) {
	ensureMemberAddCluster(t)
	client := newMemberAddSeedClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ensureMemberAddClusterGrown(ctx, t, client)
	if err := waitForAllMembersHealthy(ctx, client, 3, 3*time.Minute); err != nil {
		t.Fatal(err)
	}
	before, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig before: %v", err)
	}

	const bogus = "rsgrow-nope:27017"
	data := memberAddResourceDataWith(t, 15, 0, map[string]interface{}{"host": bogus, "priority": 1.0, "votes": 1})
	diags := RShardConfig.updateWithClient(ctx, data, client, memberAddProviderConf(), true)
	if !diags.HasError() {
		t.Fatal("want an error for the member that never became reachable")
	}
	msg := fmt.Sprint(diags)
	for _, want := range []string{bogus, "removed again"} {
		if !strings.Contains(msg, want) {
			t.Errorf("diagnostics should contain %q, got: %s", want, msg)
		}
	}

	after, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after: %v", err)
	}
	if len(after.Members) != 3 {
		t.Fatalf("want the three live members after the rollback, got %v", memberHosts(after.Members))
	}
	for i := range after.Members {
		if after.Members[i].Host != before.Members[i].Host || after.Members[i].ID != before.Members[i].ID {
			t.Errorf("member %d changed: before %+v after %+v", i, before.Members[i], after.Members[i])
		}
	}

	stateMembers, ok := data.Get("member").([]interface{})
	if !ok || len(stateMembers) != 3 {
		t.Fatalf("state must list only the live members after a failed add, got %v", data.Get("member"))
	}
	for i, raw := range stateMembers {
		if host := raw.(map[string]interface{})["host"]; host != memberAddHost(i) {
			t.Errorf("state member %d: want %s, got %v", i, memberAddHost(i), host)
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

	ensureMemberAddClusterGrown(ctx, t, client)
	if err := waitForAllMembersHealthy(ctx, client, 3, 3*time.Minute); err != nil {
		t.Fatal(err)
	}
	// An earlier run may have left the third member voting; the votes-0
	// configuration demotes it through the settings reconfig so the
	// promotion below has something to do.
	if diags := RShardConfig.updateWithClient(ctx, memberAddResourceData(t, 0), client, memberAddProviderConf(), true); diags.HasError() {
		t.Fatalf("resetting the third member to a non-voter: %v", diags)
	}
	before, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig before: %v", err)
	}
	if derefInt(before.Members[2].Votes) != 0 {
		t.Fatalf("precondition: third member should be a non-voter, got %+v", before.Members[2])
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

// INTEG-026: SHARD-027 — a block naming a live member under another name is
// refused by the pre-flight before any reconfig is sent. The test process
// reaches each node through its mapped port, so that address is an alias of
// the network name the set knows the node by: exactly the FQDN-for-short-name
// case the pre-flight exists for.
func TestIntegration_ShardConfigUpdate_RefusesAliasOfLiveMember(t *testing.T) {
	ensureMemberAddCluster(t)
	client := newMemberAddSeedClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ensureMemberAddClusterGrown(ctx, t, client)
	if err := waitForAllMembersHealthy(ctx, client, 3, 3*time.Minute); err != nil {
		t.Fatal(err)
	}
	before, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig before: %v", err)
	}

	probe := memberProbeFor(memberAddProviderConf())
	for i := range memberAddAliases {
		addr, err := memberAddMappedHost(ctx, i)
		if err != nil {
			t.Fatalf("mapped host of node %d: %v", i, err)
		}
		resp, probeErr := probe(ctx, addr)
		verdict, reason := CheckAddTarget(memberAddRSName, resp, probeErr)
		if verdict != addTargetRefused || !strings.Contains(reason, "as "+memberAddHost(i)) {
			t.Errorf("probe of %s: want a refusal naming %s, got verdict %d: %s", addr, memberAddHost(i), verdict, reason)
		}
	}

	alias, err := memberAddMappedHost(ctx, 0)
	if err != nil {
		t.Fatalf("mapped host of node 0: %v", err)
	}
	// The three live blocks are matched, so the alias is the only host to
	// wait for and inspect, and the test process can reach it: without
	// host_override the readiness wait (SHARD-029) and the probe run.
	raw := memberAddRaw(120, 0, map[string]interface{}{"host": alias, "priority": 1.0, "votes": 1})
	delete(raw, "host_override")
	data := schema.TestResourceDataRaw(t, resourceShardConfig().Schema, raw)
	diags := RShardConfig.updateWithClient(ctx, data, client, memberAddProviderConf(), true)
	if !diags.HasError() {
		t.Fatal("want the aliased member to be refused")
	}
	msg := fmt.Sprint(diags)
	for _, want := range []string{alias, "already a member", "as " + memberAddHost(0), "use that host"} {
		if !strings.Contains(msg, want) {
			t.Errorf("diagnostics should contain %q, got: %s", want, msg)
		}
	}

	after, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after: %v", err)
	}
	if after.Version != before.Version || len(after.Members) != len(before.Members) {
		t.Errorf("a refused block must not send any reconfig: version %d -> %d, members %v -> %v",
			before.Version, after.Version, memberHosts(before.Members), memberHosts(after.Members))
	}
	if data.Id() != "" {
		t.Errorf("nothing may be recorded in state when the pre-flight refuses, got id %q", data.Id())
	}
}

// INTEG-027: SHARD-028 — an arbiter whose host the pre-flight cannot inspect,
// here because host_override says the member hosts are unreachable from the
// runner, is refused before any reconfig is sent: it would be added with its
// vote, and if the host were down the resource could not undo the add.
func TestIntegration_ShardConfigUpdate_RefusesUninspectableArbiter(t *testing.T) {
	ensureMemberAddCluster(t)
	client := newMemberAddSeedClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	ensureMemberAddClusterGrown(ctx, t, client)
	before, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig before: %v", err)
	}

	const bogus = "rsgrow-nope:27017"
	data := memberAddResourceDataWith(t, 15, 0, map[string]interface{}{"host": bogus, "arbiter_only": true})
	diags := RShardConfig.updateWithClient(ctx, data, client, memberAddProviderConf(), true)
	if !diags.HasError() {
		t.Fatal("want the uninspectable arbiter to be refused")
	}
	msg := fmt.Sprint(diags)
	for _, want := range []string{"arbiter " + bogus, "could not be inspected", "add the arbiter by hand"} {
		if !strings.Contains(msg, want) {
			t.Errorf("diagnostics should contain %q, got: %s", want, msg)
		}
	}

	after, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after: %v", err)
	}
	if after.Version != before.Version || len(after.Members) != len(before.Members) {
		t.Errorf("a refused arbiter must not send any reconfig: version %d -> %d, members %v -> %v",
			before.Version, after.Version, memberHosts(before.Members), memberHosts(after.Members))
	}
}

// INTEG-028: SHARD-029 — without host_override, a new member host that does
// not answer is waited for until the operation's deadline, and the apply then
// fails naming the host before any reconfig is sent.
func TestIntegration_ShardConfigUpdate_WaitsForUnreachableMemberUntilDeadline(t *testing.T) {
	ensureMemberAddCluster(t)
	client := newMemberAddSeedClient(t)
	setupCtx, cancelSetup := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelSetup()

	ensureMemberAddClusterGrown(setupCtx, t, client)
	before, err := GetReplSetConfig(setupCtx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig before: %v", err)
	}

	const bogus = "rsgrow-nope:27017"
	raw := memberAddRaw(15, 0, map[string]interface{}{"host": bogus, "priority": 1.0, "votes": 1})
	delete(raw, "host_override")
	data := schema.TestResourceDataRaw(t, resourceShardConfig().Schema, raw)

	// The deadline stands in for the create or update timeout the SDK puts on
	// the context.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	start := time.Now()
	diags := RShardConfig.updateWithClient(ctx, data, client, memberAddProviderConf(), true)
	if !diags.HasError() {
		t.Fatal("want an error once the deadline passes with the host still unreachable")
	}
	if elapsed := time.Since(start); elapsed < 10*time.Second {
		t.Errorf("the wait should last until the deadline, returned after %s", elapsed)
	}
	msg := fmt.Sprint(diags)
	for _, want := range []string{"replica set member " + bogus, "did not accept connections", "operation timeout"} {
		if !strings.Contains(msg, want) {
			t.Errorf("diagnostics should contain %q, got: %s", want, msg)
		}
	}

	after, err := GetReplSetConfig(setupCtx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after: %v", err)
	}
	if after.Version != before.Version || len(after.Members) != len(before.Members) {
		t.Errorf("a host still unreachable at the deadline must not send any reconfig: version %d -> %d, members %v -> %v",
			before.Version, after.Version, memberHosts(before.Members), memberHosts(after.Members))
	}
	if data.Id() != "" {
		t.Errorf("nothing may be recorded in state when the wait fails, got id %q", data.Id())
	}
}
