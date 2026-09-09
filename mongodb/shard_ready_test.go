package mongodb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"go.mongodb.org/mongo-driver/mongo"
)

// errNotListening is the driver's shape for a host that is not listening or
// does not resolve.
var errNotListening = fmt.Errorf("server selection error: %w, current topology: { Type: Single, Servers: [{ Addr: rs2-1:27017, Type: Unknown, Last error: dial tcp: connect: connection refused }, ] }", context.DeadlineExceeded)

// errRefusedLogin is the shape of a SCRAM failure during the handshake.
var errRefusedLogin = errors.New("connection() error occurred during connection handshake: auth error: sasl conversation error: unable to authenticate using mechanism \"SCRAM-SHA-256\": (AuthenticationFailed) Authentication failed.")

// errUnauthorizedProbe is replSetGetStatus without credentials on a host with
// access control.
var errUnauthorizedProbe = fmt.Errorf("replSetGetStatus: %w", mongo.CommandError{Code: MongoErrUnauthorized, Name: "Unauthorized", Message: "command replSetGetStatus requires authentication"})

type fakeNetTimeout struct{}

func (fakeNetTimeout) Error() string   { return "read tcp: i/o timeout" }
func (fakeNetTimeout) Timeout() bool   { return true }
func (fakeNetTimeout) Temporary() bool { return true }

// testPoll keeps the waits under test fast.
const testPoll = time.Millisecond

// READY-T01: INIT-034 — IsConnectionError.
func TestIsConnectionError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"not listening", errNotListening, true},
		{"not listening, wrapped by getShardClient", fmt.Errorf("Error connecting to database: %w", errNotListening), true},
		{"not listening, wrapped by ConnectForInit", fmt.Errorf("failed to connect to rs2-1:27017 (auth and no-auth both failed): %w", errNotListening), true},
		{"network timeout", fakeNetTimeout{}, true},
		{"dropped mid-command", mongo.CommandError{Code: 6, Name: "HostUnreachable", Labels: []string{"NetworkError"}}, true},
		{"refused login", errRefusedLogin, false},
		{"unauthorized", mongo.CommandError{Code: MongoErrUnauthorized}, false},
		{"not yet initialized", mongo.CommandError{Code: MongoErrNotYetInitialized}, false},
		{"maxTimeMS expired is an answer", mongo.CommandError{Code: 50, Name: "MaxTimeMSExpired"}, false},
		{"interrupted", context.Canceled, false},
		{"anything else", errors.New("shard \"rs9\" not found; available shards: [rs2]"), false},
	}
	for _, tc := range cases {
		if got := IsConnectionError(tc.err); got != tc.want {
			t.Errorf("%s: IsConnectionError = %v, want %v (%v)", tc.name, got, tc.want, tc.err)
		}
	}
}

// READY-T02: INIT-034 — a real refused dial is a connection error.
func TestIsConnectionError_RefusedDial(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := fmt.Sprint(l.Addr().(*net.TCPAddr).Port)
	_ = l.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err = MongoClientInit(ctx, &MongoDatabaseConfiguration{
		Config:          &ClientConfig{Host: "127.0.0.1", Port: port, DB: "admin", Username: "admin", Password: "pw", Direct: true},
		MaxConnLifetime: 5,
	})
	if err == nil {
		t.Fatal("connecting to a closed port should fail")
	}
	if !IsConnectionError(err) {
		t.Errorf("want a connection error, got %T: %v", err, err)
	}
	if IsAuthError(err) {
		t.Errorf("a refused dial must not read as a refused login: %v", err)
	}
}

