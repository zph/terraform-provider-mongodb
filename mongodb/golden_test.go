//go:build integration

package mongodb

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GOLDEN-009: WHEN a golden integration test runs, it SHALL capture all
// MongoDB commands for the resource lifecycle and compare against a golden file.
// GOLDEN-010: WHEN the shard config golden test runs, it SHALL normalize
// dynamic values (ObjectIDs, host:port, version numbers) before comparison.

const goldenTestFile = "mongodb/golden_test.go"

// newGoldenTestClient creates a CommandRecorder tagged with the test's source
// provenance and a mongo.Client wired to that recorder.
func newGoldenTestClient(t *testing.T) (*mongo.Client, *CommandRecorder) {
	t.Helper()

	source := fmt.Sprintf("%s (%s)", t.Name(), goldenTestFile)
	rec := NewCommandRecorder(source)

	uri := fmt.Sprintf("mongodb://%s:%s@%s:%s/?replicaSet=%s&directConnection=true&retrywrites=false",
		testAdminUser, testAdminPassword,
		testMongoContainer.host, testMongoContainer.port,
		testReplSetName)

	opts := options.Client().ApplyURI(uri).SetMonitor(rec.Monitor())
	client, err := mongo.Connect(context.Background(), opts)
	if err != nil {
		t.Fatalf("connect with recorder: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("ping with recorder: %v", err)
	}

	// Reset recorder after connection handshake noise.
	rec.Reset()

	t.Cleanup(func() {
		_ = client.Disconnect(context.Background())
	})
	return client, rec
}

// recordCreateUserBody pre-registers the createUser command body so the
// CommandRecorder can reconstruct it (the driver redacts sensitive commands).
func recordCreateUserBody(rec *CommandRecorder, user DbUser, roles []Role) {
	var roleVal interface{}
	if len(roles) != 0 {
		roleVal = roles
	} else {
		roleVal = []bson.M{}
	}
	rec.RecordBody("createUser", bson.D{
		{Key: "createUser", Value: user.Name},
		{Key: "pwd", Value: user.Password},
		{Key: "roles", Value: roleVal},
	})
}

// dropUserSafe drops a user, ignoring "not found" errors.
func dropUserSafe(client *mongo.Client, username, database string) {
	_ = client.Database(database).RunCommand(
		context.Background(),
		bson.D{{Key: "dropUser", Value: username}},
	).Err()
}

// dropRoleSafe drops a role, ignoring "not found" errors.
func dropRoleSafe(client *mongo.Client, roleName, database string) {
	_ = client.Database(database).RunCommand(
		context.Background(),
		bson.D{{Key: "dropRole", Value: roleName}},
	).Err()
}

// createRoleRaw creates a role using direct BSON construction to avoid the
// omitempty bug in Resource{} where collection="" gets dropped from BSON.
// MongoDB requires both db and collection to be present, or neither.
func createRoleRaw(client *mongo.Client, roleName string, inheritedRoles []Role, privileges []PrivilegeDto, database string) error {
	var privDocs bson.A
	for _, p := range privileges {
		var resource bson.D
		if p.Cluster {
			resource = bson.D{{Key: "cluster", Value: true}}
		} else {
			resource = bson.D{
				{Key: "db", Value: p.Db},
				{Key: "collection", Value: p.Collection},
			}
		}
		privDocs = append(privDocs, bson.D{
			{Key: "resource", Value: resource},
			{Key: "actions", Value: p.Actions},
		})
	}
	if privDocs == nil {
		privDocs = bson.A{}
	}

	var roleDocs bson.A
	for _, r := range inheritedRoles {
		roleDocs = append(roleDocs, bson.D{
			{Key: "role", Value: r.Role},
			{Key: "db", Value: r.Db},
		})
	}
	if roleDocs == nil {
		roleDocs = bson.A{}
	}

	cmd := bson.D{
		{Key: "createRole", Value: roleName},
		{Key: "privileges", Value: privDocs},
		{Key: "roles", Value: roleDocs},
	}
	result := client.Database(database).RunCommand(context.Background(), cmd)
	return result.Err()
}

// --- Golden Tests: db_user ---

// GOLDEN-011: WHEN TestGolden_DbUser_Basic runs, it SHALL capture createUser,
// usersInfo, dropUser+createUser (update), usersInfo, dropUser commands.
func TestGolden_DbUser_Basic(t *testing.T) {
	client, rec := newGoldenTestClient(t)
	db := "golden_app_db"

	t.Cleanup(func() { dropUserSafe(client, "golden_app_reader", db) })

	// Create
	recordCreateUserBody(rec, DbUser{Name: "golden_app_reader", Password: "testpass1"}, []Role{{Role: "read", Db: db}})
	err := createUser(client, DbUser{Name: "golden_app_reader", Password: "testpass1"}, []Role{{Role: "read", Db: db}}, db)
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}

	// Read
	_, err = getUser(client, "golden_app_reader", db)
	if err != nil {
		t.Fatalf("getUser: %v", err)
	}

	// Update (drop + recreate with different password)
	result := client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: "golden_app_reader"}})
	if result.Err() != nil {
		t.Fatalf("dropUser for update: %v", result.Err())
	}
	recordCreateUserBody(rec, DbUser{Name: "golden_app_reader", Password: "testpass2"}, []Role{{Role: "read", Db: db}})
	err = createUser(client, DbUser{Name: "golden_app_reader", Password: "testpass2"}, []Role{{Role: "read", Db: db}}, db)
	if err != nil {
		t.Fatalf("createUser (update): %v", err)
	}

	// Read after update
	_, err = getUser(client, "golden_app_reader", db)
	if err != nil {
		t.Fatalf("getUser after update: %v", err)
	}

	// Delete
	result = client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: "golden_app_reader"}})
	if result.Err() != nil {
		t.Fatalf("dropUser: %v", result.Err())
	}

	goldenCompare(t, "db_user_basic.golden", rec.String())
}

