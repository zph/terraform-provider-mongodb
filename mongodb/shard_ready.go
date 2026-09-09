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
	// DefaultShardConfigTimeout is the default create and update timeout of
	// mongodb_shard_config: the SDK's own default, declared so that a
	// `timeouts` block is accepted. It bounds the whole operation, including
	// the readiness waits below. INIT-033
	DefaultShardConfigTimeout = 20 * time.Minute

	// readyPollInterval is the pause between attempts in the readiness waits.
	// An attempt against a host that is not answering already takes
	// MaxConnLifetime, so such a host is retried roughly every 20 seconds.
	// INIT-034, SHARD-029
	readyPollInterval = 10 * time.Second
)

// IsConnectionError reports whether err says a server could not be reached,
// as opposed to having answered. A refused dial, a name that does not
// resolve, or a handshake that never completes all surface from the driver as
// a server selection that ran into the connect deadline; a connection that
// dropped mid-command carries the NetworkError label. An answer the server
// gave, including a refused authentication or a command error, is not a
// connection error. INIT-034
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
// when the operation's deadline passed or the apply was interrupted.
// INIT-036, SHARD-029
type notReadyError struct {
	// Msg says what did not happen, such as `rs2-1:27017 did not accept
	// connections`.
	Msg string
	// Detail, when set, explains what the wait was for.
	Detail  string
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

// waitUntilReachable calls attempt until it succeeds or fails for a reason
// other than a failed connection, pausing poll between attempts, and returns
// that result. When ctx ends first it returns a *notReadyError naming what,
// with the last connection error. INIT-034, SHARD-029
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

// authRequirement is what an unauthenticated replSetGetStatus against the
// provider host says about why the provider's credentials were refused.
// INIT-035
type authRequirement int

const (
	// authRequired: the command was refused as Unauthorized. The server
	// enforces authentication, so either the user has not been created yet
	// or the credentials are wrong; SCRAM does not tell the two apart.
	authRequired authRequirement = iota
	// authNotEnforced: the server answered the command, or failed it for a
	// reason of its own such as NotYetInitialized, without asking for
	// credentials. It runs without access control and the provider user
	// simply does not exist, which is the fresh instance of INIT-029.
	authNotEnforced
	// authUnknown: the probe got no answer, for instance because the server
	// is restarting.
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

// authProbeFor returns a probe that connects directly to the provider host
// without credentials and runs replSetGetStatus, which every server with
// access control refuses as Unauthorized. INIT-035
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

// WaitForShardClient is the first step of Create. It retries connect, which
// connects as the provider user, until it succeeds, until the provider host
// turns out not to enforce authentication (noAuth true, and the caller runs
// the initialization path of INIT-029), or until ctx ends. A host that
// refuses connections or does not answer is retried. A host that refuses the
// credentials is probed without them: if the probe is refused as Unauthorized
// the host enforces authentication and its bootstrap has not created the
// user yet, so the wait goes on; if the probe is answered the host runs
// without access control and Create can initialize it at once. Any other
// error is returned as it is. When ctx ends, the error names target and, when
// the credentials were being refused, the user. INIT-034, INIT-035, INIT-036
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

// WaitForAddTargets waits until every host about to be added answers a
// connection, so a node that is still starting counts as not ready yet rather
// than unreachable when the pre-flight of SHARD-027 inspects it. Probe errors
// other than a failed connection are left for the pre-flight to judge. When
// ctx ends it returns a *notReadyError naming the host still unreachable.
// SHARD-029
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
