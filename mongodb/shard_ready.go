package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	// DefaultShardConfigTimeout is the default create and update timeout,
	// the SDK's own, declared so a timeouts block is accepted. INIT-033
	DefaultShardConfigTimeout = 20 * time.Minute

	// readyPollInterval is the pause between readiness attempts. An attempt
	// against a silent host already takes MaxConnLifetime. INIT-034, SHARD-029
	readyPollInterval = 10 * time.Second
)

// IsConnectionError reports whether err says a server could not be reached,
// as opposed to having answered: the driver surfaces a refused dial, an
// unresolvable name or a hung handshake as a server selection that hit the
// connect deadline. A refused login or a command error is an answer. INIT-034
func IsConnectionError(err error) bool {
	if err == nil || IsAuthError(err) {
		return false
	}
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.HasErrorLabel("NetworkError")
	}
	return mongo.IsTimeout(err) || mongo.IsNetworkError(err)
}

// notReadyError ends a readiness wait whose target was still not answering
// when ctx ended. INIT-036, SHARD-029
type notReadyError struct {
	Msg     string // what did not happen, e.g. `rs2-1:27017 did not accept connections`
	Detail  string // optional explanation
	Elapsed time.Duration
	Cause   error // the context's error
	Last    error // the last answer the target gave, if any
}

func (e *notReadyError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s within %s", e.Msg, e.Elapsed.Round(time.Second))
	if errors.Is(e.Cause, context.DeadlineExceeded) {
		b.WriteString(", the operation timeout (see the resource's timeouts block)")
	} else {
		b.WriteString(", when the apply was interrupted")
	}
	if e.Detail != "" {
		b.WriteString(". " + e.Detail)
	}
	if e.Last != nil {
		fmt.Fprintf(&b, ". Last error: %v", e.Last)
	}
	return b.String()
}

func (e *notReadyError) Unwrap() error { return e.Cause }

// waitUntilReachable retries attempt while it fails with a connection error,
// pausing poll between attempts. Other errors are returned as they are; when
// ctx ends first the result is a *notReadyError for what. INIT-034, SHARD-029
func waitUntilReachable(ctx context.Context, what string, poll time.Duration, attempt func(ctx context.Context) error) error {
	start := time.Now()
	var last error
	for {
		err := attempt(ctx)
		if err == nil {
			return nil
		}
		if ctx.Err() == nil {
			if !IsConnectionError(err) {
				return err
			}
			last = err
			tflog.Info(ctx, "host is not reachable yet; retrying", map[string]interface{}{
				"host": what, "elapsed": time.Since(start).Round(time.Second).String(), "error": err.Error(),
			})
		} else if last == nil {
			last = err
		}
		select {
		case <-ctx.Done():
			return &notReadyError{Msg: what + " did not accept connections", Elapsed: time.Since(start), Cause: ctx.Err(), Last: last}
		case <-time.After(poll):
		}
	}
}

// shardConnector opens the shard client the way getShardClient does.
type shardConnector func(ctx context.Context) (*mongo.Client, func(), error)

// authProbe runs replSetGetStatus against the provider host without
// credentials and returns the command's error, if any.
type authProbe func(ctx context.Context) error

// authRequirement is what an unauthenticated replSetGetStatus says about why
// the provider's credentials were refused. INIT-035
type authRequirement int

const (
	// authRequired: refused as Unauthorized. The host enforces authentication;
	// the user is not created yet or the password is wrong (SCRAM does not say).
	authRequired authRequirement = iota
	// authNotEnforced: answered, or failed for a reason of its own such as
	// NotYetInitialized. No access control; the user simply does not exist.
	authNotEnforced
	// authUnknown: no answer, e.g. the host is restarting.
	authUnknown
)

func classifyAuthProbe(err error) authRequirement {
	var cmdErr mongo.CommandError
	switch {
	case err == nil:
		return authNotEnforced
	case IsAuthError(err):
		return authRequired
	case errors.As(err, &cmdErr):
		return authNotEnforced
	default:
		return authUnknown
	}
}