func TestGolden_DbUser_CustomRole(t *testing.T) {
	client, rec := newGoldenTestClient(t)
	db := "admin"

	t.Cleanup(func() {
		dropUserSafe(client, "golden_app_user", db)
		dropRoleSafe(client, "golden_app_readwrite_role", db)
	})

	// Create role
	err := createRoleRaw(client, "golden_app_readwrite_role", nil, []PrivilegeDto{
		{Db: "golden_app_db", Collection: "", Actions: []string{"find", "insert", "update", "remove"}},
	}, db)
	if err != nil {
		t.Fatalf("createRole: %v", err)
	}

	// Read role
	_, err = getRole(client, "golden_app_readwrite_role", db)
	if err != nil {
		t.Fatalf("getRole: %v", err)
	}

	// Create user with custom role + built-in role
	recordCreateUserBody(rec, DbUser{Name: "golden_app_user", Password: "testpass1"}, []Role{
		{Role: "golden_app_readwrite_role", Db: db},
		{Role: "read", Db: "config"},
	})
	err = createUser(client, DbUser{Name: "golden_app_user", Password: "testpass1"}, []Role{
		{Role: "golden_app_readwrite_role", Db: db},
		{Role: "read", Db: "config"},
	}, db)
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}

	// Read user
	_, err = getUser(client, "golden_app_user", db)
	if err != nil {
		t.Fatalf("getUser: %v", err)
	}

	// Delete user
	result := client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: "golden_app_user"}})
	if result.Err() != nil {
		t.Fatalf("dropUser: %v", result.Err())
	}

	// Delete role
	result = client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropRole", Value: "golden_app_readwrite_role"}})
	if result.Err() != nil {
		t.Fatalf("dropRole: %v", result.Err())
	}

	goldenCompare(t, "db_user_custom_role.golden", rec.String())
}

func TestGolden_DbUser_MultipleRoles(t *testing.T) {
	client, rec := newGoldenTestClient(t)
	db := "admin"

	t.Cleanup(func() { dropUserSafe(client, "golden_backend_svc", db) })

	// Create user with 4 roles
	recordCreateUserBody(rec, DbUser{Name: "golden_backend_svc", Password: "testpass1"}, []Role{
		{Role: "readWrite", Db: "orders"},
		{Role: "readWrite", Db: "inventory"},
		{Role: "read", Db: "analytics"},
		{Role: "clusterMonitor", Db: db},
	})
	err := createUser(client, DbUser{Name: "golden_backend_svc", Password: "testpass1"}, []Role{
		{Role: "readWrite", Db: "orders"},
		{Role: "readWrite", Db: "inventory"},
		{Role: "read", Db: "analytics"},
		{Role: "clusterMonitor", Db: db},
	}, db)
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}

	// Read
	_, err = getUser(client, "golden_backend_svc", db)
	if err != nil {
		t.Fatalf("getUser: %v", err)
	}

	// Update (drop + recreate)
	result := client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: "golden_backend_svc"}})
	if result.Err() != nil {
		t.Fatalf("dropUser for update: %v", result.Err())
	}
	recordCreateUserBody(rec, DbUser{Name: "golden_backend_svc", Password: "testpass2"}, []Role{
		{Role: "readWrite", Db: "orders"},
		{Role: "readWrite", Db: "inventory"},
		{Role: "read", Db: "analytics"},
		{Role: "clusterMonitor", Db: db},
	})
	err = createUser(client, DbUser{Name: "golden_backend_svc", Password: "testpass2"}, []Role{
		{Role: "readWrite", Db: "orders"},
		{Role: "readWrite", Db: "inventory"},
		{Role: "read", Db: "analytics"},
		{Role: "clusterMonitor", Db: db},
	}, db)
	if err != nil {
		t.Fatalf("createUser (update): %v", err)
	}

	// Read after update
	_, err = getUser(client, "golden_backend_svc", db)
	if err != nil {
		t.Fatalf("getUser after update: %v", err)
	}

	// Delete
	result = client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: "golden_backend_svc"}})
	if result.Err() != nil {
		t.Fatalf("dropUser: %v", result.Err())
	}

	goldenCompare(t, "db_user_multiple_roles.golden", rec.String())
}

