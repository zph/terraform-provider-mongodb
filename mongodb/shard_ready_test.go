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

// deadlineCtx expires after d; the waits under test must stop on their own
// before then.
func deadlineCtx(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

// READY-T01: INIT-034 — IsConnectionError.
// READY-T02: INIT-034 — a real refused dial is a connection error.
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
		t.Run(tc.name, func(t *testing.T) {
			if got := IsConnectionError(tc.err); got != tc.want {
				t.Errorf("IsConnectionError = %v, want %v (%v)", got, tc.want, tc.err)
			}
		})
	}

	t.Run("refused dial", func(t *testing.T) { // READY-T02
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := fmt.Sprint(l.Addr().(*net.TCPAddr).Port)
		_ = l.Close()

		_, err = MongoClientInit(deadlineCtx(t, 300*time.Millisecond), &MongoDatabaseConfiguration{
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
	})
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
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyAuthProbe(tc.err); got != tc.want {
				t.Errorf("classifyAuthProbe = %d, want %d", got, tc.want)
			}
		})
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

// WaitForShardClient against a scripted connector and auth probe.
func TestWaitForShardClient(t *testing.T) {
	type check = func(t *testing.T, c *scriptedConnector, p *scriptedProbe, noAuth bool, err error)
	// success is the shape of every wait that ends in a client: no error, the
	// expected noAuth verdict, and exact attempt and probe counts.
	success := func(wantNoAuth bool, connects, probes int) check {
		return func(t *testing.T, c *scriptedConnector, p *scriptedProbe, noAuth bool, err error) {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if noAuth != wantNoAuth {
				t.Errorf("noAuth: want %v, got %v", wantNoAuth, noAuth)
			}
			if c.calls != connects || p.calls != probes {
				t.Errorf("want %d attempts and %d probes, got %d and %d", connects, probes, c.calls, p.calls)
			}
		}
	}
	errNotYetInitialized := fmt.Errorf("replSetGetStatus: %w", mongo.CommandError{Code: MongoErrNotYetInitialized})
	errOther := errors.New("Error resolving shard client: shard \"rs9\" not found; available shards: [rs2]")
	shortDeadline := func(t *testing.T) context.Context { return deadlineCtx(t, 30*time.Millisecond) }

	cases := []struct {
		name      string
		ctx       func(t *testing.T) context.Context
		connector *scriptedConnector
		probe     *scriptedProbe
		check     check
	}{
		{name: "retries refused connections without probing", // READY-T04: INIT-034
			connector: &scriptedConnector{errs: []error{errNotListening, errNotListening}},
			probe:     &scriptedProbe{},
			check:     success(false, 3, 0)},
		{name: "refused login on an auth host waits for the user", // READY-T05: INIT-035
			connector: &scriptedConnector{errs: []error{errRefusedLogin, errRefusedLogin}},
			probe:     &scriptedProbe{err: errUnauthorizedProbe},
			check:     success(false, 3, 2)},
		{name: "refused login on a no-auth host enters init at once", // READY-T06: INIT-029, INIT-035
			connector: &scriptedConnector{errs: []error{errRefusedLogin, errRefusedLogin}},
			probe:     &scriptedProbe{},
			check:     success(true, 1, 1)},
		{name: "refused login on an uninitialized no-auth host enters init at once", // READY-T06: INIT-029, INIT-035
			connector: &scriptedConnector{errs: []error{errRefusedLogin, errRefusedLogin}},
			probe:     &scriptedProbe{err: errNotYetInitialized},
			check:     success(true, 1, 1)},
		{name: "unanswered probe keeps waiting", // READY-T07: INIT-035
			connector: &scriptedConnector{errs: []error{errRefusedLogin}},
			probe:     &scriptedProbe{err: errNotListening},
			check:     success(false, 2, 1)},
		{name: "other errors return at once", // READY-T08
			connector: &scriptedConnector{then: errOther},
			probe:     &scriptedProbe{},
			check: func(t *testing.T, c *scriptedConnector, p *scriptedProbe, _ bool, err error) {
				if !errors.Is(err, errOther) {
					t.Fatalf("want the error as it is, got %v", err)
				}
				if c.calls != 1 || p.calls != 0 {
					t.Errorf("want 1 attempt and no probe, got %d and %d", c.calls, p.calls)
				}
			}},
		{name: "deadline names host, user and last error", // READY-T09: INIT-036
			ctx:       shortDeadline,
			connector: &scriptedConnector{then: errRefusedLogin},
			probe:     &scriptedProbe{err: errUnauthorizedProbe},
			check: func(t *testing.T, c *scriptedConnector, _ *scriptedProbe, _ bool, err error) {
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
				assertErrorContains(t, err, `rs2-1:27017 did not accept the credentials of user "admin"`, "the operation timeout", "not been created yet", "AuthenticationFailed")
				if c.calls < 2 {
					t.Errorf("the wait should have retried before the deadline, made %d attempts", c.calls)
				}
			}},
		{name: "deadline names a silent host", // READY-T10: INIT-036
			ctx:       shortDeadline,
			connector: &scriptedConnector{then: errNotListening},
			probe:     &scriptedProbe{},
			check: func(t *testing.T, _ *scriptedConnector, p *scriptedProbe, _ bool, err error) {
				if err == nil {
					t.Fatal("want an error at the deadline")
				}
				assertErrorContains(t, err, "rs2-1:27017 did not accept connections", "timeouts block", "connection refused")
				if p.calls != 0 {
					t.Errorf("the probe must not run for a connection error, ran %d times", p.calls)
				}
			}},
		{name: "interrupted apply ends the wait with the cancellation", // READY-T11
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				time.AfterFunc(5*time.Millisecond, cancel)
				return ctx
			},
			connector: &scriptedConnector{then: errNotListening},
			probe:     &scriptedProbe{},
			check: func(t *testing.T, _ *scriptedConnector, _ *scriptedProbe, _ bool, err error) {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("want the cancellation as the cause, got %v", err)
				}
				if !strings.Contains(err.Error(), "interrupted") {
					t.Errorf("error should say the apply was interrupted, got: %v", err)
				}
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.ctx != nil {
				ctx = tc.ctx(t)
			}
			_, _, noAuth, err := WaitForShardClient(ctx, "rs2-1:27017", "admin", tc.connector.connect, tc.probe.probe, testPoll)
			tc.check(t, tc.connector, tc.probe, noAuth, err)
		})
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

