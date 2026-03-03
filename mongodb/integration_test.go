//go:build integration

package mongodb

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcmongodb "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	testMongoImage    = "mongo:7"
	testAdminUser     = "admin"
	testAdminPassword = "testpassword"
	testAdminDB       = "admin"
	testReplSetName   = "rs0"
)

// testMongoContainer holds the shared container state for integration tests.
var testMongoContainer struct {
	container *tcmongodb.MongoDBContainer
	host      string
	port      string
	client    *mongo.Client
}

// isPodman detects if the container runtime is Podman by checking if `podman`
// is on PATH and Docker is absent or is a Podman alias.
func isPodman() bool {
	if _, err := exec.LookPath("podman"); err != nil {
		return false
	}
	out, err := exec.Command("docker", "--version").Output()
	if err != nil {
		return true
	}
	return strings.Contains(strings.ToLower(string(out)), "podman")
}

func TestMain(m *testing.M) {
	ctx := context.Background()

	// Podman compatibility: disable Ryuk reaper which cannot mount the socket.
	if isPodman() {
		os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}

	image := testMongoImage
	if env := os.Getenv("MONGO_TEST_IMAGE"); env != "" {
		image = env
	}

	container, err := tcmongodb.Run(ctx, image,
		tcmongodb.WithUsername(testAdminUser),
		tcmongodb.WithPassword(testAdminPassword),
		tcmongodb.WithReplicaSet(testReplSetName),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start MongoDB container: %v\n", err)
		os.Exit(1)
	}

	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get container host: %v\n", err)
		os.Exit(1)
	}
	mappedPort, err := container.MappedPort(ctx, "27017")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get container port: %v\n", err)
		os.Exit(1)
	}

	testMongoContainer.container = container
	testMongoContainer.host = host
	testMongoContainer.port = mappedPort.Port()

	// Create an admin client for test setup using the mongo driver directly
	uri := fmt.Sprintf("mongodb://%s:%s@%s:%s/?replicaSet=%s&directConnection=true",
		testAdminUser, testAdminPassword, host, mappedPort.Port(), testReplSetName)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect admin client: %v\n", err)
		os.Exit(1)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		fmt.Fprintf(os.Stderr, "failed to ping MongoDB: %v\n", err)
		os.Exit(1)
	}

	testMongoContainer.client = client

	code := m.Run()

	_ = client.Disconnect(ctx)
	_ = container.Terminate(ctx)
	os.Exit(code)
}

// newTestConfig creates a MongoDatabaseConfiguration pointing at the test container.
func newTestConfig() *MongoDatabaseConfiguration {
	return &MongoDatabaseConfiguration{
		Config: &ClientConfig{
			Host:        testMongoContainer.host,
			Port:        testMongoContainer.port,
			Username:    testAdminUser,
			Password:    testAdminPassword,
			DB:          testAdminDB,
			ReplicaSet:  testReplSetName,
			Direct:      true,
			RetryWrites: false,
		},
		MaxConnLifetime: 10,
	}
}

// newTestClient creates a connected mongo.Client using the provider's MongoClientInit.
func newTestClient(t *testing.T) *mongo.Client {
	t.Helper()
	conf := newTestConfig()
	client, err := MongoClientInit(context.Background(), conf)
	if err != nil {
		t.Fatalf("MongoClientInit failed: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Disconnect(context.Background())
	})
	return client
}

// INTEG-001: MongoClientInit connects to live MongoDB replica set
func TestIntegration_MongoClientInit_Success(t *testing.T) {
	conf := newTestConfig()
	client, err := MongoClientInit(context.Background(), conf)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

// INTEG-002: MongoClientInit with invalid credentials returns error
func TestIntegration_MongoClientInit_BadCredentials(t *testing.T) {
	conf := &MongoDatabaseConfiguration{
		Config: &ClientConfig{
			Host:        testMongoContainer.host,
			Port:        testMongoContainer.port,
			Username:    "wronguser",
			Password:    "wrongpassword",
			DB:          testAdminDB,
			Direct:      true,
			RetryWrites: false,
		},
		MaxConnLifetime: 5,
	}
	_, err := MongoClientInit(context.Background(), conf)
	if err == nil {
		t.Fatal("expected error for bad credentials, got nil")
	}
}

// INTEG-003: createUser then getUser returns matching user
func TestIntegration_CreateUser_GetUser(t *testing.T) {
	client := newTestClient(t)

	user := DbUser{Name: "integuser1", Password: "pass123"}
	roles := []Role{{Role: "readWrite", Db: "testdb"}}

	err := createUser(client, user, roles, testAdminDB)
	if err != nil {
		t.Fatalf("createUser failed: %v", err)
	}

	result, err := getUser(client, "integuser1", testAdminDB)
	if err != nil {
		t.Fatalf("getUser failed: %v", err)
	}
	if len(result.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(result.Users))
	}
	if result.Users[0].User != "integuser1" {
		t.Errorf("expected username 'integuser1', got '%s'", result.Users[0].User)
	}
	if len(result.Users[0].Roles) != 1 {
		t.Errorf("expected 1 role, got %d", len(result.Users[0].Roles))
	}
}