// authProbeFor connects directly to the provider host without credentials and
// runs replSetGetStatus. INIT-035
func authProbeFor(providerConf *MongoDatabaseConfiguration) authProbe {
	return func(ctx context.Context) error {
		probeCtx, cancel := context.WithTimeout(ctx, memberPreflightTimeout)
		defer cancel()
		client, err := MongoClientInitNoAuth(probeCtx, &MongoDatabaseConfiguration{
			Config:          BuildShardClientConfig(providerConf.Config, providerConf.Config.Host, providerConf.Config.Port, ""),
			MaxConnLifetime: providerConf.MaxConnLifetime,
		})
		if err != nil {
			return err
		}
		defer func() { _ = client.Disconnect(probeCtx) }()
		_, err = GetReplSetStatus(probeCtx, client)
		return err
	}
}

// WaitForShardClient retries connect until it succeeds, until the host turns
// out not to enforce authentication (noAuth true, the caller runs the init
// path of INIT-029), or until ctx ends. A connection error is retried; a
// refused login is probed without credentials and retried while the probe is
// refused as Unauthorized. Other errors are returned as they are. The error at
// the deadline names target and, for a refused login, username.
// INIT-034, INIT-035, INIT-036
func WaitForShardClient(ctx context.Context, target, username string, connect shardConnector, probe authProbe, poll time.Duration) (*mongo.Client, func(), bool, error) {
	start := time.Now()
	notReady := &notReadyError{Msg: target + " did not accept connections"}
	for {
		client, cleanup, err := connect(ctx)
		if err == nil {
			return client, cleanup, false, nil
		}
		fields := map[string]interface{}{
			"host": target, "elapsed": time.Since(start).Round(time.Second).String(), "error": err.Error(),
		}
		if ctx.Err() == nil {
			switch {
			case IsAuthError(err):
				notReady.Msg = fmt.Sprintf("%s did not accept the credentials of user %q", target, username)
				switch classifyAuthProbe(probe(ctx)) {
				case authNotEnforced:
					tflog.Info(ctx, "provider user cannot log in and the host does not enforce authentication; initializing without credentials", fields)
					return nil, nil, true, nil
				case authRequired:
					notReady.Detail = "The server enforces authentication, so either the user has not been created yet or the password is wrong."
					tflog.Info(ctx, "host enforces authentication and refused the provider user; waiting for the user to be created", fields)
				default:
					notReady.Detail = "The server refused the credentials, and a probe without credentials got no answer."
					tflog.Info(ctx, "host refused the provider user and did not answer the probe; retrying", fields)
				}
			case IsConnectionError(err):
				notReady.Msg = target + " did not accept connections"
				notReady.Detail = ""
				tflog.Info(ctx, "host is not reachable yet; retrying", fields)
			default:
				return nil, nil, false, err
			}
			notReady.Last = err
		} else if notReady.Last == nil {
			notReady.Last = err
		}
		select {
		case <-ctx.Done():
			notReady.Elapsed = time.Since(start)
			notReady.Cause = ctx.Err()
			return nil, nil, false, notReady
		case <-time.After(poll):
		}
	}
}

// WaitForAddTargets waits for each host to be added to accept a connection
// before the pre-flight of SHARD-027 inspects it. Probe errors other than a
// connection error are left to the pre-flight. SHARD-029
func WaitForAddTargets(ctx context.Context, overrides []MemberOverride, probe memberProbe, poll time.Duration) error {
	for _, o := range overrides {
		err := waitUntilReachable(ctx, "replica set member "+o.Host, poll, func(ctx context.Context) error {
			_, err := probe(ctx, o.Host)
			return err
		})
		var notReady *notReadyError
		if errors.As(err, &notReady) {
			return err
		}
	}
	return nil
}