func TestGolden_DbUser_Import(t *testing.T) {
	client, rec := newGoldenTestClient(t)
	db := "admin"

	t.Cleanup(func() { dropUserSafe(client, "golden_existing_user", db) })

	// Setup: pre-create the user (simulating existing user)
	recordCreateUserBody(rec, DbUser{Name: "golden_existing_user", Password: "existingpass"}, []Role{
		{Role: "readWriteAnyDatabase", Db: db},
	})
	err := createUser(client, DbUser{Name: "golden_existing_user", Password: "existingpass"}, []Role{
		{Role: "readWriteAnyDatabase", Db: db},
	}, db)
	if err != nil {
		t.Fatalf("setup createUser: %v", err)
	}

	// Import read (what terraform import does)
	_, err = getUser(client, "golden_existing_user", db)
	if err != nil {
		t.Fatalf("getUser (import): %v", err)
	}

	// Cleanup
	result := client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: "golden_existing_user"}})
	if result.Err() != nil {
		t.Fatalf("dropUser: %v", result.Err())
	}

	goldenCompare(t, "db_user_import.golden", rec.String())
}

// --- Golden Tests: db_role ---

func TestGolden_DbRole_Basic(t *testing.T) {
	client, rec := newGoldenTestClient(t)
	db := "admin"

	t.Cleanup(func() { dropRoleSafe(client, "golden_analyst_role", db) })

	// Create
	err := createRoleRaw(client, "golden_analyst_role", nil, []PrivilegeDto{
		{Db: "analytics", Collection: "", Actions: []string{"find", "collStats", "dbStats", "listCollections"}},
	}, db)
	if err != nil {
		t.Fatalf("createRole: %v", err)
	}

	// Read
	_, err = getRole(client, "golden_analyst_role", db)
	if err != nil {
		t.Fatalf("getRole: %v", err)
	}

	// Update (drop + recreate)
	result := client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropRole", Value: "golden_analyst_role"}})
	if result.Err() != nil {
		t.Fatalf("dropRole for update: %v", result.Err())
	}
	err = createRoleRaw(client, "golden_analyst_role", nil, []PrivilegeDto{
		{Db: "analytics", Collection: "", Actions: []string{"find", "collStats", "dbStats", "listCollections"}},
	}, db)
	if err != nil {
		t.Fatalf("createRole (update): %v", err)
	}

	// Read after update
	_, err = getRole(client, "golden_analyst_role", db)
	if err != nil {
		t.Fatalf("getRole after update: %v", err)
	}

	// Delete
	result = client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropRole", Value: "golden_analyst_role"}})
	if result.Err() != nil {
		t.Fatalf("dropRole: %v", result.Err())
	}

	goldenCompare(t, "db_role_basic.golden", rec.String())
}

func TestGolden_DbRole_ClusterPrivilege(t *testing.T) {
	client, rec := newGoldenTestClient(t)
	db := "admin"

	t.Cleanup(func() { dropRoleSafe(client, "golden_failover_operator", db) })

	// Create
	err := createRoleRaw(client, "golden_failover_operator", nil, []PrivilegeDto{
		{Cluster: true, Actions: []string{"replSetGetConfig", "replSetGetStatus", "replSetStateChange"}},
	}, db)
	if err != nil {
		t.Fatalf("createRole: %v", err)
	}

	// Read
	_, err = getRole(client, "golden_failover_operator", db)
	if err != nil {
		t.Fatalf("getRole: %v", err)
	}

	// Delete
	result := client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropRole", Value: "golden_failover_operator"}})
	if result.Err() != nil {
		t.Fatalf("dropRole: %v", result.Err())
	}

	goldenCompare(t, "db_role_cluster_privilege.golden", rec.String())
}