// INTEG-004: createUser with empty roles succeeds
func TestIntegration_CreateUser_NoRoles(t *testing.T) {
	client := newTestClient(t)

	user := DbUser{Name: "noroleuser", Password: "pass123"}
	err := createUser(client, user, nil, testAdminDB)
	if err != nil {
		t.Fatalf("createUser with no roles failed: %v", err)
	}

	result, err := getUser(client, "noroleuser", testAdminDB)
	if err != nil {
		t.Fatalf("getUser failed: %v", err)
	}
	if len(result.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(result.Users))
	}
	if len(result.Users[0].Roles) != 0 {
		t.Errorf("expected 0 roles, got %d", len(result.Users[0].Roles))
	}
}

// INTEG-005: getUser for non-existent user returns empty
func TestIntegration_GetUser_NonExistent(t *testing.T) {
	client := newTestClient(t)

	result, err := getUser(client, "nonexistent_user", testAdminDB)
	if err != nil {
		t.Fatalf("getUser failed: %v", err)
	}
	if len(result.Users) != 0 {
		t.Errorf("expected 0 users, got %d", len(result.Users))
	}
}

// INTEG-006: createRole then getRole returns matching role
func TestIntegration_CreateRole_GetRole(t *testing.T) {
	client := newTestClient(t)

	roleName := "integrole1"
	inheritedRoles := []Role{}
	privileges := []PrivilegeDto{
		{Db: "testdb", Collection: "testcol", Actions: []string{"find", "insert"}},
	}

	err := createRole(client, roleName, inheritedRoles, privileges, testAdminDB)
	if err != nil {
		t.Fatalf("createRole failed: %v", err)
	}

	result, err := getRole(client, roleName, testAdminDB)
	if err != nil {
		t.Fatalf("getRole failed: %v", err)
	}
	if len(result.Roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(result.Roles))
	}
	if result.Roles[0].Role != roleName {
		t.Errorf("expected role name '%s', got '%s'", roleName, result.Roles[0].Role)
	}
}

// INTEG-007: createRole with both Db and Cluster=true returns error
func TestIntegration_CreateRole_DbAndCluster_Error(t *testing.T) {
	client := newTestClient(t)

	roleName := "badrole1"
	privileges := []PrivilegeDto{
		{Db: "testdb", Cluster: true, Actions: []string{"find"}},
	}

	err := createRole(client, roleName, nil, privileges, testAdminDB)
	if err == nil {
		t.Fatal("expected error for Db+Cluster conflict, got nil")
	}
	if !strings.Contains(err.Error(), "cluster=true") {
		t.Errorf("expected error mentioning 'cluster=true', got: %v", err)
	}
}

// INTEG-008: getRole for non-existent role returns empty
func TestIntegration_GetRole_NonExistent(t *testing.T) {
	client := newTestClient(t)

	result, err := getRole(client, "nonexistent_role", testAdminDB)
	if err != nil {
		t.Fatalf("getRole failed: %v", err)
	}
	if len(result.Roles) != 0 {
		t.Errorf("expected 0 roles, got %d", len(result.Roles))
	}
}

// INTEG-009: GetReplSetConfig returns valid config
func TestIntegration_GetReplSetConfig(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	config, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig failed: %v", err)
	}
	if config.ID == "" {
		t.Error("expected non-empty replica set ID")
	}
	if len(config.Members) < 1 {
		t.Errorf("expected at least 1 member, got %d", len(config.Members))
	}
}

// INTEG-010: SetReplSetConfig updates settings that persist
func TestIntegration_SetReplSetConfig_UpdateSettings(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	config, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig failed: %v", err)
	}

	originalHBI := config.Settings.HeartbeatIntervalMillis
	newHBI := int64(3000)
	if originalHBI == newHBI {
		newHBI = 4000
	}

	config.Version++
	config.Settings.HeartbeatIntervalMillis = newHBI

	err = SetReplSetConfig(ctx, client, config)
	if err != nil {
		t.Fatalf("SetReplSetConfig failed: %v", err)
	}

	// Re-read to verify persistence
	updated, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after update failed: %v", err)
	}
	if updated.Settings.HeartbeatIntervalMillis != newHBI {
		t.Errorf("HeartbeatIntervalMillis: expected %d, got %d", newHBI, updated.Settings.HeartbeatIntervalMillis)
	}
}