// READY-T03: INIT-035 — classifyAuthProbe.
func TestClassifyAuthProbe(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want authRequirement
	}{
		{"answered", nil, authNotEnforced},
		{"unauthorized", errUnauthorizedProbe, authRequired},
		{"not yet initialized, without access control", fmt.Errorf("replSetGetStatus: %w", mongo.CommandError{Code: MongoErrNotYetInitialized}), authNotEnforced},
		{"no replication, without access control", mongo.CommandError{Code: 76, Name: "NoReplicationEnabled"}, authNotEnforced},
		{"not listening", errNotListening, authUnknown},
		{"undecodable", errors.New("failed to decode replSetGetStatus"), authUnknown},
	}
	for _, tc := range cases {
		if got := classifyAuthProbe(tc.err); got != tc.want {
			t.Errorf("%s: classifyAuthProbe = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// scriptedConnector fails with errs in turn, then with then for good (nil
// means success).
type scriptedConnector struct {
	errs  []error
	then  error
	calls int
}

func (c *scriptedConnector) connect(context.Context) (*mongo.Client, func(), error) {
	c.calls++
	if c.calls <= len(c.errs) {
		return nil, func() {}, c.errs[c.calls-1]
	}
	return nil, func() {}, c.then
}

type scriptedProbe struct {
	err   error
	calls int
}

func (p *scriptedProbe) probe(context.Context) error {
	p.calls++
	return p.err
}

func waitForShardClientWith(ctx context.Context, c *scriptedConnector, p *scriptedProbe) (bool, error) {
	_, _, noAuth, err := WaitForShardClient(ctx, "rs2-1:27017", "admin", c.connect, p.probe, testPoll)
	return noAuth, err
}

// READY-T04: INIT-034 — connection errors are retried without probing.
func TestWaitForShardClient_RetriesRefusedConnections(t *testing.T) {
	c := &scriptedConnector{errs: []error{errNotListening, errNotListening}}
	p := &scriptedProbe{}
	noAuth, err := waitForShardClientWith(context.Background(), c, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if noAuth {
		t.Error("noAuth must be false when the connection succeeded")
	}
	if c.calls != 3 {
		t.Errorf("want 3 attempts, got %d", c.calls)
	}
	if p.calls != 0 {
		t.Errorf("the probe must not run for a connection error, ran %d times", p.calls)
	}
}

// READY-T05: INIT-035 — a refused login on an auth host waits for the user.
func TestWaitForShardClient_WaitsForUserOnAuthHost(t *testing.T) {
	c := &scriptedConnector{errs: []error{errRefusedLogin, errRefusedLogin}}
	p := &scriptedProbe{err: errUnauthorizedProbe}
	noAuth, err := waitForShardClientWith(context.Background(), c, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if noAuth {
		t.Error("noAuth must be false once the user logs in")
	}
	if c.calls != 3 || p.calls != 2 {
		t.Errorf("want 3 attempts and 2 probes, got %d and %d", c.calls, p.calls)
	}
}

// READY-T06: INIT-029, INIT-035 — a refused login on a no-auth host enters
// the init path at once.
func TestWaitForShardClient_NoAuthHostEntersInit(t *testing.T) {
	for _, probeErr := range []error{nil, fmt.Errorf("replSetGetStatus: %w", mongo.CommandError{Code: MongoErrNotYetInitialized})} {
		c := &scriptedConnector{errs: []error{errRefusedLogin, errRefusedLogin}}
		p := &scriptedProbe{err: probeErr}
		noAuth, err := waitForShardClientWith(context.Background(), c, p)
		if err != nil {
			t.Fatalf("probe %v: unexpected error: %v", probeErr, err)
		}
		if !noAuth {
			t.Errorf("probe %v: want noAuth", probeErr)
		}
		if c.calls != 1 || p.calls != 1 {
			t.Errorf("probe %v: want 1 attempt and 1 probe, got %d and %d", probeErr, c.calls, p.calls)
		}
	}
}

// READY-T07: INIT-035 — a probe that gets no answer keeps the wait going.
func TestWaitForShardClient_UnansweredProbeKeepsWaiting(t *testing.T) {
	c := &scriptedConnector{errs: []error{errRefusedLogin}}
	p := &scriptedProbe{err: errNotListening}
	noAuth, err := waitForShardClientWith(context.Background(), c, p)
	if err != nil || noAuth {
		t.Fatalf("want a plain success, got noAuth=%v err=%v", noAuth, err)
	}
	if c.calls != 2 || p.calls != 1 {
		t.Errorf("want 2 attempts and 1 probe, got %d and %d", c.calls, p.calls)
	}
}

// READY-T08: other errors are returned at once.
func TestWaitForShardClient_OtherErrorsReturnAtOnce(t *testing.T) {
	want := errors.New("Error resolving shard client: shard \"rs9\" not found; available shards: [rs2]")
	c := &scriptedConnector{then: want}
	p := &scriptedProbe{}
	_, err := waitForShardClientWith(context.Background(), c, p)
	if !errors.Is(err, want) {
		t.Fatalf("want the error as it is, got %v", err)
	}
	if c.calls != 1 || p.calls != 0 {
		t.Errorf("want 1 attempt and no probe, got %d and %d", c.calls, p.calls)
	}
}

// READY-T09: INIT-036 — the deadline error names host, user and last error.
func TestWaitForShardClient_DeadlineNamesUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	c := &scriptedConnector{then: errRefusedLogin}
	p := &scriptedProbe{err: errUnauthorizedProbe}
	_, err := waitForShardClientWith(ctx, c, p)
	if err == nil {
		t.Fatal("want an error at the deadline")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the error should carry the deadline: %v", err)
	}
	var notReady *notReadyError
	if !errors.As(err, &notReady) {
		t.Errorf("want a *notReadyError, got %T", err)
	}
	for _, want := range []string{`rs2-1:27017 did not accept the credentials of user "admin"`, "the operation timeout", "not been created yet", "AuthenticationFailed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should contain %q, got: %v", want, err)
		}
	}
	if c.calls < 2 {
		t.Errorf("the wait should have retried before the deadline, made %d attempts", c.calls)
	}
}

// READY-T10: INIT-036 — the deadline error names a silent host.
func TestWaitForShardClient_DeadlineNamesHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	c := &scriptedConnector{then: errNotListening}
	p := &scriptedProbe{}
	_, err := waitForShardClientWith(ctx, c, p)
	if err == nil {
		t.Fatal("want an error at the deadline")
	}
	for _, want := range []string{"rs2-1:27017 did not accept connections", "timeouts block", "connection refused"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should contain %q, got: %v", want, err)
		}
	}
	if p.calls != 0 {
		t.Errorf("the probe must not run for a connection error, ran %d times", p.calls)
	}
}