func TestGolden_DbRole_Composite(t *testing.T) {
	client, rec := newGoldenTestClient(t)
	db := "admin"

	t.Cleanup(func() {
		dropRoleSafe(client, "golden_admin_composite_role", db)
		dropRoleSafe(client, "golden_custom_monitoring", db)
		dropRoleSafe(client, "golden_custom_data_access", db)
	})

	// Create leaf role: monitoring
	err := createRoleRaw(client, "golden_custom_monitoring", nil, []PrivilegeDto{
		{Cluster: true, Actions: []string{"replSetGetStatus", "serverStatus"}},
	}, db)
	if err != nil {
		t.Fatalf("createRole monitoring: %v", err)
	}

	// Read monitoring
	_, err = getRole(client, "golden_custom_monitoring", db)
	if err != nil {
		t.Fatalf("getRole monitoring: %v", err)
	}

	// Create leaf role: data access
	err = createRoleRaw(client, "golden_custom_data_access", nil, []PrivilegeDto{
		{Db: "orders", Collection: "", Actions: []string{"find", "insert", "update", "remove", "createIndex"}},
		{Db: "orders", Collection: "audit_log", Actions: []string{"find"}},
	}, db)
	if err != nil {
		t.Fatalf("createRole data_access: %v", err)
	}

	// Read data access
	_, err = getRole(client, "golden_custom_data_access", db)
	if err != nil {
		t.Fatalf("getRole data_access: %v", err)
	}

	// Create composite role inheriting both
	err = createRoleRaw(client, "golden_admin_composite_role",
		[]Role{
			{Role: "golden_custom_monitoring", Db: db},
			{Role: "golden_custom_data_access", Db: db},
		},
		[]PrivilegeDto{
			{Db: db, Collection: "", Actions: []string{"collStats", "dbStats", "listCollections"}},
		}, db)
	if err != nil {
		t.Fatalf("createRole composite: %v", err)
	}

	// Read composite
	_, err = getRole(client, "golden_admin_composite_role", db)
	if err != nil {
		t.Fatalf("getRole composite: %v", err)
	}

	// Delete all (reverse dependency order)
	for _, name := range []string{"golden_admin_composite_role", "golden_custom_data_access", "golden_custom_monitoring"} {
		result := client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropRole", Value: name}})
		if result.Err() != nil {
			t.Fatalf("dropRole %s: %v", name, result.Err())
		}
	}

	goldenCompare(t, "db_role_composite.golden", rec.String())
}

func TestGolden_DbRole_Inherited(t *testing.T) {
	client, rec := newGoldenTestClient(t)
	db := "admin"

	t.Cleanup(func() {
		dropRoleSafe(client, "golden_extended_operations", db)
		dropRoleSafe(client, "golden_base_operations", db)
	})

	// Create base role
	err := createRoleRaw(client, "golden_base_operations", nil, []PrivilegeDto{
		{Db: "golden_app_db", Collection: "", Actions: []string{"find", "insert", "update"}},
	}, db)
	if err != nil {
		t.Fatalf("createRole base: %v", err)
	}

	// Read base
	_, err = getRole(client, "golden_base_operations", db)
	if err != nil {
		t.Fatalf("getRole base: %v", err)
	}

	// Create derived role
	err = createRoleRaw(client, "golden_extended_operations",
		[]Role{{Role: "golden_base_operations", Db: db}},
		nil, db)
	if err != nil {
		t.Fatalf("createRole derived: %v", err)
	}

	// Read derived
	_, err = getRole(client, "golden_extended_operations", db)
	if err != nil {
		t.Fatalf("getRole derived: %v", err)
	}

	// Delete (reverse order)
	for _, name := range []string{"golden_extended_operations", "golden_base_operations"} {
		result := client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropRole", Value: name}})
		if result.Err() != nil {
			t.Fatalf("dropRole %s: %v", name, result.Err())
		}
	}

	goldenCompare(t, "db_role_inherited.golden", rec.String())
}

// --- Golden Tests: shard_config ---

// normalizeReplSetBody replaces dynamic values in replSetReconfig/replSetGetConfig
// output with stable placeholders for golden comparison.
func normalizeReplSetBody(output string) string {
	// ObjectID hex strings (24 hex chars)
	reOID := regexp.MustCompile(`[0-9a-f]{24}`)
	output = reOID.ReplaceAllString(output, "<OBJECT_ID>")

	// host:port patterns from container (e.g., "localhost:55123" or "172.17.0.2:27017")
	reHostPort := regexp.MustCompile(`"[a-zA-Z0-9._-]+:\d{4,5}"`)
	output = reHostPort.ReplaceAllString(output, `"<HOST:PORT>"`)

	// Version numbers in replSetReconfig
	reVersion := regexp.MustCompile(`"version":\s*\d+`)
	output = reVersion.ReplaceAllString(output, `"version": <VERSION>`)

	// Term numbers
	reTerm := regexp.MustCompile(`"term":\s*\d+`)
	output = reTerm.ReplaceAllString(output, `"term": <TERM>`)

	return output
}

// GOLDEN-019: WHEN normalizeShardedBody processes output, it SHALL normalize
// shard host strings (e.g. "shard01/host:port") with <SHARD_HOST> and shard
// state values with <SHARD_STATE>, in addition to all replSetBody normalizations.
func normalizeShardedBody(output string) string {
	output = normalizeReplSetBody(output)

	// Shard host strings like "shard01/shard01svr0:27018" or "shard02/shard02svr0:27018"
	reShardHost := regexp.MustCompile(`"[a-zA-Z0-9_-]+/[a-zA-Z0-9._-]+:\d{4,5}"`)
	output = reShardHost.ReplaceAllString(output, `"<SHARD_HOST>"`)

	// Shard state field (integer, typically 1)
	reShardState := regexp.MustCompile(`"state":\s*\d+`)
	output = reShardState.ReplaceAllString(output, `"state": <SHARD_STATE>`)

	return output
}