// INTEG-011: GetReplSetStatus returns valid status
func TestIntegration_GetReplSetStatus_Basic(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	status, err := GetReplSetStatus(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetStatus failed: %v", err)
	}
	if status.Set != testReplSetName {
		t.Errorf("expected set name %q, got %q", testReplSetName, status.Set)
	}
	if len(status.Members) < 1 {
		t.Errorf("expected at least 1 member, got %d", len(status.Members))
	}
	if status.MyState != MemberStatePrimary {
		t.Errorf("expected MyState=PRIMARY (%d), got %d", MemberStatePrimary, status.MyState)
	}
}

// INTEG-012: GetReplSetStatus.GetSelf returns the self member
func TestIntegration_GetReplSetStatus_GetSelf(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	status, err := GetReplSetStatus(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetStatus failed: %v", err)
	}

	self := status.GetSelf()
	if self == nil {
		t.Fatal("GetSelf returned nil")
	}
	if !self.Self {
		t.Error("expected Self=true on self member")
	}
	if self.State != MemberStatePrimary {
		t.Errorf("expected state PRIMARY (%d), got %d", MemberStatePrimary, self.State)
	}
	if self.Health != MemberHealthUp {
		t.Errorf("expected health UP (%d), got %d", MemberHealthUp, self.Health)
	}
}

// INTEG-013: ReplSetStatus.Primary returns the primary member matching GetSelf
func TestIntegration_GetReplSetStatus_Primary(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	status, err := GetReplSetStatus(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetStatus failed: %v", err)
	}

	primary := status.Primary()
	if primary == nil {
		t.Fatal("Primary returned nil")
	}
	if primary.State != MemberStatePrimary {
		t.Errorf("expected state PRIMARY (%d), got %d", MemberStatePrimary, primary.State)
	}

	self := status.GetSelf()
	if self == nil {
		t.Fatal("GetSelf returned nil")
	}
	if primary.Name != self.Name {
		t.Errorf("Primary name %q does not match GetSelf name %q", primary.Name, self.Name)
	}
}

// INTEG-014: GetMembersByState(SECONDARY) returns empty on single-node RS
func TestIntegration_GetReplSetStatus_NoSecondaries(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	status, err := GetReplSetStatus(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetStatus failed: %v", err)
	}

	secondaries := status.GetMembersByState(MemberStateSecondary, 0)
	if len(secondaries) != 0 {
		t.Errorf("expected 0 secondaries on single-node RS, got %d", len(secondaries))
	}
}

// INTEG-015: createRole with Cluster=true and empty Db succeeds
func TestIntegration_CreateRole_ClusterPrivilege(t *testing.T) {
	client := newTestClient(t)

	roleName := "clusterrole1"
	privileges := []PrivilegeDto{
		{Cluster: true, Actions: []string{"listDatabases"}},
	}

	err := createRole(client, roleName, nil, privileges, testAdminDB)
	if err != nil {
		t.Fatalf("createRole with cluster privilege failed: %v", err)
	}

	result, err := getRole(client, roleName, testAdminDB)
	if err != nil {
		t.Fatalf("getRole failed: %v", err)
	}
	if len(result.Roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(result.Roles))
	}
	if len(result.Roles[0].Privileges) != 1 {
		t.Fatalf("expected 1 privilege, got %d", len(result.Roles[0].Privileges))
	}
	if !result.Roles[0].Privileges[0].Resource.Cluster {
		t.Error("expected privilege resource Cluster=true")
	}
}

// INTEG-016: SetReplSetConfig multi-setting update persists all changes
func TestIntegration_SetReplSetConfig_MultiSetting(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	config, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig failed: %v", err)
	}

	// Set multiple settings at once
	config.Version++
	config.Settings.ChainingAllowed = false
	newHBI := int64(3500)
	if config.Settings.HeartbeatIntervalMillis == newHBI {
		newHBI = 4500
	}
	config.Settings.HeartbeatIntervalMillis = newHBI
	newETM := int64(15000)
	if config.Settings.ElectionTimeoutMillis == newETM {
		newETM = 16000
	}
	config.Settings.ElectionTimeoutMillis = newETM

	err = SetReplSetConfig(ctx, client, config)
	if err != nil {
		t.Fatalf("SetReplSetConfig failed: %v", err)
	}

	updated, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after update failed: %v", err)
	}
	if updated.Settings.ChainingAllowed != false {
		t.Errorf("ChainingAllowed: expected false, got %v", updated.Settings.ChainingAllowed)
	}
	if updated.Settings.HeartbeatIntervalMillis != newHBI {
		t.Errorf("HeartbeatIntervalMillis: expected %d, got %d", newHBI, updated.Settings.HeartbeatIntervalMillis)
	}
	if updated.Settings.ElectionTimeoutMillis != newETM {
		t.Errorf("ElectionTimeoutMillis: expected %d, got %d", newETM, updated.Settings.ElectionTimeoutMillis)
	}
}