// WaitForAddTargets against a scripted per-host probe.
func TestWaitForAddTargets(t *testing.T) {
	type check = func(t *testing.T, p *hostProbe, err error)
	// probed is the shape of a wait that returns: no error and exactly these
	// probes per host.
	probed := func(want map[string]int) check {
		return func(t *testing.T, p *hostProbe, err error) {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(p.calls) != len(want) {
				t.Fatalf("want probes %v, got %v", want, p.calls)
			}
			for host, n := range want {
				if p.calls[host] != n {
					t.Errorf("%s: want %d probes, got %v", host, n, p.calls)
				}
			}
		}
	}
	twoHosts := []MemberOverride{{Host: "rs2-2:27017"}, {Host: "rs2-3:27017"}}

	cases := []struct {
		name      string
		ctx       func(t *testing.T) context.Context
		probe     *hostProbe
		overrides []MemberOverride
		check     check
	}{
		{name: "waits for each host in block order", // READY-T12: SHARD-029
			probe: &hostProbe{errs: map[string][]error{
				"rs2-2:27017": {errNotListening, errNotListening},
				"rs2-3:27017": {errNotListening},
			}},
			overrides: twoHosts,
			check:     probed(map[string]int{"rs2-2:27017": 3, "rs2-3:27017": 2})},
		{name: "leaves other probe errors to the pre-flight", // READY-T13: SHARD-029
			probe: &hostProbe{then: map[string]error{
				"rs2-2:27017": errMemberProbeSkipped,
				"rs2-3:27017": errors.New("invalid member host \"rs2-3:\": empty port"),
			}},
			overrides: twoHosts,
			check:     probed(map[string]int{"rs2-2:27017": 1, "rs2-3:27017": 1})},
		{name: "deadline names the silent host", // READY-T14: SHARD-029
			ctx:       func(t *testing.T) context.Context { return deadlineCtx(t, 30*time.Millisecond) },
			probe:     &hostProbe{then: map[string]error{"rs2-3:27017": errNotListening}},
			overrides: []MemberOverride{{Host: "rs2-2:27017"}, {Host: "rs2-3:27017"}, {Host: "rs2-4:27017"}},
			check: func(t *testing.T, p *hostProbe, err error) {
				if err == nil {
					t.Fatal("want an error at the deadline")
				}
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("the error should carry the deadline: %v", err)
				}
				assertErrorContains(t, err, "replica set member rs2-3:27017 did not accept connections", "connection refused")
				if p.calls["rs2-2:27017"] != 1 || p.calls["rs2-4:27017"] != 0 {
					t.Errorf("want rs2-2 probed once and rs2-4 not at all, got %v", p.calls)
				}
			}},
		{name: "nothing to add, nothing to wait for", // READY-T15: SHARD-029
			probe: &hostProbe{},
			check: probed(nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.ctx != nil {
				ctx = tc.ctx(t)
			}
			tc.check(t, tc.probe, WaitForAddTargets(ctx, tc.overrides, tc.probe.probe, testPoll))
		})
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