// newGoldenMongosClient creates a CommandRecorder-wired client for the mongos
// router. Handshake noise (hello, saslStart, etc.) is already filtered by the
// recorder, so no reset is needed here. Tests manage rec.Reset() explicitly.
func newGoldenMongosClient(t *testing.T, rec *CommandRecorder) *mongo.Client {
	t.Helper()

	uri := fmt.Sprintf("mongodb://%s:%s@%s:%s/?retrywrites=false",
		shardedAdminUser, shardedAdminPassword,
		shardedCluster.mongosHost, shardedCluster.mongosPort)

	opts := options.Client().ApplyURI(uri).SetMonitor(rec.Monitor())
	client, err := mongo.Connect(context.Background(), opts)
	if err != nil {
		t.Fatalf("connect to mongos with recorder: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("ping mongos with recorder: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Disconnect(context.Background())
	})
	return client
}

// newGoldenShardedClient creates a CommandRecorder-wired direct client for a
// shard. Handshake noise is already filtered by the recorder, so no reset is
// needed here. Tests manage rec.Reset() explicitly.
func newGoldenShardedClient(t *testing.T, rec *CommandRecorder, host, port, rsName string) *mongo.Client {
	t.Helper()

	uri := fmt.Sprintf("mongodb://%s:%s@%s:%s/?replicaSet=%s&directConnection=true&retrywrites=false",
		shardedAdminUser, shardedAdminPassword,
		host, port, rsName)

	opts := options.Client().ApplyURI(uri).SetMonitor(rec.Monitor())
	client, err := mongo.Connect(context.Background(), opts)
	if err != nil {
		t.Fatalf("connect to shard %s with recorder: %v", rsName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("ping shard %s with recorder: %v", rsName, err)
	}

	t.Cleanup(func() {
		_ = client.Disconnect(context.Background())
	})
	return client
}

func TestGolden_ShardConfig_Basic(t *testing.T) {
	client, rec := newGoldenTestClient(t)
	ctx := context.Background()

	// Read current config
	_, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig: %v", err)
	}

	// Perform a settings update (simulates terraform apply)
	config, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig for update: %v", err)
	}

	// Reset to capture only the update operations
	rec.Reset()

	config.Version++
	config.Settings.ChainingAllowed = true
	config.Settings.HeartbeatIntervalMillis = 1000
	config.Settings.HeartbeatTimeoutSecs = 10
	config.Settings.ElectionTimeoutMillis = 10000

	err = SetReplSetConfig(ctx, client, config)
	if err != nil {
		t.Fatalf("SetReplSetConfig: %v", err)
	}

	// Read after update
	_, err = GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after update: %v", err)
	}

	output := normalizeReplSetBody(rec.String())
	goldenCompare(t, "shard_config_basic.golden", output)
}

// GOLDEN-020: WHEN TestGolden_ShardConfig_MongosDiscovery runs, it SHALL capture
// listShards via mongos, then replSetGetConfig, replSetReconfig, replSetGetConfig
// on a shard, and compare against a golden file with sharded normalization.
func TestGolden_ShardConfig_MongosDiscovery(t *testing.T) {
	ensureShardedCluster(t)

	source := fmt.Sprintf("%s (%s)", t.Name(), goldenTestFile)
	rec := NewCommandRecorder(source)
	ctx := context.Background()

	// Step 1: discover shards via mongos
	mongosClient := newGoldenMongosClient(t, rec)

	_, err := ListShards(ctx, mongosClient)
	if err != nil {
		t.Fatalf("ListShards: %v", err)
	}

	// Reset — golden file only captures shard RS operations
	rec.Reset()

	// Step 2: direct shard operations on shard01
	shard01Client := newGoldenShardedClient(t, rec,
		shardedCluster.shard01Host, shardedCluster.shard01Port, "shard01")

	config, err := GetReplSetConfig(ctx, shard01Client)
	if err != nil {
		t.Fatalf("GetReplSetConfig: %v", err)
	}

	config.Version++
	config.Settings.ChainingAllowed = true
	config.Settings.HeartbeatIntervalMillis = 1000
	config.Settings.HeartbeatTimeoutSecs = 10
	config.Settings.ElectionTimeoutMillis = 10000

	err = SetReplSetConfig(ctx, shard01Client, config)
	if err != nil {
		t.Fatalf("SetReplSetConfig: %v", err)
	}

	_, err = GetReplSetConfig(ctx, shard01Client)
	if err != nil {
		t.Fatalf("GetReplSetConfig after update: %v", err)
	}

	output := normalizeShardedBody(rec.String())
	goldenCompare(t, "shard_config_mongos_discovery.golden", output)
}