// READY-T11: an interrupted apply ends the wait with the cancellation.
func TestWaitForShardClient_Interrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(5*time.Millisecond, cancel)
	c := &scriptedConnector{then: errNotListening}
	_, err := waitForShardClientWith(ctx, c, &scriptedProbe{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want the cancellation as the cause, got %v", err)
	}
	if !strings.Contains(err.Error(), "interrupted") {
		t.Errorf("error should say the apply was interrupted, got: %v", err)
	}
}

// hostProbe fails each host with its errs in turn, then with then for good;
// otherwise it answers as a fresh mongod.
type hostProbe struct {
	errs  map[string][]error
	then  map[string]error
	calls map[string]int
}

func (p *hostProbe) probe(_ context.Context, host string) (*IsMasterResp, error) {
	if p.calls == nil {
		p.calls = map[string]int{}
	}
	p.calls[host]++
	if n := p.calls[host]; n <= len(p.errs[host]) {
		return nil, p.errs[host][n-1]
	}
	if err := p.then[host]; err != nil {
		return nil, err
	}
	return &IsMasterResp{IsReplicaSet: true}, nil
}

// READY-T12: SHARD-029 — each host is waited for in block order.
func TestWaitForAddTargets_WaitsForEachHost(t *testing.T) {
	p := &hostProbe{errs: map[string][]error{
		"rs2-2:27017": {errNotListening, errNotListening},
		"rs2-3:27017": {errNotListening},
	}}
	overrides := []MemberOverride{{Host: "rs2-2:27017"}, {Host: "rs2-3:27017"}}
	if err := WaitForAddTargets(context.Background(), overrides, p.probe, testPoll); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.calls["rs2-2:27017"] != 3 || p.calls["rs2-3:27017"] != 2 {
		t.Errorf("want 3 and 2 probes, got %v", p.calls)
	}
}

