//go:build integration

package mongodb

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// shardedCluster holds the shared sharded cluster state for integration tests.
// It is lazily initialized via sync.Once on first use.
var shardedCluster struct {
	once sync.Once
	err  error

	network     *testcontainers.DockerNetwork
	configsvr   *testcontainers.DockerContainer
	shard01     *testcontainers.DockerContainer
	shard02     *testcontainers.DockerContainer
	mongos      *testcontainers.DockerContainer
	mongosHost  string
	mongosPort  string
	shard01Host string
	shard01Port string
	shard02Host string
	shard02Port string
}

const (
	shardedAdminUser     = "admin"
	shardedAdminPassword = "testpassword"
	shardedAdminDB       = "admin"
)

// setupShardedCluster orchestrates the full sharded cluster startup.
func setupShardedCluster() error {
	ctx := context.Background()

	image := testMongoImage
	if env := os.Getenv("MONGO_TEST_IMAGE"); env != "" {
		image = env
	}

	// 1. Create Docker network
	nw, err := network.New(ctx)
	if err != nil {
		return fmt.Errorf("create network: %w", err)
	}
	shardedCluster.network = nw

	// 2. Start configsvr, shard01, shard02 in parallel
	type containerResult struct {
		name      string
		container *testcontainers.DockerContainer
		err       error
	}
	results := make(chan containerResult, 3)

	startContainer := func(name string, alias string, port string, cmd []string) {
		natPort := nat.Port(port + "/tcp")
		c, err := testcontainers.Run(ctx, image,
			network.WithNetwork([]string{alias}, nw),
			testcontainers.WithCmd(cmd...),
			testcontainers.WithExposedPorts(string(natPort)),
			testcontainers.WithWaitStrategy(wait.ForListeningPort(natPort).WithStartupTimeout(120*time.Second)),
		)
		results <- containerResult{name: name, container: c, err: err}
	}

	go startContainer("configsvr", "configsvr0", "27019", []string{
		"mongod", "--configsvr", "--replSet", "configRS", "--port", "27019", "--bind_ip_all",
	})
	go startContainer("shard01", "shard01svr0", "27018", []string{
		"mongod", "--shardsvr", "--replSet", "shard01", "--port", "27018", "--bind_ip_all",
	})
	go startContainer("shard02", "shard02svr0", "27018", []string{
		"mongod", "--shardsvr", "--replSet", "shard02", "--port", "27018", "--bind_ip_all",
	})

	for i := 0; i < 3; i++ {
		r := <-results
		if r.err != nil {
			return fmt.Errorf("start %s: %w", r.name, r.err)
		}
		switch r.name {
		case "configsvr":
			shardedCluster.configsvr = r.container
		case "shard01":
			shardedCluster.shard01 = r.container
		case "shard02":
			shardedCluster.shard02 = r.container
		}
	}

	// 3. Init each replica set
	if err := initRS(ctx, shardedCluster.configsvr, "configRS", "configsvr0", "27019"); err != nil {
		return fmt.Errorf("init configRS: %w", err)
	}
	if err := initRS(ctx, shardedCluster.shard01, "shard01", "shard01svr0", "27018"); err != nil {
		return fmt.Errorf("init shard01: %w", err)
	}
	if err := initRS(ctx, shardedCluster.shard02, "shard02", "shard02svr0", "27018"); err != nil {
		return fmt.Errorf("init shard02: %w", err)
	}

	// 4. Wait for each RS to elect a primary
	if err := waitForPrimaryExec(ctx, shardedCluster.configsvr, "27019"); err != nil {
		return fmt.Errorf("wait configRS primary: %w", err)
	}
	if err := waitForPrimaryExec(ctx, shardedCluster.shard01, "27018"); err != nil {
		return fmt.Errorf("wait shard01 primary: %w", err)
	}
	if err := waitForPrimaryExec(ctx, shardedCluster.shard02, "27018"); err != nil {
		return fmt.Errorf("wait shard02 primary: %w", err)
	}

	// 5. Start mongos
	mongosPort := nat.Port("27017/tcp")
	mongosContainer, err := testcontainers.Run(ctx, image,
		network.WithNetwork([]string{"mongos0"}, nw),
		testcontainers.WithEntrypoint("mongos"),
		testcontainers.WithCmd(
			"--configdb", "configRS/configsvr0:27019",
			"--bind_ip_all",
			"--port", "27017",
		),
		testcontainers.WithExposedPorts(string(mongosPort)),
		testcontainers.WithWaitStrategy(wait.ForListeningPort(mongosPort).WithStartupTimeout(120*time.Second)),
	)
	if err != nil {
		return fmt.Errorf("start mongos: %w", err)
	}
	shardedCluster.mongos = mongosContainer

	// 6. Register shards via mongos
	if err := mongosExec(ctx, mongosContainer, `sh.addShard("shard01/shard01svr0:27018")`); err != nil {
		return fmt.Errorf("addShard shard01: %w", err)
	}
	if err := mongosExec(ctx, mongosContainer, `sh.addShard("shard02/shard02svr0:27018")`); err != nil {
		return fmt.Errorf("addShard shard02: %w", err)
	}

	// 7. Create admin user on mongos
	createAdminJS := fmt.Sprintf(
		`db.getSiblingDB("admin").createUser({user:"%s",pwd:"%s",roles:["root"]})`,
		shardedAdminUser, shardedAdminPassword,
	)
	if err := mongosExec(ctx, mongosContainer, createAdminJS); err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	// 8. Extract mapped ports
	mongosHost, err := mongosContainer.Host(ctx)
	if err != nil {
		return fmt.Errorf("mongos host: %w", err)
	}
	mongosMapped, err := mongosContainer.MappedPort(ctx, "27017")
	if err != nil {
		return fmt.Errorf("mongos port: %w", err)
	}
	shardedCluster.mongosHost = mongosHost
	shardedCluster.mongosPort = mongosMapped.Port()

	shard01Host, err := shardedCluster.shard01.Host(ctx)
	if err != nil {
		return fmt.Errorf("shard01 host: %w", err)
	}
	shard01Mapped, err := shardedCluster.shard01.MappedPort(ctx, "27018")
	if err != nil {
		return fmt.Errorf("shard01 port: %w", err)
	}
	shardedCluster.shard01Host = shard01Host
	shardedCluster.shard01Port = shard01Mapped.Port()

	shard02Host, err := shardedCluster.shard02.Host(ctx)
	if err != nil {
		return fmt.Errorf("shard02 host: %w", err)
	}
	shard02Mapped, err := shardedCluster.shard02.MappedPort(ctx, "27018")
	if err != nil {
		return fmt.Errorf("shard02 port: %w", err)
	}
	shardedCluster.shard02Host = shard02Host
	shardedCluster.shard02Port = shard02Mapped.Port()

	// Create admin user on each shard directly (for direct connections)
	for _, sc := range []struct {
		name string
		c    *testcontainers.DockerContainer
		port string
	}{
		{"shard01", shardedCluster.shard01, "27018"},
		{"shard02", shardedCluster.shard02, "27018"},
	} {
		js := fmt.Sprintf(
			`db.getSiblingDB("admin").createUser({user:"%s",pwd:"%s",roles:["root"]})`,
			shardedAdminUser, shardedAdminPassword,
		)
		if err := containerExec(ctx, sc.c, sc.port, js); err != nil {
			return fmt.Errorf("create admin on %s: %w", sc.name, err)
		}
	}

	return nil
}