// GOLDEN-021: WHEN TestGolden_ShardConfig_MultiShard runs, it SHALL capture
// listShards via mongos, then independent replSetGetConfig on both shard01 and
// shard02, and compare against a golden file with sharded normalization.
func TestGolden_ShardConfig_MultiShard(t *testing.T) {
	ensureShardedCluster(t)

	source := fmt.Sprintf("%s (%s)", t.Name(), goldenTestFile)
	rec := NewCommandRecorder(source)
	ctx := context.Background()

	// Step 1: discover shards via mongos
	mongosClient := newGoldenMongosClient(t, rec)

	_, err := ListShards(ctx, mongosClient)
	if err != nil {
		t.Fatalf("ListShards: %v", err)
	}

	// Step 2: read config from shard01
	shard01Client := newGoldenShardedClient(t, rec,
		shardedCluster.shard01Host, shardedCluster.shard01Port, "shard01")

	_, err = GetReplSetConfig(ctx, shard01Client)
	if err != nil {
		t.Fatalf("GetReplSetConfig shard01: %v", err)
	}

	// Step 3: read config from shard02
	shard02Client := newGoldenShardedClient(t, rec,
		shardedCluster.shard02Host, shardedCluster.shard02Port, "shard02")

	_, err = GetReplSetConfig(ctx, shard02Client)
	if err != nil {
		t.Fatalf("GetReplSetConfig shard02: %v", err)
	}

	output := normalizeShardedBody(rec.String())
	goldenCompare(t, "shard_config_multi_shard.golden", output)
}

// --- Golden Tests: original_user ---

func TestGolden_OriginalUser(t *testing.T) {
	client, rec := newGoldenTestClient(t)
	db := "admin"

	t.Cleanup(func() { dropUserSafe(client, "golden_orig_admin", db) })

	// Create (simulates original_user create — same createUser command)
	recordCreateUserBody(rec, DbUser{Name: "golden_orig_admin", Password: "origpass"}, []Role{
		{Role: "root", Db: db},
	})
	err := createUser(client, DbUser{Name: "golden_orig_admin", Password: "origpass"}, []Role{
		{Role: "root", Db: db},
	}, db)
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}

	// Read
	_, err = getUser(client, "golden_orig_admin", db)
	if err != nil {
		t.Fatalf("getUser: %v", err)
	}

	// Delete
	result := client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: "golden_orig_admin"}})
	if result.Err() != nil {
		t.Fatalf("dropUser: %v", result.Err())
	}

	goldenCompare(t, "original_user.golden", rec.String())
}

// --- Golden Tests: patterns ---

// GOLDEN-012: WHEN TestGolden_Pattern_MonitoringUser runs, it SHALL capture
// the full lifecycle of a monitoring role + exporter user.
func TestGolden_Pattern_MonitoringUser(t *testing.T) {
	client, rec := newGoldenTestClient(t)
	db := "admin"

	t.Cleanup(func() {
		dropUserSafe(client, "golden_mongodb_exporter", db)
		dropRoleSafe(client, "golden_metrics_exporter", db)
	})

	// Create role with 3 privileges
	err := createRoleRaw(client, "golden_metrics_exporter", nil, []PrivilegeDto{
		{Cluster: true, Actions: []string{"serverStatus", "replSetGetStatus"}},
		{Db: "", Collection: "", Actions: []string{"dbStats", "collStats", "indexStats"}},
		{Db: "local", Collection: "oplog.rs", Actions: []string{"find"}},
	}, db)
	if err != nil {
		t.Fatalf("createRole: %v", err)
	}

	// Read role
	_, err = getRole(client, "golden_metrics_exporter", db)
	if err != nil {
		t.Fatalf("getRole: %v", err)
	}

	// Create user with custom + built-in role
	recordCreateUserBody(rec, DbUser{Name: "golden_mongodb_exporter", Password: "exporterpass"}, []Role{
		{Role: "golden_metrics_exporter", Db: db},
		{Role: "clusterMonitor", Db: db},
	})
	err = createUser(client, DbUser{Name: "golden_mongodb_exporter", Password: "exporterpass"}, []Role{
		{Role: "golden_metrics_exporter", Db: db},
		{Role: "clusterMonitor", Db: db},
	}, db)
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}

	// Read user
	_, err = getUser(client, "golden_mongodb_exporter", db)
	if err != nil {
		t.Fatalf("getUser: %v", err)
	}

	// Delete user
	result := client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: "golden_mongodb_exporter"}})
	if result.Err() != nil {
		t.Fatalf("dropUser: %v", result.Err())
	}

	// Delete role
	result = client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropRole", Value: "golden_metrics_exporter"}})
	if result.Err() != nil {
		t.Fatalf("dropRole: %v", result.Err())
	}

	goldenCompare(t, "pattern_monitoring_user.golden", rec.String())
}