// READY-T13: SHARD-029 — other probe errors are left to the pre-flight.
func TestWaitForAddTargets_LeavesOtherErrorsToPreflight(t *testing.T) {
	p := &hostProbe{then: map[string]error{
		"rs2-2:27017": errMemberProbeSkipped,
		"rs2-3:27017": errors.New("invalid member host \"rs2-3:\": empty port"),
	}}
	overrides := []MemberOverride{{Host: "rs2-2:27017"}, {Host: "rs2-3:27017"}}
	if err := WaitForAddTargets(context.Background(), overrides, p.probe, testPoll); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.calls["rs2-2:27017"] != 1 || p.calls["rs2-3:27017"] != 1 {
		t.Errorf("want one probe per host, got %v", p.calls)
	}
}

// READY-T14: SHARD-029 — the deadline error names the silent host.
func TestWaitForAddTargets_DeadlineNamesHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	p := &hostProbe{then: map[string]error{"rs2-3:27017": errNotListening}}
	overrides := []MemberOverride{{Host: "rs2-2:27017"}, {Host: "rs2-3:27017"}, {Host: "rs2-4:27017"}}
	err := WaitForAddTargets(ctx, overrides, p.probe, testPoll)
	if err == nil {
		t.Fatal("want an error at the deadline")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the error should carry the deadline: %v", err)
	}
	for _, want := range []string{"replica set member rs2-3:27017 did not accept connections", "connection refused"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should contain %q, got: %v", want, err)
		}
	}
	if p.calls["rs2-2:27017"] != 1 || p.calls["rs2-4:27017"] != 0 {
		t.Errorf("want rs2-2 probed once and rs2-4 not at all, got %v", p.calls)
	}
}

// READY-T15: SHARD-029 — nothing to add, nothing to wait for.
func TestWaitForAddTargets_NoHosts(t *testing.T) {
	p := &hostProbe{}
	if err := WaitForAddTargets(context.Background(), nil, p.probe, testPoll); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.calls) != 0 {
		t.Errorf("no host should be probed, got %v", p.calls)
	}
}

// READY-T16: INIT-033 — timeouts { create, update } with the default.
func TestShardConfigSchema_Timeouts(t *testing.T) {
	res := resourceShardConfig()
	if res.Timeouts == nil {
		t.Fatal("Timeouts must be declared so a timeouts block is accepted")
	}
	for name, got := range map[string]*time.Duration{"create": res.Timeouts.Create, "update": res.Timeouts.Update} {
		if got == nil || *got != DefaultShardConfigTimeout {
			t.Errorf("%s timeout: want default %s, got %v", name, DefaultShardConfigTimeout, got)
		}
	}
	if res.Timeouts.Read != nil || res.Timeouts.Delete != nil || res.Timeouts.Default != nil {
		t.Error("only create and update are declared")
	}

	var configured schema.ResourceTimeout
	cfg := terraform.NewResourceConfigRaw(map[string]interface{}{
		"timeouts": map[string]interface{}{"create": "45m"},
	})
	if err := configured.ConfigDecode(res, cfg); err != nil {
		t.Fatalf("ConfigDecode: %v", err)
	}
	if configured.Create == nil || *configured.Create != 45*time.Minute {
		t.Errorf("configured create timeout: want 45m, got %v", configured.Create)
	}
	if configured.Update == nil || *configured.Update != DefaultShardConfigTimeout {
		t.Errorf("update timeout should keep its default, got %v", configured.Update)
	}
}