// initRS initiates a replica set on the given container.
func initRS(ctx context.Context, c *testcontainers.DockerContainer, rsName, hostname, port string) error {
	js := fmt.Sprintf(
		`rs.initiate({_id:"%s",members:[{_id:0,host:"%s:%s"}]})`,
		rsName, hostname, port,
	)
	return containerExec(ctx, c, port, js)
}

// waitForPrimaryExec polls rs.status() via container exec until myState == 1 (PRIMARY).
func waitForPrimaryExec(ctx context.Context, c *testcontainers.DockerContainer, port string) error {
	deadline := time.Now().Add(120 * time.Second)
	js := `JSON.stringify({state: rs.status().myState})`
	for time.Now().Before(deadline) {
		code, reader, err := c.Exec(ctx, []string{
			"mongosh", "--port", port, "--quiet", "--eval", js,
		}, exec.Multiplexed())
		if err == nil && code == 0 {
			out, _ := io.ReadAll(reader)
			if strings.Contains(string(out), `"state":1`) {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for PRIMARY on port %s", port)
}

// mongosExec runs a JS expression against the mongos container.
func mongosExec(ctx context.Context, c *testcontainers.DockerContainer, js string) error {
	return containerExec(ctx, c, "27017", js)
}

// containerExec runs a JS expression via mongosh on a container.
func containerExec(ctx context.Context, c *testcontainers.DockerContainer, port, js string) error {
	code, reader, err := c.Exec(ctx, []string{
		"mongosh", "--port", port, "--quiet", "--eval", js,
	}, exec.Multiplexed())
	if err != nil {
		return fmt.Errorf("exec failed: %w", err)
	}
	out, _ := io.ReadAll(reader)
	if code != 0 {
		return fmt.Errorf("exec exited %d: %s", code, string(out))
	}
	return nil
}

// ensureShardedCluster lazily starts the sharded cluster. If setup fails, it
// calls t.Skipf so the test is skipped rather than failed hard.
func ensureShardedCluster(t *testing.T) {
	t.Helper()
	shardedCluster.once.Do(func() {
		shardedCluster.err = setupShardedCluster()
	})
	if shardedCluster.err != nil {
		t.Skipf("sharded cluster unavailable: %v", shardedCluster.err)
	}
}

// teardownShardedCluster terminates all sharded cluster containers and removes
// the network. Called from TestMain.
func teardownShardedCluster() {
	ctx := context.Background()
	for _, c := range []*testcontainers.DockerContainer{
		shardedCluster.mongos,
		shardedCluster.shard01,
		shardedCluster.shard02,
		shardedCluster.configsvr,
	} {
		if c != nil {
			_ = c.Terminate(ctx)
		}
	}
	if shardedCluster.network != nil {
		_ = shardedCluster.network.Remove(ctx)
	}
}

// newMongosConfig creates a ClientConfig pointing at the mongos router.
func newMongosConfig() *ClientConfig {
	return &ClientConfig{
		Host:        shardedCluster.mongosHost,
		Port:        shardedCluster.mongosPort,
		Username:    shardedAdminUser,
		Password:    shardedAdminPassword,
		DB:          shardedAdminDB,
		RetryWrites: false,
	}
}

// newMongosClient creates a connected mongo.Client for the mongos router.
func newMongosClient(t *testing.T) *mongo.Client {
	t.Helper()
	uri := fmt.Sprintf("mongodb://%s:%s@%s:%s/",
		shardedAdminUser, shardedAdminPassword,
		shardedCluster.mongosHost, shardedCluster.mongosPort,
	)
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect to mongos: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("ping mongos: %v", err)
	}
	return client
}

// newDirectShardClient creates a client connected directly to a shard member.
func newDirectShardClient(t *testing.T, host, port, rsName string) *mongo.Client {
	t.Helper()
	uri := fmt.Sprintf("mongodb://%s:%s@%s:%s/?replicaSet=%s&directConnection=true",
		shardedAdminUser, shardedAdminPassword, host, port, rsName,
	)
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect to shard: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("ping shard: %v", err)
	}
	return client
}

// shardHostOverride returns "host:mappedPort" for use as host_override in tests.
func shardHostOverride(shardName string) string {
	switch shardName {
	case "shard01":
		return fmt.Sprintf("%s:%s", shardedCluster.shard01Host, shardedCluster.shard01Port)
	case "shard02":
		return fmt.Sprintf("%s:%s", shardedCluster.shard02Host, shardedCluster.shard02Port)
	default:
		return ""
	}
}

// SINTEG-005: DetectConnectionType returns ConnTypeMongos against mongos
func TestShardedIntegration_DetectConnectionType_Mongos(t *testing.T) {
	ensureShardedCluster(t)
	client := newMongosClient(t)

	connType, err := DetectConnectionType(context.Background(), client)
	if err != nil {
		t.Fatalf("DetectConnectionType failed: %v", err)
	}
	if connType != ConnTypeMongos {
		t.Errorf("expected ConnTypeMongos (%d), got %s (%d)", ConnTypeMongos, connType, connType)
	}
}

// SINTEG-013: DetectConnectionType returns ConnTypeReplicaSet against shard direct
func TestShardedIntegration_DetectConnectionType_ShardRS(t *testing.T) {
	ensureShardedCluster(t)
	client := newDirectShardClient(t, shardedCluster.shard01Host, shardedCluster.shard01Port, "shard01")

	connType, err := DetectConnectionType(context.Background(), client)
	if err != nil {
		t.Fatalf("DetectConnectionType failed: %v", err)
	}
	if connType != ConnTypeReplicaSet {
		t.Errorf("expected ConnTypeReplicaSet (%d), got %s (%d)", ConnTypeReplicaSet, connType, connType)
	}
}

// SINTEG-006: ListShards returns 2 shards with correct IDs
func TestShardedIntegration_ListShards_ReturnsBothShards(t *testing.T) {
	ensureShardedCluster(t)
	client := newMongosClient(t)

	shards, err := ListShards(context.Background(), client)
	if err != nil {
		t.Fatalf("ListShards failed: %v", err)
	}
	if len(shards.Shards) != 2 {
		t.Fatalf("expected 2 shards, got %d", len(shards.Shards))
	}

	ids := map[string]bool{}
	for _, s := range shards.Shards {
		ids[s.ID] = true
	}
	if !ids["shard01"] {
		t.Error("shard01 not found in shard list")
	}
	if !ids["shard02"] {
		t.Error("shard02 not found in shard list")
	}
}

// SINTEG-007: FindShardByName matches real shard
func TestShardedIntegration_FindShardByName_Found(t *testing.T) {
	ensureShardedCluster(t)
	client := newMongosClient(t)

	shards, err := ListShards(context.Background(), client)
	if err != nil {
		t.Fatalf("ListShards failed: %v", err)
	}

	host, err := FindShardByName(shards, "shard01")
	if err != nil {
		t.Fatalf("FindShardByName failed: %v", err)
	}
	if !strings.Contains(host, "shard01") {
		t.Errorf("expected host to contain 'shard01', got %q", host)
	}
}

// SINTEG-008: FindShardByName errors for bogus name, lists available
func TestShardedIntegration_FindShardByName_NotFound(t *testing.T) {
	ensureShardedCluster(t)
	client := newMongosClient(t)

	shards, err := ListShards(context.Background(), client)
	if err != nil {
		t.Fatalf("ListShards failed: %v", err)
	}

	_, err = FindShardByName(shards, "nonexistent_shard")
	if err == nil {
		t.Fatal("expected error for nonexistent shard, got nil")
	}
	if !strings.Contains(err.Error(), "shard01") || !strings.Contains(err.Error(), "shard02") {
		t.Errorf("expected error to list available shards, got: %v", err)
	}
}

// SINTEG-009: ResolveShardClient with host_override returns working client
func TestShardedIntegration_ResolveShardClient_WithHostOverride(t *testing.T) {
	ensureShardedCluster(t)
	mongosClient := newMongosClient(t)
	cfg := newMongosConfig()

	override := shardHostOverride("shard01")
	shardClient, cleanup, err := ResolveShardClient(
		context.Background(), mongosClient, cfg, "shard01", override, 30,
	)
	if err != nil {
		t.Fatalf("ResolveShardClient failed: %v", err)
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := shardClient.Ping(ctx, nil); err != nil {
		t.Fatalf("shard client ping failed: %v", err)
	}
}

// SINTEG-010: GetReplSetConfig via resolved shard client returns correct RS name
func TestShardedIntegration_ResolveShardClient_GetReplSetConfig(t *testing.T) {
	ensureShardedCluster(t)
	mongosClient := newMongosClient(t)
	cfg := newMongosConfig()

	override := shardHostOverride("shard01")
	shardClient, cleanup, err := ResolveShardClient(
		context.Background(), mongosClient, cfg, "shard01", override, 30,
	)
	if err != nil {
		t.Fatalf("ResolveShardClient failed: %v", err)
	}
	defer cleanup()

	rsConfig, err := GetReplSetConfig(context.Background(), shardClient)
	if err != nil {
		t.Fatalf("GetReplSetConfig failed: %v", err)
	}
	if rsConfig.ID != "shard01" {
		t.Errorf("expected RS name 'shard01', got %q", rsConfig.ID)
	}
}

// SINTEG-011: SetReplSetConfig on shard via discovery persists and round-trips
func TestShardedIntegration_ResolveShardClient_SetReplSetConfig_RoundTrip(t *testing.T) {
	ensureShardedCluster(t)
	mongosClient := newMongosClient(t)
	cfg := newMongosConfig()

	override := shardHostOverride("shard02")
	shardClient, cleanup, err := ResolveShardClient(
		context.Background(), mongosClient, cfg, "shard02", override, 30,
	)
	if err != nil {
		t.Fatalf("ResolveShardClient failed: %v", err)
	}
	defer cleanup()

	ctx := context.Background()
	rsConfig, err := GetReplSetConfig(ctx, shardClient)
	if err != nil {
		t.Fatalf("GetReplSetConfig failed: %v", err)
	}

	originalHBI := rsConfig.Settings.HeartbeatIntervalMillis
	newHBI := int64(3000)
	if originalHBI == newHBI {
		newHBI = 4000
	}

	rsConfig.Version++
	rsConfig.Settings.HeartbeatIntervalMillis = newHBI

	if err := SetReplSetConfig(ctx, shardClient, rsConfig); err != nil {
		t.Fatalf("SetReplSetConfig failed: %v", err)
	}

	updated, err := GetReplSetConfig(ctx, shardClient)
	if err != nil {
		t.Fatalf("GetReplSetConfig after update failed: %v", err)
	}
	if updated.Settings.HeartbeatIntervalMillis != newHBI {
		t.Errorf("HeartbeatIntervalMillis: expected %d, got %d", newHBI, updated.Settings.HeartbeatIntervalMillis)
	}
}

// SINTEG-012: Both shards discoverable, have different RS names
func TestShardedIntegration_MultiShard_IndependentClients(t *testing.T) {
	ensureShardedCluster(t)
	mongosClient := newMongosClient(t)
	cfg := newMongosConfig()

	ctx := context.Background()

	shard01Client, cleanup1, err := ResolveShardClient(
		ctx, mongosClient, cfg, "shard01", shardHostOverride("shard01"), 30,
	)
	if err != nil {
		t.Fatalf("ResolveShardClient shard01 failed: %v", err)
	}
	defer cleanup1()

	shard02Client, cleanup2, err := ResolveShardClient(
		ctx, mongosClient, cfg, "shard02", shardHostOverride("shard02"), 30,
	)
	if err != nil {
		t.Fatalf("ResolveShardClient shard02 failed: %v", err)
	}
	defer cleanup2()

	rs1, err := GetReplSetConfig(ctx, shard01Client)
	if err != nil {
		t.Fatalf("GetReplSetConfig shard01 failed: %v", err)
	}
	rs2, err := GetReplSetConfig(ctx, shard02Client)
	if err != nil {
		t.Fatalf("GetReplSetConfig shard02 failed: %v", err)
	}

	if rs1.ID == rs2.ID {
		t.Errorf("expected different RS names, both got %q", rs1.ID)
	}
	if rs1.ID != "shard01" {
		t.Errorf("expected shard01 RS name 'shard01', got %q", rs1.ID)
	}
	if rs2.ID != "shard02" {
		t.Errorf("expected shard02 RS name 'shard02', got %q", rs2.ID)
	}
}

// SINTEG-014: ResolveShardClient on direct RS returns same client (no mongos discovery)
func TestShardedIntegration_ResolveShardClient_DirectRS_Passthrough(t *testing.T) {
	ensureShardedCluster(t)

	directClient := newDirectShardClient(t, shardedCluster.shard01Host, shardedCluster.shard01Port, "shard01")
	directCfg := &ClientConfig{
		Host:        shardedCluster.shard01Host,
		Port:        shardedCluster.shard01Port,
		Username:    shardedAdminUser,
		Password:    shardedAdminPassword,
		DB:          shardedAdminDB,
		ReplicaSet:  "shard01",
		Direct:      true,
		RetryWrites: false,
	}

	returned, cleanup, err := ResolveShardClient(
		context.Background(), directClient, directCfg, "shard01", "", 30,
	)
	if err != nil {
		t.Fatalf("ResolveShardClient failed: %v", err)
	}
	defer cleanup()

	// On direct RS, ResolveShardClient should return the same client
	if returned != directClient {
		t.Error("expected ResolveShardClient to return the same client on direct RS connection")
	}
}