// INTEG-017: MergeMembers priority update persists through SetReplSetConfig round-trip
func TestIntegration_MergeMembers_PriorityUpdate(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	config, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig failed: %v", err)
	}
	if len(config.Members) < 1 {
		t.Fatal("need at least 1 member")
	}

	host := config.Members[0].Host
	newPriority := 3
	if config.Members[0].Priority == newPriority {
		newPriority = 4
	}

	overrides := []MemberOverride{
		{Host: host, Priority: newPriority, Votes: derefInt(config.Members[0].Votes), BuildIndexes: derefBool(config.Members[0].BuildIndexes)},
	}
	merged, err := MergeMembers(config.Members, overrides)
	if err != nil {
		t.Fatalf("MergeMembers failed: %v", err)
	}
	config.Members = merged
	config.Version++

	err = SetReplSetConfig(ctx, client, config)
	if err != nil {
		t.Fatalf("SetReplSetConfig failed: %v", err)
	}

	updated, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after update failed: %v", err)
	}
	if updated.Members[0].Priority != newPriority {
		t.Errorf("priority: want %d, got %d", newPriority, updated.Members[0].Priority)
	}
}

// INTEG-018: MergeMembers tags update persists through round-trip
func TestIntegration_MergeMembers_TagsUpdate(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	config, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig failed: %v", err)
	}
	if len(config.Members) < 1 {
		t.Fatal("need at least 1 member")
	}

	host := config.Members[0].Host
	overrides := []MemberOverride{
		{
			Host: host, Priority: config.Members[0].Priority,
			Votes: derefInt(config.Members[0].Votes), BuildIndexes: derefBool(config.Members[0].BuildIndexes),
			Tags: map[string]string{"dc": "east", "rack": "r1"},
		},
	}
	merged, err := MergeMembers(config.Members, overrides)
	if err != nil {
		t.Fatalf("MergeMembers failed: %v", err)
	}
	config.Members = merged
	config.Version++

	err = SetReplSetConfig(ctx, client, config)
	if err != nil {
		t.Fatalf("SetReplSetConfig failed: %v", err)
	}

	updated, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after update failed: %v", err)
	}
	if updated.Members[0].Tags["dc"] != "east" {
		t.Errorf("tags dc: want east, got %v", updated.Members[0].Tags["dc"])
	}
	if updated.Members[0].Tags["rack"] != "r1" {
		t.Errorf("tags rack: want r1, got %v", updated.Members[0].Tags["rack"])
	}
}

// INTEG-019: MergeMembers votes update persists through round-trip
func TestIntegration_MergeMembers_VotesUpdate(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	config, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig failed: %v", err)
	}
	if len(config.Members) < 1 {
		t.Fatal("need at least 1 member")
	}

	host := config.Members[0].Host
	overrides := []MemberOverride{
		{Host: host, Priority: config.Members[0].Priority, Votes: 1, BuildIndexes: derefBool(config.Members[0].BuildIndexes)},
	}
	merged, err := MergeMembers(config.Members, overrides)
	if err != nil {
		t.Fatalf("MergeMembers failed: %v", err)
	}
	config.Members = merged
	config.Version++

	err = SetReplSetConfig(ctx, client, config)
	if err != nil {
		t.Fatalf("SetReplSetConfig failed: %v", err)
	}

	updated, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after update failed: %v", err)
	}
	if derefInt(updated.Members[0].Votes) != 1 {
		t.Errorf("votes: want 1, got %d", derefInt(updated.Members[0].Votes))
	}
}

// INTEG-020: MergeMembers with bogus host returns error
func TestIntegration_MergeMembers_HostNotFound(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	config, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig failed: %v", err)
	}

	overrides := []MemberOverride{
		{Host: "nonexistent:99999", Priority: 1},
	}
	_, err = MergeMembers(config.Members, overrides)
	if err == nil {
		t.Fatal("expected error for nonexistent host, got nil")
	}
}

// INTEG-021: RSConfigMembersToState round-trip matches applied config
func TestIntegration_ReadMembers_RoundTrip(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	config, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig failed: %v", err)
	}
	if len(config.Members) < 1 {
		t.Fatal("need at least 1 member")
	}

	host := config.Members[0].Host
	managed := map[string]bool{host: true}
	state := RSConfigMembersToState(config.Members, managed)
	if len(state) != 1 {
		t.Fatalf("expected 1 member in state, got %d", len(state))
	}
	m := state[0].(map[string]interface{})
	if m["host"] != host {
		t.Errorf("host: want %s, got %v", host, m["host"])
	}
}

// Ensure testcontainers import is used (compile guard).
var _ testcontainers.Container = (*tcmongodb.MongoDBContainer)(nil)
