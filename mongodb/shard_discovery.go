package mongodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// ConnectionType classifies the MongoDB topology the provider is connected to.
type ConnectionType int

const (
	ConnTypeMongos     ConnectionType = iota // mongos router
	ConnTypeReplicaSet                       // direct replica set member
	ConnTypeStandalone                       // standalone mongod
)

// String returns a human-readable label for the connection type.
func (ct ConnectionType) String() string {
	switch ct {
	case ConnTypeMongos:
		return "mongos"
	case ConnTypeReplicaSet:
		return "replica_set"
	case ConnTypeStandalone:
		return "standalone"
	default:
		return "unknown"
	}
}

// classifyConnectionType determines the ConnectionType from an isMaster response
// without requiring a live server connection. // DISC-001, DISC-006
func classifyConnectionType(resp *IsMasterResp) ConnectionType {
	if resp.Msg == "isdbgrid" {
		return ConnTypeMongos
	}
	if resp.SetName != "" {
		return ConnTypeReplicaSet
	}
	return ConnTypeStandalone
}

// DetectConnectionType runs isMaster against the client and classifies the
// connection topology. // DISC-001, DISC-006
func DetectConnectionType(ctx context.Context, client *mongo.Client) (ConnectionType, error) {
	var resp IsMasterResp
	res := client.Database("admin").RunCommand(ctx, bson.D{{Key: "isMaster", Value: 1}})
	if res.Err() != nil {
		return ConnTypeStandalone, fmt.Errorf("isMaster failed: %w", res.Err())
	}
	if err := res.Decode(&resp); err != nil {
		return ConnTypeStandalone, fmt.Errorf("isMaster decode failed: %w", err)
	}
	return classifyConnectionType(&resp), nil
}

// ListShards runs the listShards admin command against a mongos and decodes
// the result into the existing ShardList type. // DISC-001
func ListShards(ctx context.Context, client *mongo.Client) (*ShardList, error) {
	var shards ShardList
	res := client.Database("admin").RunCommand(ctx, bson.D{{Key: "listShards", Value: 1}})
	if res.Err() != nil {
		return nil, fmt.Errorf("listShards failed: %w", res.Err())
	}
	if err := res.Decode(&shards); err != nil {
		return nil, fmt.Errorf("listShards decode failed: %w", err)
	}
	if shards.OK != 1 {
		return nil, fmt.Errorf("listShards error: %s", shards.Errmsg)
	}
	return &shards, nil
}

// ParseShardHost parses the host field from listShards output.
// Format: "rsName/host1:port,host2:port,host3:port"
// Returns the replica set name and slice of host:port strings. // DISC-002
func ParseShardHost(hostStr string) (string, []string, error) {
	slashIdx := strings.Index(hostStr, "/")
	if slashIdx < 0 {
		return "", nil, fmt.Errorf("invalid shard host format (missing '/'): %q", hostStr)
	}
	rsName := hostStr[:slashIdx]
	if rsName == "" {
		return "", nil, fmt.Errorf("empty replica set name in shard host: %q", hostStr)
	}
	hostsStr := hostStr[slashIdx+1:]
	if hostsStr == "" {
		return "", nil, fmt.Errorf("empty host list in shard host: %q", hostStr)
	}
	hosts := strings.Split(hostsStr, ",")
	return rsName, hosts, nil
}

// FindShardByName looks up a shard by _id in the listShards response.
// Returns the host string or an error listing available shard names. // DISC-002, DISC-005
func FindShardByName(shards *ShardList, shardName string) (string, error) {
	available := make([]string, 0, len(shards.Shards))
	for _, s := range shards.Shards {
		if s.ID == shardName {
			return s.Host, nil
		}
		available = append(available, s.ID)
	}
	return "", fmt.Errorf("shard %q not found; available shards: %v", shardName, available)
}

