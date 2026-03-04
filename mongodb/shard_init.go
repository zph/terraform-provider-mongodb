package mongodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
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
