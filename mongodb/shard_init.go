package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	// MongoErrNotYetInitialized is MongoDB error code 94, returned when
	// replSetGetConfig is called on an uninitialized replica set. // INIT-001
	MongoErrNotYetInitialized = 94

	// MongoErrAlreadyInitialized is MongoDB error code 23, returned when
	// replSetInitiate is called on an already-initialized RS. // INIT-015
	MongoErrAlreadyInitialized = 23

	// MongoErrNotWriteReady is MongoDB error code 17405 (Location17405),
	// returned when replSetReconfig is called on a freshly elected PRIMARY
	// that cannot yet accept writes. // INIT-025
	MongoErrNotWriteReady = 17405

	// MongoErrVersionConflict is MongoDB error code 103
	// (NewReplicaSetConfigurationIncompatible), returned when replSetReconfig
	// sends a config version not greater than the current one. // INIT-027
	MongoErrVersionConflict = 103

	// MongoErrUnauthorized is MongoDB error code 13, returned when an
	// operation requires authentication but the client is not authorized. // INIT-029
	MongoErrUnauthorized = 13

	// MongoErrAuthenticationFailed is MongoDB error code 18, returned when
	// SCRAM-SHA authentication fails (wrong credentials or user missing). // INIT-029
	MongoErrAuthenticationFailed = 18

	// MongoErrFailedReadPreference is MongoDB error code 133
	// (FailedToSatisfyReadPreference), returned when addShard cannot find a
	// host matching the read preference (e.g. PRIMARY not yet discovered). // CLUS-011
	MongoErrFailedReadPreference = 133

	// DefaultInitTimeoutSecs is the default timeout for RS initialization. // INIT-020
	DefaultInitTimeoutSecs = 60

	// initPollInterval is the polling interval for WaitForPrimary and
	// WaitForMajorityHealthy.
	initPollInterval = 500 * time.Millisecond
)

// IsNotYetInitialized returns true if err wraps a mongo.CommandError with
// code 94 (NotYetInitialized). // INIT-001
func IsNotYetInitialized(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code == MongoErrNotYetInitialized
	}
	return false
}

// IsAlreadyInitialized returns true if err wraps a mongo.CommandError with
// code 23 (AlreadyInitialized). // INIT-015
func IsAlreadyInitialized(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code == MongoErrAlreadyInitialized
	}
	return false
}

// IsNotWriteReady returns true if err wraps a mongo.CommandError with
// code 17405 (Location17405). This occurs when a freshly elected PRIMARY
// has not yet written its initial no-op oplog entry. // INIT-025
func IsNotWriteReady(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code == MongoErrNotWriteReady
	}
	return false
}

// IsAuthError returns true if err indicates an authentication or authorization
// failure. Checks for mongo.CommandError codes 13 (Unauthorized) and 18
// (AuthenticationFailed), as well as connection-level SCRAM handshake failures
// which are not wrapped as CommandErrors. // INIT-029
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code == MongoErrUnauthorized || cmdErr.Code == MongoErrAuthenticationFailed
	}
	msg := err.Error()
	return strings.Contains(msg, "AuthenticationFailed") || strings.Contains(msg, "auth error")
}

// IsReadPreferenceError returns true if err indicates a FailedToSatisfyReadPreference
// error (code 133). This occurs when mongos cannot find a PRIMARY for a newly
// added replica set that hasn't been fully discovered yet. // CLUS-011
func IsReadPreferenceError(err error) bool {
	if err == nil {
		return false
	}
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code == MongoErrFailedReadPreference
	}
	return strings.Contains(err.Error(), "FailedToSatisfyReadPreference")
}

// diagContainsAuthError returns true if any diagnostic in the slice contains
// an authentication or authorization error message. // INIT-029
func diagContainsAuthError(diags diag.Diagnostics) bool {
	for _, d := range diags {
		if IsAuthError(fmt.Errorf("%s %s", d.Summary, d.Detail)) {
			return true
		}
	}
	return false
}

// IsVersionConflict returns true if err wraps a mongo.CommandError with
// code 103 (NewReplicaSetConfigurationIncompatible). This occurs when the
// config version sent is not greater than the server's current version,
// typically due to an internal auto-reconfig between read and write. // INIT-027
func IsVersionConflict(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code == MongoErrVersionConflict
	}
	return false
}

// BuildInitialMembers converts MemberOverride slices into ConfigMembers with
// sequential _id values starting from 0. // INIT-005
func BuildInitialMembers(overrides []MemberOverride) ConfigMembers {
	if len(overrides) == 0 {
		return ConfigMembers{}
	}
	members := make(ConfigMembers, len(overrides))
	for i, o := range overrides {
		members[i] = ConfigMember{
			ID:           i,
			Host:         o.Host,
			Priority:     o.Priority,
			Votes:        intPtr(o.Votes),
			Hidden:       boolPtr(o.Hidden),
			ArbiterOnly:  boolPtr(o.ArbiterOnly),
			BuildIndexes: boolPtr(o.BuildIndexes),
		}
		if o.Tags != nil {
			members[i].Tags = ReplsetTags(o.Tags)
		}
	}
	return members
}