// GOLDEN-013: WHEN TestGolden_Pattern_RoleHierarchy runs, it SHALL capture
// the full lifecycle of a 3-tier role hierarchy with 3 users.
func TestGolden_Pattern_RoleHierarchy(t *testing.T) {
	client, rec := newGoldenTestClient(t)
	db := "admin"

	t.Cleanup(func() {
		dropUserSafe(client, "golden_admin_user", db)
		dropUserSafe(client, "golden_editor_user", db)
		dropUserSafe(client, "golden_viewer_user", db)
		dropRoleSafe(client, "golden_app_admin", db)
		dropRoleSafe(client, "golden_app_editor", db)
		dropRoleSafe(client, "golden_app_viewer", db)
	})

	// Layer 1: viewer role
	err := createRoleRaw(client, "golden_app_viewer", nil, []PrivilegeDto{
		{Db: "golden_app_db", Collection: "", Actions: []string{"find", "listCollections"}},
		{Db: "golden_app_db", Collection: "", Actions: []string{"collStats", "dbStats"}},
	}, db)
	if err != nil {
		t.Fatalf("createRole viewer: %v", err)
	}
	_, err = getRole(client, "golden_app_viewer", db)
	if err != nil {
		t.Fatalf("getRole viewer: %v", err)
	}

	// Layer 2: editor inherits viewer
	err = createRoleRaw(client, "golden_app_editor",
		[]Role{{Role: "golden_app_viewer", Db: db}},
		[]PrivilegeDto{
			{Db: "golden_app_db", Collection: "", Actions: []string{"insert", "update", "remove"}},
		}, db)
	if err != nil {
		t.Fatalf("createRole editor: %v", err)
	}
	_, err = getRole(client, "golden_app_editor", db)
	if err != nil {
		t.Fatalf("getRole editor: %v", err)
	}

	// Layer 3: admin inherits editor
	err = createRoleRaw(client, "golden_app_admin",
		[]Role{{Role: "golden_app_editor", Db: db}},
		[]PrivilegeDto{
			{Db: "golden_app_db", Collection: "", Actions: []string{"createIndex", "dropIndex", "createCollection", "dropCollection"}},
		}, db)
	if err != nil {
		t.Fatalf("createRole admin: %v", err)
	}
	_, err = getRole(client, "golden_app_admin", db)
	if err != nil {
		t.Fatalf("getRole admin: %v", err)
	}

	// Create users at each tier
	for _, u := range []struct {
		name string
		role string
	}{
		{"golden_viewer_user", "golden_app_viewer"},
		{"golden_editor_user", "golden_app_editor"},
		{"golden_admin_user", "golden_app_admin"},
	} {
		recordCreateUserBody(rec, DbUser{Name: u.name, Password: u.name + "_pass"}, []Role{
			{Role: u.role, Db: db},
		})
		err = createUser(client, DbUser{Name: u.name, Password: u.name + "_pass"}, []Role{
			{Role: u.role, Db: db},
		}, db)
		if err != nil {
			t.Fatalf("createUser %s: %v", u.name, err)
		}
		_, err = getUser(client, u.name, db)
		if err != nil {
			t.Fatalf("getUser %s: %v", u.name, err)
		}
	}

	// Delete users
	for _, name := range []string{"golden_admin_user", "golden_editor_user", "golden_viewer_user"} {
		result := client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: name}})
		if result.Err() != nil {
			t.Fatalf("dropUser %s: %v", name, result.Err())
		}
	}

	// Delete roles (reverse dependency order)
	for _, name := range []string{"golden_app_admin", "golden_app_editor", "golden_app_viewer"} {
		result := client.Database(db).RunCommand(context.Background(), bson.D{{Key: "dropRole", Value: name}})
		if result.Err() != nil {
			t.Fatalf("dropRole %s: %v", name, result.Err())
		}
	}

	goldenCompare(t, "pattern_role_hierarchy.golden", rec.String())
}

// --- Golden Tests: shard ---

