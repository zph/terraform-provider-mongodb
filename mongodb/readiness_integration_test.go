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
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// authRS is one mongod started with --replSet and a keyfile, which turns
// access control on, that is neither initiated nor holds any user: a seed
// host whose bootstrap has not run yet. Started lazily; tests skip if it
// cannot start. TestMain tears it down.
var authRS struct {
	once      sync.Once
	err       error
	container *testcontainers.DockerContainer
	host      string
	port      string
}

const authRSName = "rsauth"

// authRSCommand writes a keyfile the mongodb user can read and re-enters the
// image's entrypoint, which drops privileges, with mongod. A replica set with
// authorization requires a keyfile, and the image copies files in as root.
const authRSCommand = "printf terraformprovidermongodbtestkeyfile0123456789 > /tmp/keyfile" +
	" && chown mongodb /tmp/keyfile && chmod 400 /tmp/keyfile" +
	" && exec docker-entrypoint.sh mongod --replSet " + authRSName + " --port 27017 --bind_ip_all --keyFile /tmp/keyfile"

func setupAuthRS() error {
	ctx := context.Background()
	image := testMongoImage
	if env := os.Getenv("MONGO_TEST_IMAGE"); env != "" {
		image = env
	}
	natPort := nat.Port("27017/tcp")
	c, err := testcontainers.Run(ctx, image,
		testcontainers.WithCmd("bash", "-c", authRSCommand),
		testcontainers.WithExposedPorts(string(natPort)),
		testcontainers.WithWaitStrategy(wait.ForListeningPort(natPort).WithStartupTimeout(120*time.Second)),
	)
	if err != nil {
		return fmt.Errorf("start %s: %w", authRSName, err)
	}
	authRS.container = c
	host, err := c.Host(ctx)
	if err != nil {
		return err
	}
	mapped, err := c.MappedPort(ctx, natPort)
	if err != nil {
		return err
	}
	authRS.host, authRS.port = host, mapped.Port()
	return nil
}

func ensureAuthRS(t *testing.T) {
	t.Helper()
	authRS.once.Do(func() { authRS.err = setupAuthRS() })
	if authRS.err != nil {
		t.Skipf("replica set with a keyfile unavailable: %v", authRS.err)
	}
}

// teardownAuthRS is called from TestMain.
func teardownAuthRS() {
	if authRS.container != nil {
		_ = authRS.container.Terminate(context.Background())
	}
}

func authRSProviderConf(username, password string) *MongoDatabaseConfiguration {
	return &MongoDatabaseConfiguration{
		Config: &ClientConfig{
			Host:        authRS.host,
			Port:        authRS.port,
			Username:    username,
			Password:    password,
			DB:          "admin",
			Direct:      true,
			RetryWrites: false,
		},
		MaxConnLifetime: 10,
	}
}

// bootstrapAuthRS does what a seed host's bootstrap does: initiates the set
// and creates the first user, both through the localhost exception.
func bootstrapAuthRS(ctx context.Context) error {
	if err := initRS(ctx, authRS.container, authRSName, "localhost", "27017"); err != nil {
		return fmt.Errorf("initiate: %w", err)
	}
	if err := waitForPrimaryExec(ctx, authRS.container, "27017"); err != nil {
		return err
	}
	js := fmt.Sprintf(`db.getSiblingDB("admin").createUser({user:"%s",pwd:"%s",roles:["root"]})`, testAdminUser, testAdminPassword)
	if err := mongoExec(ctx, authRS.container, "27017", js); err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// INTEG-029: INIT-034, INIT-035 — the live answers classify as the wait
// expects: a refused login is not a connection error, and the unauthenticated
// probe tells a host with access control (Unauthorized) from one without
// (answered), although the provider user exists on neither.
func TestIntegration_Readiness_ClassifiesLiveAnswers(t *testing.T) {
	ensureAuthRS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// A user that no bootstrap ever creates, so the outcome does not depend
	// on whether INTEG-030 has run.
	withAuth := authRSProviderConf("ghost", "nope")
	_, err := MongoClientInit(ctx, withAuth)
	if err == nil {
		t.Fatal("a host with access control must refuse an unknown user")
	}
	if !IsAuthError(err) || IsConnectionError(err) {
		t.Errorf("a refused login should read as an auth error and not as a connection error: %v", err)
	}
	if got := classifyAuthProbe(authProbeFor(withAuth)(ctx)); got != authRequired {
		t.Errorf("probe of a host with access control: want authRequired (%d), got %d", authRequired, got)
	}

	noAuth := newTestConfig()
	noAuth.Config.Username, noAuth.Config.Password = "ghost", "nope"
	_, err = MongoClientInit(ctx, noAuth)
	if err == nil {
		t.Fatal("an unknown user must be refused even without access control")
	}
	if !IsAuthError(err) {
		t.Errorf("a refused login without access control should still read as an auth error: %v", err)
	}
	if got := classifyAuthProbe(authProbeFor(noAuth)(ctx)); got != authNotEnforced {
		t.Errorf("probe of a host without access control: want authNotEnforced (%d), got %d", authNotEnforced, got)
	}
}

// INTEG-030: INIT-033 through INIT-036 — Create against a keyfile seed host
// whose bootstrap runs while Create is already waiting. The provider user is
// refused until the bootstrap initiates the set and creates the user through
// the localhost exception; Create keeps waiting because the probe is refused
// as Unauthorized, then connects and applies the settings.
func TestIntegration_ShardConfigCreate_WaitsForBootstrap(t *testing.T) {
	ensureAuthRS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	conf := authRSProviderConf(testAdminUser, testAdminPassword)
	data := schema.TestResourceDataRaw(t, resourceShardConfig().Schema, map[string]interface{}{
		"shard_name":              authRSName,
		"election_timeout_millis": 7000,
		"init_timeout_secs":       120,
		"member": []interface{}{
			map[string]interface{}{"host": "localhost:27017", "priority": 1.0, "votes": 1},
		},
	})

	done := make(chan diag.Diagnostics, 1)
	go func() { done <- RShardConfig.Create(ctx, data, conf) }()

	// Create must be refused at least once before the bootstrap runs.
	select {
	case diags := <-done:
		t.Fatalf("Create returned before the bootstrap ran: %v", diags)
	case <-time.After(3 * time.Second):
	}
	if err := bootstrapAuthRS(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	diags := <-done
	if diags.HasError() {
		t.Fatalf("Create: %v", diags)
	}
	if data.Id() != authRSName {
		t.Errorf("resource id: want %s, got %q", authRSName, data.Id())
	}

	client, err := MongoClientInit(ctx, conf)
	if err != nil {
		t.Fatalf("connecting as the user the bootstrap created: %v", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()
	cfg, err := GetReplSetConfig(ctx, client)
	if err != nil {
		t.Fatalf("GetReplSetConfig: %v", err)
	}
	if cfg.Settings.ElectionTimeoutMillis != 7000 {
		t.Errorf("election timeout: want 7000 applied by Create, got %d", cfg.Settings.ElectionTimeoutMillis)
	}
	if len(cfg.Members) != 1 || cfg.Members[0].Host != "localhost:27017" {
		t.Errorf("members: want only localhost:27017, got %v", memberHosts(cfg.Members))
	}
}