// SplitHostPort splits a "host:port" string. If no port is present, defaults
// to "27017". Returns an error for empty input or empty host/port components.
func SplitHostPort(hostPort string) (string, string, error) {
	if hostPort == "" {
		return "", "", fmt.Errorf("empty host:port string")
	}
	colonIdx := strings.LastIndex(hostPort, ":")
	if colonIdx < 0 {
		// No colon — treat entire string as host, default port
		return hostPort, "27017", nil
	}
	host := hostPort[:colonIdx]
	port := hostPort[colonIdx+1:]
	if host == "" {
		return "", "", fmt.Errorf("empty host in %q", hostPort)
	}
	if port == "" {
		return "", "", fmt.Errorf("empty port in %q", hostPort)
	}
	return host, port, nil
}

// BuildShardClientConfig creates a new ClientConfig for a direct connection
// to a shard member, inheriting credentials, TLS, and proxy from the
// provider's config. // DISC-003, DISC-010
func BuildShardClientConfig(providerCfg *ClientConfig, host, port, rsName string) *ClientConfig {
	return &ClientConfig{
		Host:               host,
		Port:               port,
		Username:           providerCfg.Username,
		Password:           providerCfg.Password,
		DB:                 providerCfg.DB,
		Ssl:                providerCfg.Ssl,
		InsecureSkipVerify: providerCfg.InsecureSkipVerify,
		Certificate:        providerCfg.Certificate,
		ReplicaSet:         rsName,
		RetryWrites:        providerCfg.RetryWrites,
		Direct:             true,
		Proxy:              providerCfg.Proxy,
	}
}

// ResolveShardClient orchestrates shard discovery. If the provider is
// connected to a mongos, it runs listShards, finds the target shard, creates
// a temporary direct connection, and returns (client, cleanup). If the
// provider is already on a RS member, it returns the provider client with a
// no-op cleanup. // DISC-001 through DISC-010
func ResolveShardClient(
	ctx context.Context,
	providerClient *mongo.Client,
	providerCfg *ClientConfig,
	shardName string,
	hostOverride string,
	maxConnLifetime time.Duration,
) (*mongo.Client, func(), error) {
	noop := func() {}

	connType, err := DetectConnectionType(ctx, providerClient)
	if err != nil {
		return nil, noop, fmt.Errorf("failed to detect connection type: %w", err)
	}

	tflog.Debug(ctx, "detected connection type", map[string]interface{}{
		"connection_type": connType.String(),
		"shard_name":      shardName,
	})

	// DISC-006: Direct RS or standalone — use provider client as-is
	if connType != ConnTypeMongos {
		return providerClient, noop, nil
	}

	// DISC-001: Connected to mongos — discover shards
	shards, err := ListShards(ctx, providerClient)
	if err != nil {
		return nil, noop, err
	}

	// DISC-002/005: Find the target shard
	shardHost, err := FindShardByName(shards, shardName)
	if err != nil {
		return nil, noop, err
	}

	rsName, hosts, err := ParseShardHost(shardHost)
	if err != nil {
		return nil, noop, err
	}

	// DISC-008: Use host_override if set, otherwise first discovered host
	targetHostPort := hosts[0]
	if hostOverride != "" {
		targetHostPort = hostOverride
	}

	// DISC-009: Validate the host:port
	host, port, err := SplitHostPort(targetHostPort)
	if err != nil {
		return nil, noop, fmt.Errorf("invalid host_override %q: %w", targetHostPort, err)
	}

	tflog.Debug(ctx, "creating temporary shard connection", map[string]interface{}{
		"shard_name":    shardName,
		"rs_name":       rsName,
		"target_host":   host,
		"target_port":   port,
		"host_override": hostOverride != "",
	})

	// DISC-003/010: Build config inheriting provider creds/TLS/proxy
	shardCfg := BuildShardClientConfig(providerCfg, host, port, rsName)
	shardConf := &MongoDatabaseConfiguration{
		Config:          shardCfg,
		MaxConnLifetime: maxConnLifetime,
	}

	shardClient, err := MongoClientInit(ctx, shardConf)
	if err != nil {
		return nil, noop, fmt.Errorf("failed to connect to shard %q at %s:%s: %w", shardName, host, port, err)
	}

	// DISC-007: Cleanup disconnects the temporary client
	cleanup := func() {
		_ = shardClient.Disconnect(ctx)
	}

	return shardClient, cleanup, nil
}