// InitiateReplicaSet runs replSetInitiate with a single-member config on the
// given client. The config has _id=rsName, version=1, and a single member
// with _id=0 at firstHost. // INIT-007
func InitiateReplicaSet(ctx context.Context, client *mongo.Client, rsName, firstHost string) error {
	config := bson.D{
		{Key: "_id", Value: rsName},
		{Key: "version", Value: 1},
		{Key: "members", Value: bson.A{
			bson.D{
				{Key: "_id", Value: 0},
				{Key: "host", Value: firstHost},
			},
		}},
	}

	tflog.Info(ctx, "initiating replica set", map[string]interface{}{
		"rs_name":    rsName,
		"first_host": firstHost,
	})

	res := client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "replSetInitiate", Value: config},
	})
	if res.Err() != nil {
		return fmt.Errorf("replSetInitiate: %w", res.Err())
	}

	var resp OKResponse
	if err := res.Decode(&resp); err != nil {
		return fmt.Errorf("replSetInitiate decode: %w", err)
	}
	if resp.OK != 1 {
		return fmt.Errorf("replSetInitiate failed: %s", resp.Errmsg)
	}
	return nil
}

// WaitForPrimary polls replSetGetStatus until myState equals PRIMARY or the
// timeout is reached. // INIT-008, INIT-009
func WaitForPrimary(ctx context.Context, client *mongo.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("replica set did not elect a primary within %s", timeout)
		}

		status, err := GetReplSetStatus(ctx, client)
		if err == nil && status.MyState == MemberStatePrimary {
			tflog.Info(ctx, "replica set member reached PRIMARY state")
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(initPollInterval):
		}
	}
}

// WaitForMajorityHealthy polls replSetGetStatus until a majority of the
// expected member count report a healthy state (PRIMARY or SECONDARY).
// // INIT-013, INIT-014
func WaitForMajorityHealthy(ctx context.Context, client *mongo.Client, expectedCount int, timeout time.Duration) error {
	majority := (expectedCount / 2) + 1
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("majority of members (%d/%d) did not reach healthy state within %s",
				majority, expectedCount, timeout)
		}

		status, err := GetReplSetStatus(ctx, client)
		if err == nil {
			healthy := 0
			for _, m := range status.Members {
				if m.Health == MemberHealthUp &&
					(m.State == MemberStatePrimary || m.State == MemberStateSecondary) {
					healthy++
				}
			}
			if healthy >= majority {
				tflog.Info(ctx, "majority of members healthy", map[string]interface{}{
					"healthy":  healthy,
					"expected": expectedCount,
				})
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(initPollInterval):
		}
	}
}

// SetReplSetConfigWithRetry wraps SetReplSetConfig with retry logic for
// transient post-election errors:
//   - Location17405: freshly elected PRIMARY cannot accept writes yet (INIT-025)
//   - Code 103: config version conflict from internal auto-reconfig (INIT-027)
//
// On version conflicts, the function re-reads the current config version from
// the server before retrying. Retries use initPollInterval backoff until the
// timeout is exceeded. // INIT-025, INIT-026, INIT-027, INIT-028
func SetReplSetConfigWithRetry(ctx context.Context, client *mongo.Client, cfg *RSConfig, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := SetReplSetConfig(ctx, client, cfg)
		if err == nil {
			return nil
		}
		retryable := IsNotWriteReady(err) || IsVersionConflict(err)
		if !retryable {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("replSetReconfig: transient error not resolved within %s: %w", timeout, err)
		}
		// INIT-027/028: Re-read current version to handle auto-reconfig drift
		if IsVersionConflict(err) {
			current, readErr := GetReplSetConfig(ctx, client)
			if readErr != nil {
				return fmt.Errorf("replSetReconfig version conflict and failed to re-read config: %w", readErr)
			}
			cfg.Version = current.Version + 1
		}
		tflog.Debug(ctx, "retrying replSetReconfig after transient error", map[string]interface{}{
			"error":   err.Error(),
			"version": cfg.Version,
		})
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(initPollInterval):
		}
	}
}

// ConnectForInit establishes a direct connection to a single MongoDB member
// for replica set initialization. It first tries with authentication, then
// falls back to no-auth for fresh instances. // INIT-017, INIT-018, INIT-022
func ConnectForInit(ctx context.Context, providerCfg *ClientConfig, host, port string, maxConnLifetime time.Duration) (*mongo.Client, func(), error) {
	noop := func() {}

	// INIT-022: Direct mode for uninitialized members
	cfg := &ClientConfig{
		Host:               host,
		Port:               port,
		Username:           providerCfg.Username,
		Password:           providerCfg.Password,
		DB:                 providerCfg.DB,
		Ssl:                providerCfg.Ssl,
		InsecureSkipVerify: providerCfg.InsecureSkipVerify,
		Certificate:        providerCfg.Certificate,
		ReplicaSet:         "",
		RetryWrites:        providerCfg.RetryWrites,
		Direct:             true,
		Proxy:              providerCfg.Proxy,
	}

	conf := &MongoDatabaseConfiguration{
		Config:          cfg,
		MaxConnLifetime: maxConnLifetime,
	}

	// INIT-017: Try with authentication first
	client, err := MongoClientInit(ctx, conf)
	if err == nil {
		cleanup := func() { _ = client.Disconnect(ctx) }
		tflog.Debug(ctx, "connected to init target with auth", map[string]interface{}{
			"host": host, "port": port,
		})
		return client, cleanup, nil
	}

	tflog.Debug(ctx, "auth connection failed, trying no-auth fallback", map[string]interface{}{
		"host": host, "port": port, "error": err.Error(),
	})

	// INIT-018: Fallback to no-auth for fresh instances
	client, err = MongoClientInitNoAuth(ctx, conf)
	if err != nil {
		return nil, noop, fmt.Errorf("failed to connect to %s:%s (auth and no-auth both failed): %w", host, port, err)
	}

	cleanup := func() { _ = client.Disconnect(ctx) }
	tflog.Debug(ctx, "connected to init target without auth", map[string]interface{}{
		"host": host, "port": port,
	})
	return client, cleanup, nil
}