// startShard03Container starts a third shard container for add/remove lifecycle testing.
// It returns the container, mapped host, mapped port, and a cleanup function.
func startShard03Container(t *testing.T) (host, port string) {
	t.Helper()
	ctx := context.Background()

	image := testMongoImage
	if env := os.Getenv("MONGO_TEST_IMAGE"); env != "" {
		image = env
	}

	natPort := nat.Port("27018/tcp")
	c, err := testcontainers.Run(ctx, image,
		network.WithNetwork([]string{"shard03svr0"}, shardedCluster.network),
		testcontainers.WithCmd(
			"mongod", "--shardsvr", "--replSet", "shard03", "--port", "27018", "--bind_ip_all",
		),
		testcontainers.WithExposedPorts(string(natPort)),
		testcontainers.WithWaitStrategy(wait.ForListeningPort(natPort).WithStartupTimeout(120*time.Second)),
	)
	if err != nil {
		t.Fatalf("start shard03 container: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })

	// Init replica set
	if err := initRS(ctx, c, "shard03", "shard03svr0", "27018"); err != nil {
		t.Fatalf("init shard03 RS: %v", err)
	}
	if err := waitForPrimaryExec(ctx, c, "27018"); err != nil {
		t.Fatalf("wait shard03 primary: %v", err)
	}

	h, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("shard03 host: %v", err)
	}
	mapped, err := c.MappedPort(ctx, "27018")
	if err != nil {
		t.Fatalf("shard03 port: %v", err)
	}

	return h, mapped.Port()
}

// removeShard03Safe runs removeShard in a loop until completed or timeout, ignoring errors.
func removeShard03Safe(client *mongo.Client) {
	ctx := context.Background()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		res := client.Database("admin").RunCommand(ctx, bson.D{{Key: "removeShard", Value: "shard03"}})
		if res.Err() != nil {
			return
		}
		var resp ShardRemoveResp
		if err := res.Decode(&resp); err != nil {
			return
		}
		if resp.State == ShardRemoveCompleted {
			return
		}
		time.Sleep(2 * time.Second)
	}
}

// GOLDEN-022: WHEN TestGolden_Shard_AddRemove runs, it SHALL capture the full
// addShard + listShards + removeShard lifecycle and compare against a golden file.
func TestGolden_Shard_AddRemove(t *testing.T) {
	ensureShardedCluster(t)

	// Start shard03 container on the shared network
	_, _ = startShard03Container(t)

	source := fmt.Sprintf("%s (%s)", t.Name(), goldenTestFile)
	rec := NewCommandRecorder(source)
	ctx := context.Background()

	mongosClient := newGoldenMongosClient(t, rec)
	t.Cleanup(func() { removeShard03Safe(mongosClient) })

	// Reset recorder — only capture addShard onward
	rec.Reset()

	// Create: addShard
	connStr := BuildShardConnectionString("shard03", []string{"shard03svr0:27018"})
	res := mongosClient.Database("admin").RunCommand(ctx, bson.D{
		{Key: "addShard", Value: connStr},
	})
	if res.Err() != nil {
		t.Fatalf("addShard: %v", res.Err())
	}
	var addResp OKResponse
	if err := res.Decode(&addResp); err != nil {
		t.Fatalf("addShard decode: %v", err)
	}
	if addResp.OK != 1 {
		t.Fatalf("addShard failed: %s", addResp.Errmsg)
	}

	// Read: listShards
	shards, err := ListShards(ctx, mongosClient)
	if err != nil {
		t.Fatalf("ListShards: %v", err)
	}

	found := false
	for _, s := range shards.Shards {
		if s.ID == "shard03" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("shard03 not found in listShards after addShard")
	}

	// Delete: removeShard (poll until completed)
	deadline := time.Now().Add(60 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("removeShard did not complete within 60s")
		}

		res := mongosClient.Database("admin").RunCommand(ctx, bson.D{
			{Key: "removeShard", Value: "shard03"},
		})
		if res.Err() != nil {
			t.Fatalf("removeShard: %v", res.Err())
		}

		var rmResp ShardRemoveResp
		if err := res.Decode(&rmResp); err != nil {
			t.Fatalf("removeShard decode: %v", err)
		}
		if rmResp.OK != 1 {
			t.Fatalf("removeShard failed: %s", rmResp.Msg)
		}

		if rmResp.State == ShardRemoveCompleted {
			break
		}

		time.Sleep(2 * time.Second)
	}

	// Verify removal: listShards should no longer contain shard03
	shards, err = ListShards(ctx, mongosClient)
	if err != nil {
		t.Fatalf("ListShards after remove: %v", err)
	}
	for _, s := range shards.Shards {
		if s.ID == "shard03" {
			t.Error("shard03 still found in listShards after removal")
		}
	}

	output := normalizeShardedBody(rec.String())
	goldenCompare(t, "shard_add_remove.golden", output)
}

// GOLDEN-023: WHEN TestGolden_Shard_ListShards runs against the existing
// sharded cluster, it SHALL capture the listShards command output and compare
// against a golden file with sharded normalization.
func TestGolden_Shard_ListShards(t *testing.T) {
	ensureShardedCluster(t)

	source := fmt.Sprintf("%s (%s)", t.Name(), goldenTestFile)
	rec := NewCommandRecorder(source)
	ctx := context.Background()

	mongosClient := newGoldenMongosClient(t, rec)

	// Reset to only capture listShards
	rec.Reset()

	shards, err := ListShards(ctx, mongosClient)
	if err != nil {
		t.Fatalf("ListShards: %v", err)
	}
	if len(shards.Shards) < 2 {
		t.Fatalf("expected at least 2 shards, got %d", len(shards.Shards))
	}

	output := normalizeShardedBody(rec.String())
	goldenCompare(t, "shard_list_shards.golden", output)
}
