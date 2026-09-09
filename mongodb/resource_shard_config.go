package mongodb

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"go.mongodb.org/mongo-driver/mongo"
)

type ResourceShardConfig struct {
}

// Create waits for the replica set to accept the provider connection, then
// detects whether it is initialized. The wait is bounded by the create
// timeout: a host that refuses connections, or that enforces authentication
// but has not created the provider user yet, is retried (INIT-034, INIT-035),
// so the resource can be applied in the same run as the instances it
// configures.
// INIT-001: If replSetGetConfig returns code 94, enter init flow.
// INIT-002: If replSetGetConfig returns a valid config, delegate to Update.
// INIT-015: If replSetGetConfig returns code 23, delegate to Update.
// INIT-029: If the provider user cannot log in and the host does not enforce
// authentication, enter init flow (the user does not exist yet on a fresh
// instance; ConnectForInit has a no-auth fallback).
func (r *ResourceShardConfig) Create(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	providerConf := i.(*MongoDatabaseConfiguration)
	target := providerConf.Config.Host + ":" + providerConf.Config.Port
	tflog.Info(ctx, "waiting for the replica set to accept the provider connection", map[string]interface{}{
		"host": target, "timeout": data.Timeout(schema.TimeoutCreate).String(),
	})
	connect := func(ctx context.Context) (*mongo.Client, func(), error) {
		return r.getShardClient(ctx, data, i)
	}
	client, cleanup, noAuth, err := WaitForShardClient(ctx, target, providerConf.Config.Username, connect, authProbeFor(providerConf), readyPollInterval)
	if err != nil {
		return diag.FromErr(err)
	}
	if noAuth {
		return r.initializeReplicaSet(ctx, data, i)
	}
	defer cleanup()

	_, err = GetReplSetConfig(ctx, client)
	switch {
	case err == nil:
		// INIT-002: Already configured, delegate to Update (client is authed)
		return r.updateWithClient(ctx, data, client, providerConf, false)
	case IsAlreadyInitialized(err):
		// INIT-015: Already initialized, delegate to Update (client is authed)
		return r.updateWithClient(ctx, data, client, providerConf, false)
	case IsNotYetInitialized(err):
		// INIT-001: Enter initialization flow
		return r.initializeReplicaSet(ctx, data, i)
	default:
		return diag.FromErr(err)
	}
}

// initializeReplicaSet runs replSetInitiate with the first member block,
// waits for PRIMARY, then hands over to updateWithClient for the remaining
// members and the settings. INIT-007, INIT-008, INIT-010
func (r *ResourceShardConfig) initializeReplicaSet(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	providerConf := i.(*MongoDatabaseConfiguration)

	overrides, ok := extractMemberOverrides(data)
	if !ok || len(overrides) == 0 {
		// INIT-003: member blocks required for initialization
		return diag.Errorf("member blocks are required for replica set initialization")
	}
	if errD := validateMemberOverrides(overrides); errD != nil {
		return errD
	}

	shardName := data.Get("shard_name").(string)
	timeout := time.Duration(data.Get("init_timeout_secs").(int)) * time.Second
	firstHost := overrides[0].Host

	host, port, err := SplitHostPort(firstHost)
	if err != nil {
		return diag.FromErr(fmt.Errorf("invalid first member host %q: %w", firstHost, err))
	}

	// INIT-006/017/018/022: Direct connect with auth fallback, retried while
	// the host is still starting (INIT-034).
	var initClient *mongo.Client
	initCleanup := func() {}
	err = waitUntilReachable(ctx, "replica set member "+firstHost, readyPollInterval, func(ctx context.Context) error {
		var connErr error
		initClient, initCleanup, connErr = ConnectForInit(ctx, providerConf.Config, host, port, providerConf.MaxConnLifetime)
		return connErr
	})
	if err != nil {
		return diag.FromErr(err)
	}
	defer initCleanup()

	// INIT-007: replSetInitiate with single member
	err = InitiateReplicaSet(ctx, initClient, shardName, firstHost)
	if err != nil {
		if IsAlreadyInitialized(err) {
			// INIT-015/030: Already initialized — try authenticated client
			// first (users may exist from a previous run), fall back to
			// initClient if auth fails.
			if authClient, authCleanup, authErr := r.getShardClient(ctx, data, i); authErr == nil {
				defer authCleanup()
				return r.updateWithClient(ctx, data, authClient, providerConf, false)
			}
			// initClient may be a no-auth connection (INIT-018): the replica
			// set can exist before any users do, so the per-member oplog
			// connections need the same fallback. OPLOG-017
			return r.updateWithClient(ctx, data, initClient, providerConf, true)
		}
		return diag.FromErr(err)
	}

	// INIT-008/009: Wait for PRIMARY
	if err := WaitForPrimary(ctx, initClient, timeout); err != nil {
		return diag.FromErr(err)
	}

	// INIT-010: from here the set is reconciled like any initialized set.
	// Users cannot exist yet, so the per-member oplog connections keep the
	// no-auth fallback (INIT-018, OPLOG-017).
	return r.updateWithClient(ctx, data, initClient, providerConf, true)
}

// CATCHUP-005
type SettingsModel struct {
	ChainingAllowed         bool  `tfsdk:"chaining_allowed,omitempty"`
	HeartbeatIntervalMillis int64 `tfsdk:"heartbeat_interval_millis,omitempty"`
	HeartbeatTimeoutSecs    int   `tfsdk:"heartbeat_timeout_secs,omitempty"`
	ElectionTimeoutMillis   int64 `tfsdk:"election_timeout_millis,omitempty"`
	CatchUpTimeoutMillis    int64 `tfsdk:"catch_up_timeout_millis,omitempty"`
}

type ShardModel struct {
	ID       string        `tfsdk:"id"`
	Settings SettingsModel `tfsdk:"settings"`
}

// MemberOverride represents Terraform-declared member configuration.
// SHARD-003: Members are identified by host (case-sensitive exact match).
type MemberOverride struct {
	Host         string
	Priority     float64
	Votes        int
	Hidden       bool
	ArbiterOnly  bool
	BuildIndexes bool
	Tags         map[string]string
}

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

// toFloat64 converts Terraform schema priority (TypeFloat → float64, or int from state) to float64.
func toFloat64(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch p := v.(type) {
	case float64:
		return p
	case int:
		return float64(p)
	case int64:
		return float64(p)
	default:
		return 0
	}
}
func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// MergeMembers applies Terraform member overrides onto RSConfig members,
// matching by host. Update partitions unknown hosts off as adds beforehand
// (SHARD-012), so the not-found error here is a guard rather than a
// user-facing path.
// SHARD-005: All fields from the override are applied.
// SHARD-006: Unlisted members are left unchanged.
func MergeMembers(rsMembers ConfigMembers, overrides []MemberOverride) (ConfigMembers, error) {
	if len(overrides) == 0 {
		return rsMembers, nil
	}

	hostIndex := make(map[string]int, len(rsMembers))
	for i, m := range rsMembers {
		hostIndex[m.Host] = i
	}

	for _, o := range overrides {
		idx, found := hostIndex[o.Host]
		if !found {
			return nil, fmt.Errorf("member host %q not found in replica set members: %v",
				o.Host, memberHosts(rsMembers))
		}

		rsMembers[idx].Priority = o.Priority
		rsMembers[idx].Votes = intPtr(o.Votes)
		rsMembers[idx].Hidden = boolPtr(o.Hidden)
		rsMembers[idx].ArbiterOnly = boolPtr(o.ArbiterOnly)
		rsMembers[idx].BuildIndexes = boolPtr(o.BuildIndexes)
		if o.Tags != nil {
			rsMembers[idx].Tags = ReplsetTags(o.Tags)
		} else {
			rsMembers[idx].Tags = nil
		}
	}

	return rsMembers, nil
}

func memberHosts(members ConfigMembers) []string {
	hosts := make([]string, len(members))
	for i, m := range members {
		hosts[i] = m.Host
	}
	return hosts
}

// RSConfigMembersToState converts ConfigMembers to the []interface{} format
// for Terraform state, in the order of managedHosts (the member blocks), so
// the list compares position by position with the configuration instead of
// diffing forever when the server orders members differently. Hosts not in
// the replica set are skipped. If managedHosts is nil, returns nil (no member
// block declared).
// SHARD-007: Read-back for drift detection.
// SHARD-008: Only managed hosts returned.
// SHARD-025: Block order, not server order.
func RSConfigMembersToState(members ConfigMembers, managedHosts []string) []interface{} {
	if managedHosts == nil {
		return nil
	}

	byHost := make(map[string]ConfigMember, len(members))
	for _, m := range members {
		byHost[m.Host] = m
	}
	result := make([]interface{}, 0, len(managedHosts))
	for _, host := range managedHosts {
		m, ok := byHost[host]
		if !ok {
			continue
		}

		memberMap := map[string]interface{}{
			"host":          m.Host,
			"priority":      m.Priority,
			"arbiter_only":  derefBool(m.ArbiterOnly),
			"build_indexes": derefBool(m.BuildIndexes),
			"hidden":        derefBool(m.Hidden),
			"votes":         derefInt(m.Votes),
		}

		if m.Tags != nil {
			tags := make(map[string]interface{}, len(m.Tags))
			for k, v := range m.Tags {
				tags[k] = v
			}
			memberMap["tags"] = tags
		} else {
			memberMap["tags"] = map[string]interface{}{}
		}

		result = append(result, memberMap)
	}

	return result
}

// validateMemberOverrides returns diagnostics if any member has an empty or
// missing host, or if two blocks name the same host: the second would be
// merged twice or, for a new host, added a second time and rejected by the
// server after the first add went through. SHARD-026
func validateMemberOverrides(overrides []MemberOverride) diag.Diagnostics {
	seen := make(map[string]int, len(overrides))
	for i, o := range overrides {
		if strings.TrimSpace(o.Host) == "" {
			return diag.Errorf("member at index %d: host is required and must be non-empty (host:port)", i)
		}
		if first, dup := seen[o.Host]; dup {
			return diag.Errorf("member at index %d: host %q is already declared by the member at index %d", i, o.Host, first)
		}
		seen[o.Host] = i
	}
	return nil
}

// extractMemberOverrides parses the Terraform "member" block into MemberOverride structs.
// Host is read safely; use validateMemberOverrides after extraction to reject empty or missing host.
func extractMemberOverrides(data *schema.ResourceData) ([]MemberOverride, bool) {
	v, ok := data.GetOk("member")
	if !ok {
		return nil, false
	}
	tfMembers := v.([]interface{})
	if len(tfMembers) == 0 {
		return nil, false
	}
	overrides := make([]MemberOverride, 0, len(tfMembers))
	for _, raw := range tfMembers {
		m := raw.(map[string]interface{})
		var host string
		if v, ok := m["host"]; ok && v != nil {
			if s, ok := v.(string); ok {
				host = s
			}
		}
		override := MemberOverride{
			Host:         host,
			Priority:     toFloat64(m["priority"]),
			Votes:        m["votes"].(int),
			Hidden:       m["hidden"].(bool),
			ArbiterOnly:  m["arbiter_only"].(bool),
			BuildIndexes: m["build_indexes"].(bool),
		}
		if tags, ok := m["tags"].(map[string]interface{}); ok && len(tags) > 0 {
			override.Tags = make(map[string]string, len(tags))
			for k, v := range tags {
				override.Tags[k] = v.(string)
			}
		}
		// SHARD-024: the server forces an arbiter's priority to 0, so send 0
		// rather than the schema default and let the read-back match.
		if override.ArbiterOnly {
			override.Priority = 0
		}
		overrides = append(overrides, override)
	}
	return overrides, true
}

// managedHostsFromState returns the hosts of the member blocks in block order,
// without duplicates. Entries with a missing or empty host are skipped so
// invalid state cannot panic. SHARD-025
func managedHostsFromState(data *schema.ResourceData) []string {
	v, ok := data.GetOk("member")
	if !ok {
		return nil
	}
	tfMembers := v.([]interface{})
	if len(tfMembers) == 0 {
		return nil
	}
	managed := make([]string, 0, len(tfMembers))
	seen := make(map[string]bool, len(tfMembers))
	for _, raw := range tfMembers {
		m := raw.(map[string]interface{})
		var host string
		if v, ok := m["host"]; ok && v != nil {
			if s, ok := v.(string); ok {
				host = s
			}
		}
		if strings.TrimSpace(host) != "" && !seen[host] {
			seen[host] = true
			managed = append(managed, host)
		}
	}
	return managed
}

// suppressArbiterPriority hides priority diffs on arbiter blocks. The server
// forces an arbiter's priority to 0, so the schema default of 1 would
// otherwise diff against the read-back forever. k is "member.N.priority".
// SHARD-024
func suppressArbiterPriority(k, _, _ string, d *schema.ResourceData) bool {
	arbiter, _ := d.Get(strings.TrimSuffix(k, "priority") + "arbiter_only").(bool)
	return arbiter
}

// oplogConfigured returns true if oplog_size_mb is explicitly set.
// OPLOG-004
func oplogConfigured(data *schema.ResourceData) bool {
	_, ok := data.GetOk("oplog_size_mb")
	return ok
}

// oplogResizeMemberTimeout bounds each per-member connect+resize so one wedged
// member (e.g. fsyncLock, stalled storage) cannot hang an apply indefinitely.
const oplogResizeMemberTimeout = 60 * time.Second

// oplogReadMemberTimeout bounds each per-member connect+collStats during the
// read-back. Deliberately shorter than the resize timeout: the read-back runs
// on every refresh, so an unreachable member should cost seconds, not the
// better part of a minute. OPLOG-018
const oplogReadMemberTimeout = 20 * time.Second

// applyOplogConfig applies oplog size via replSetResizeOplog if configured
// and changed. replSetResizeOplog only resizes the member it is issued
// against, so the command is fanned out to every data-bearing member over a
// direct connection, secondaries first and the primary last. Members that
// are not PRIMARY or SECONDARY (e.g. still in initial sync) are skipped;
// their size surfaces as drift on the next plan. It reports whether the
// fan-out ran, so the caller can tell an untouched oplog from a resized one
// when the read-back cannot reach any member.
// OPLOG-003, OPLOG-004, OPLOG-006, OPLOG-009, OPLOG-010, OPLOG-012,
// OPLOG-014, OPLOG-016, OPLOG-019
func applyOplogConfig(ctx context.Context, client *mongo.Client, data *schema.ResourceData, members ConfigMembers, providerConf *MongoDatabaseConfiguration, allowNoAuthFallback bool) (bool, error) {
	if !oplogConfigured(data) {
		return false, nil
	}
	// OPLOG-014: the resize is idempotent but not free (one direct connection
	// per member); skip the fan-out when the configured size cannot have
	// changed. Pending convergence always shows as a change because the
	// read-back stores OplogSizeMismatch or a divergent size.
	if !data.IsNewResource() && !data.HasChange("oplog_size_mb") {
		return false, nil
	}
	sizeMB := data.Get("oplog_size_mb").(float64)

	primaryHost := ""
	var resizableHosts map[string]bool
	status, err := GetReplSetStatus(ctx, client)
	if err != nil {
		// OPLOG-010/016: ordering and health checks are best-effort; fall
		// back to configuration order rather than failing a resize that
		// would succeed.
		tflog.Warn(ctx, "replSetGetStatus failed; resizing in configuration order without member health checks", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		if p := status.Primary(); p != nil {
			primaryHost = p.Name
			found := false
			for _, m := range members {
				if m.Host == primaryHost {
					found = true
					break
				}
			}
			if !found {
				tflog.Warn(ctx, "primary from replSetGetStatus does not match any configured member; resizing in configuration order", map[string]interface{}{
					"primary": primaryHost,
				})
			}
		} else {
			tflog.Warn(ctx, "no PRIMARY identified; resizing in configuration order")
		}
		resizableHosts = make(map[string]bool)
		for _, m := range status.GetMembersByState(MemberStatePrimary, 0) {
			resizableHosts[m.Name] = true
		}
		for _, m := range status.GetMembersByState(MemberStateSecondary, 0) {
			resizableHosts[m.Name] = true
		}
	}

	err = ResizeOplogAcrossMembers(ctx, members, primaryHost, func(ctx context.Context, host string) (bool, error) {
		// OPLOG-016: a member still syncing has no resizable oplog yet; skip
		// it instead of failing the apply. It surfaces as drift later.
		if resizableHosts != nil && !resizableHosts[host] {
			tflog.Warn(ctx, "skipping oplog resize on member that is not PRIMARY or SECONDARY", map[string]interface{}{
				"host": host,
			})
			return false, nil
		}
		memberCtx, cancel := context.WithTimeout(ctx, oplogResizeMemberTimeout)
		defer cancel()
		memberClient, cleanup, err := connectToMember(memberCtx, providerConf, host, allowNoAuthFallback)
		if err != nil {
			return false, err
		}
		defer cleanup()
		if err := SetOplogConfig(memberCtx, memberClient, sizeMB); err != nil {
			return false, err
		}
		return true, nil
	})
	return true, err
}

// readOplogConfig reads the oplog size of every data-bearing member and
// stores the common size into Terraform state, or OplogSizeMismatch when
// members disagree, so both undersized and oversized members surface as
// drift in the next plan. Unreachable members are skipped with a warning so
// a single down member does not fail refresh; when no member can be read,
// the value already in state is kept — unless a resize was just attempted,
// in which case OplogSizeMismatch is stored so an apply whose outcome could
// not be confirmed surfaces as drift instead of recording the configured
// size as achieved.
// OPLOG-005, OPLOG-008, OPLOG-011, OPLOG-013, OPLOG-020
func readOplogConfig(ctx context.Context, data *schema.ResourceData, members ConfigMembers, providerConf *MongoDatabaseConfiguration, allowNoAuthFallback bool, resizeAttempted bool) error {
	if !oplogConfigured(data) {
		return nil
	}
	size, err := OplogSizeAcrossMembers(ctx, members, func(ctx context.Context, host string) (float64, error) {
		memberCtx, cancel := context.WithTimeout(ctx, oplogReadMemberTimeout)
		defer cancel()
		memberClient, cleanup, err := connectToMember(memberCtx, providerConf, host, allowNoAuthFallback)
		if err != nil {
			return 0, err
		}
		defer cleanup()
		cfg, err := GetOplogConfig(memberCtx, memberClient)
		if err != nil {
			return 0, err
		}
		return cfg.SizeMB, nil
	}, func(host string, err error) {
		tflog.Warn(ctx, "skipping unreadable member during oplog size read", map[string]interface{}{
			"host":  host,
			"error": err.Error(),
		})
	})
	if err != nil {
		// OPLOG-020: a resize just ran and no member could confirm it. Keeping
		// the configured value would record the apply as achieved; store the
		// mismatch sentinel so the next plan re-applies it.
		if resizeAttempted {
			tflog.Warn(ctx, "could not read oplog size from any member after a resize; recording a mismatch so the next plan re-applies", map[string]interface{}{
				"error": err.Error(),
			})
			return data.Set("oplog_size_mb", OplogSizeMismatch)
		}
		// OPLOG-008: failing here would block every plan of the resource
		// (including the one that fixes the problem); keep the last known
		// value instead.
		tflog.Warn(ctx, "could not read oplog size from any member; keeping value from state", map[string]interface{}{
			"error": err.Error(),
		})
		return nil
	}
	return data.Set("oplog_size_mb", size)
}

// memberPreflightTimeout bounds the direct connection and isMaster that
// inspect a host before it is added. As with the oplog read-back, a host that
// cannot be reached from here should cost seconds. SHARD-027
const memberPreflightTimeout = 20 * time.Second

// memberProbeFor returns a probe that connects directly to a host without
// credentials and runs isMaster, which needs none. A fresh node has no users
// yet and a live member answers before authentication, so one connection
// reads both, and the provider's password is never sent to a host that is so
// far only a name in the configuration. SHARD-027
func memberProbeFor(providerConf *MongoDatabaseConfiguration) memberProbe {
	return func(ctx context.Context, hostPort string) (*IsMasterResp, error) {
		host, port, err := SplitHostPort(hostPort)
		if err != nil {
			return nil, fmt.Errorf("invalid member host %q: %w", hostPort, err)
		}
		probeCtx, cancel := context.WithTimeout(ctx, memberPreflightTimeout)
		defer cancel()
		client, err := MongoClientInitNoAuth(probeCtx, &MongoDatabaseConfiguration{
			Config:          BuildShardClientConfig(providerConf.Config, host, port, ""),
			MaxConnLifetime: providerConf.MaxConnLifetime,
		})
		if err != nil {
			return nil, err
		}
		defer func() { _ = client.Disconnect(probeCtx) }()
		return GetIsMaster(probeCtx, client)
	}
}

// errMemberProbeSkipped is what the probe reports under host_override: the
// member hosts are by definition not reachable from the runner (DISC-008), so
// every host to add reads as uninspectable. Data-bearing members are then
// added on the strength of the wait after the add; arbiters are refused.
// SHARD-027, SHARD-028
var errMemberProbeSkipped = errors.New("host_override is set, so member hosts are not reachable from the Terraform runner")

func memberProbeSkipped(context.Context, string) (*IsMasterResp, error) {
	return nil, errMemberProbeSkipped
}

// connectToMember opens a direct connection to a single replica set member
// for node-local commands, inheriting the provider's credentials, TLS, and
// proxy settings. host_override does not apply here; the schema rejects it
// in combination with oplog_size_mb (OPLOG-015). The no-auth fallback
// (INIT-018) is only allowed on paths reached from replica set
// initialization — including the already-initialized delegation into
// updateWithClient — where the replica set may exist before any users do;
// on steady-state paths an authentication failure must surface as one.
// OPLOG-017
func connectToMember(ctx context.Context, providerConf *MongoDatabaseConfiguration, hostPort string, allowNoAuthFallback bool) (*mongo.Client, func(), error) {
	host, port, err := SplitHostPort(hostPort)
	if err != nil {
		return nil, func() {}, fmt.Errorf("invalid member host %q: %w", hostPort, err)
	}
	if allowNoAuthFallback {
		return ConnectForInit(ctx, providerConf.Config, host, port, providerConf.MaxConnLifetime)
	}
	memberConf := &MongoDatabaseConfiguration{
		Config:          BuildShardClientConfig(providerConf.Config, host, port, ""),
		MaxConnLifetime: providerConf.MaxConnLifetime,
	}
	client, err := MongoClientInit(ctx, memberConf)
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to connect to member %s: %w", hostPort, err)
	}
	return client, func() { _ = client.Disconnect(ctx) }, nil
}

func (r *ResourceShardConfig) Update(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	client, cleanup, err := r.getShardClient(ctx, data, i)
	if err != nil {
		return diag.FromErr(err)
	}
	defer cleanup()
	return r.updateWithClient(ctx, data, client, i.(*MongoDatabaseConfiguration), false)
}

// updateWithClient reconciles an initialized replica set with the Terraform
// configuration: settings and matched members in one replSetReconfig, then
// missing members one reconfig each, then state from a final read. Shared by
// Update and initializeReplicaSet; allowNoAuthFallback is passed through to
// the per-member oplog connections (INIT-018, OPLOG-017). INIT-030
func (r *ResourceShardConfig) updateWithClient(ctx context.Context, data *schema.ResourceData, client *mongo.Client, providerConf *MongoDatabaseConfiguration, allowNoAuthFallback bool) diag.Diagnostics {
	var m ShardModel

	m.Settings.ChainingAllowed = data.Get("chaining_allowed").(bool)
	m.Settings.HeartbeatIntervalMillis = int64(data.Get("heartbeat_interval_millis").(int))
	m.Settings.HeartbeatTimeoutSecs = data.Get("heartbeat_timeout_secs").(int)
	m.Settings.ElectionTimeoutMillis = int64(data.Get("election_timeout_millis").(int))
	// CATCHUP-005
	m.Settings.CatchUpTimeoutMillis = int64(data.Get("catch_up_timeout_millis").(int))

	config, errD := r.getReplSetConfig(ctx, client)
	if errD != nil {
		return errD
	}

	ctx = tflog.SetField(ctx, `replSetConfig`, config)
	tflog.Debug(ctx, `fetched ReplSetConfig`)

	version := config.Version
	version += 1
	config.Version = version

	config.Settings.ChainingAllowed = m.Settings.ChainingAllowed
	config.Settings.HeartbeatIntervalMillis = m.Settings.HeartbeatIntervalMillis
	config.Settings.HeartbeatTimeoutSecs = m.Settings.HeartbeatTimeoutSecs
	config.Settings.ElectionTimeoutMillis = m.Settings.ElectionTimeoutMillis
	// CATCHUP-003
	config.Settings.CatchUpTimeoutMillis = m.Settings.CatchUpTimeoutMillis

	// SHARD-012: matched blocks are merged here, except that a block raising
	// a member's votes keeps the live votes and priority for now (SHARD-023);
	// promotions and adds each get their own reconfig after this one
	// (SHARD-013).
	var newMembers, promotions []MemberOverride
	if overrides, ok := extractMemberOverrides(data); ok {
		if errD := validateMemberOverrides(overrides); errD != nil {
			return errD
		}
		existing, missing := PartitionMemberOverrides(config.Members, overrides)
		// SHARD-029: a host that is still starting is waited for, up to the
		// operation timeout, before it is inspected.
		// SHARD-027: a host that is already a member under another name must
		// be refused before anything is sent. Under host_override the member
		// hosts are not reachable from here (DISC-008), so neither the wait
		// nor the probe is attempted and every host reads as uninspectable:
		// data-bearing adds go ahead with the wait after each add as the only
		// check, and arbiters are refused (SHARD-028).
		if len(missing) > 0 {
			probe := memberProbeFor(providerConf)
			if _, overridden := data.GetOk("host_override"); overridden {
				probe = memberProbeSkipped
			}
			if err := WaitForAddTargets(ctx, missing, probe, readyPollInterval); err != nil {
				return diag.FromErr(err)
			}
			if err := PreflightAddTargets(ctx, config.ID, missing, probe); err != nil {
				return diag.FromErr(err)
			}
		}
		held, promote := HoldPromotions(config.Members, existing)
		merged, mergeErr := MergeMembers(config.Members, held)
		if mergeErr != nil {
			return diag.FromErr(mergeErr)
		}
		config.Members = merged
		newMembers = missing
		promotions = promote
	}

	ctx = tflog.SetField(ctx, `updated replSetConfig`, config)
	tflog.Debug(ctx, `replacement ReplSetConfig`)

	// INIT-025/026/027/028: Retry on transient post-election errors
	timeout := time.Duration(data.Get("init_timeout_secs").(int)) * time.Second
	err := SetReplSetConfigWithRetry(ctx, client, config, timeout)
	if err != nil {
		return diag.FromErr(err)
	}

	data.SetId(config.ID)
	if err := data.Set("shard_name", config.ID); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("chaining_allowed", config.Settings.ChainingAllowed); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("heartbeat_interval_millis", config.Settings.HeartbeatIntervalMillis); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("heartbeat_timeout_secs", config.Settings.HeartbeatTimeoutSecs); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("election_timeout_millis", config.Settings.ElectionTimeoutMillis); err != nil {
		return diag.FromErr(err)
	}
	// CATCHUP-004
	if err := data.Set("catch_up_timeout_millis", config.Settings.CatchUpTimeoutMillis); err != nil {
		return diag.FromErr(err)
	}

	// SHARD-013/021/023: promotions and adds run after the settings are in
	// state, so a failure here cannot leave the applied reconfig unrecorded;
	// the next apply re-partitions against the live config and finishes only
	// what is still missing. Promotions go first: they are members from an
	// earlier apply that have had time to sync.
	pending, memberErr := ReconcileMembers(ctx, promotions, newMembers, memberAddOpsForClient(client, timeout))

	// SHARD-018/021: state comes from the config the server holds now, which
	// the adds (and newlyAdded reconfigs on 4.4+) may have changed. This runs
	// even when the member phase failed: the SDK persists state on error, and
	// without this write it would hold the planned blocks, such as votes 1 for
	// a member the server has at votes 0, so a plan without refresh would show
	// nothing left to do.
	finalConfig, err := GetReplSetConfig(ctx, client)
	if err != nil {
		if memberErr != nil {
			return diag.Errorf("%s; reading the configuration afterwards also failed: %s", memberErr, err)
		}
		return diag.FromErr(err)
	}
	managedHosts := managedHostsFromState(data)
	if err := data.Set("member", RSConfigMembersToState(finalConfig.Members, managedHosts)); err != nil {
		return diag.FromErr(err)
	}
	if memberErr != nil {
		return diag.FromErr(memberErr)
	}
	diags := pendingMemberWarnings(pending)

	// OPLOG-003: members still in initial sync are skipped by the fan-out
	// (OPLOG-016) and surface as drift once they are SECONDARY.
	resizeAttempted, err := applyOplogConfig(ctx, client, data, finalConfig.Members, providerConf, allowNoAuthFallback)
	if err != nil {
		return append(diags, diag.FromErr(err)...)
	}

	// OPLOG-005: Read back oplog state for drift detection
	if err := readOplogConfig(ctx, data, finalConfig.Members, providerConf, allowNoAuthFallback, resizeAttempted); err != nil {
		return append(diags, diag.FromErr(err)...)
	}

	return diags
}

// pendingMemberWarnings turns members that were still syncing when their wait
// ended into warnings. The apply succeeds, state records them as non-voters,
// and the next plan shows the promotion as a pending change. SHARD-017,
// SHARD-023
func pendingMemberWarnings(pending []memberPending) diag.Diagnostics {
	var diags diag.Diagnostics
	for _, p := range pending {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Warning,
			Summary:  p.Summary(),
			Detail:   p.Detail(),
		})
	}
	return diags
}

func (r *ResourceShardConfig) getReplSetConfig(ctx context.Context, client *mongo.Client) (*RSConfig, diag.Diagnostics) {
	rs, err := GetReplSetConfig(ctx, client)
	return rs, diag.FromErr(err)
}

func (r *ResourceShardConfig) Read(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	client, cleanup, err := r.getShardClient(ctx, data, i)
	if err != nil {
		return diag.FromErr(err)
	}
	defer cleanup()

	config, errD := r.getReplSetConfig(ctx, client)
	if errD != nil {
		return errD
	}

	data.SetId(config.ID)
	if err := data.Set("shard_name", config.ID); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("chaining_allowed", config.Settings.ChainingAllowed); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("heartbeat_interval_millis", config.Settings.HeartbeatIntervalMillis); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("heartbeat_timeout_secs", config.Settings.HeartbeatTimeoutSecs); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("election_timeout_millis", config.Settings.ElectionTimeoutMillis); err != nil {
		return diag.FromErr(err)
	}
	// CATCHUP-004
	if err := data.Set("catch_up_timeout_millis", config.Settings.CatchUpTimeoutMillis); err != nil {
		return diag.FromErr(err)
	}

	// SHARD-007/008/010: Read back managed members for drift detection
	managedHosts := managedHostsFromState(data)
	memberState := RSConfigMembersToState(config.Members, managedHosts)
	if err := data.Set("member", memberState); err != nil {
		return diag.FromErr(err)
	}

	// OPLOG-005: Read back oplog config for drift detection
	if err := readOplogConfig(ctx, data, config.Members, i.(*MongoDatabaseConfiguration), false, false); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func (r *ResourceShardConfig) Delete(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	client, cleanup, err := r.getShardClient(ctx, data, i)
	if err != nil {
		return diag.FromErr(err)
	}
	defer cleanup()

	_ = client
	//	var stateId = data.State().ID
	//	roleName, database, err := r.ParseId(stateId)
	//
	//	if err != nil {
	//		return diag.Errorf("%s", err)
	//	}
	//
	//	db := client.Database(database)
	//	result := db.RunCommand(context.Background(), bson.D{{Key: "dropRole", Value: roleName}})
	//
	//	if result.Err() != nil {
	//		return diag.Errorf("%s", result.Err())
	//	}
	//
	return nil
}

// getShardClient returns a MongoDB client connected to the appropriate shard.
// If the provider is connected to a mongos, it auto-discovers the shard via
// listShards and creates a temporary direct connection. The returned cleanup
// function MUST be called via defer to disconnect temporary clients. Errors
// wrap the driver's so callers can tell a refused connection from a refused
// login (INIT-034, INIT-035).
// DISC-001 through DISC-010
func (r *ResourceShardConfig) getShardClient(ctx context.Context, data *schema.ResourceData, i interface{}) (*mongo.Client, func(), error) {
	providerConf := i.(*MongoDatabaseConfiguration)
	providerClient, err := MongoClientInit(ctx, providerConf)
	if err != nil {
		return nil, func() {}, fmt.Errorf("Error connecting to database: %w", err)
	}

	shardName := data.Get("shard_name").(string)
	hostOverride := ""
	if v, ok := data.GetOk("host_override"); ok {
		hostOverride = v.(string)
	}

	shardClient, shardCleanup, err := ResolveShardClient(
		ctx, providerClient, providerConf.Config,
		shardName, hostOverride, providerConf.MaxConnLifetime,
	)
	if err != nil {
		_ = providerClient.Disconnect(ctx)
		return nil, func() {}, fmt.Errorf("Error resolving shard client: %w", err)
	}

	// Build a combined cleanup that disconnects both clients when the shard
	// client is a separate temporary connection, or just the provider client.
	cleanup := func() {
		shardCleanup()
		// If shard client is the same as provider client, shardCleanup is a
		// noop, so we still need to disconnect the provider client.
		if shardClient != providerClient {
			_ = providerClient.Disconnect(ctx)
		} else {
			_ = providerClient.Disconnect(ctx)
		}
	}

	return shardClient, cleanup, nil
}

func (r *ResourceShardConfig) ParseId(id string) (string, string, error) {
	result, errEncoding := base64.StdEncoding.DecodeString(id)

	if errEncoding != nil {
		return "", "", fmt.Errorf("unexpected format of ID Error : %s", errEncoding)
	}
	parts := strings.SplitN(string(result), ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("unexpected format of ID (%s), expected database.roleName", id)
	}

	database := parts[0]
	roleName := parts[1]

	return roleName, database, nil
}

var RShardConfig = ResourceShardConfig{}

func resourceShardConfig() *schema.Resource {
	return &schema.Resource{
		CreateContext: RShardConfig.Create,
		ReadContext:   RShardConfig.Read,
		UpdateContext: RShardConfig.Update,
		DeleteContext: RShardConfig.Delete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		// GATE-005: require feature opt-in
		// PREVIEW-022, PREVIEW-023: command preview
		CustomizeDiff: customdiff.All(
			requireFeature("mongodb_shard_config"),
			previewCommands(shardConfigCommandPreview),
		),
		// INIT-033: the create and update timeouts bound the whole operation,
		// including the waits for hosts that are still starting (INIT-034,
		// SHARD-029) and for the provider user to be created (INIT-035).
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(DefaultShardConfigTimeout),
			Update: schema.DefaultTimeout(DefaultShardConfigTimeout),
		},
		Schema: map[string]*schema.Schema{
			"planned_commands": commandPreviewSchema(), // PREVIEW-005
			"shard_name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"chaining_allowed": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
			},
			"heartbeat_interval_millis": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1000,
			},
			"heartbeat_timeout_secs": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  10,
			},
			"election_timeout_millis": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  10000,
			},
			// CATCHUP-001: Optional catchup timeout in milliseconds
			"catch_up_timeout_millis": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  -1,
			},
			// SHARD-001: Optional member block for per-member configuration
			"member": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"host": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The host:port of the replica set member to configure",
						},
						"arbiter_only": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Whether this member is an arbiter",
						},
						"build_indexes": {
							Type:        schema.TypeBool,
							Optional:    true,
							Default:     true,
							Description: "Whether this member builds indexes",
						},
						"hidden": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Whether this member is hidden from client discovery",
						},
						// SHARD-020: sent explicitly, including 0 (required for hidden members).
						// SHARD-024: arbiters always have priority 0; the diff is suppressed for them.
						"priority": {
							Type:             schema.TypeFloat,
							Optional:         true,
							Default:          1.0,
							DiffSuppressFunc: suppressArbiterPriority,
							Description:      "Election priority for this member (0 = never primary). MongoDB accepts 0-1000 (integer or decimal). Always 0 for arbiters. Default 1.",
						},
						"tags": {
							Type:     schema.TypeMap,
							Optional: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
							Description: "Replica set tags for this member (zone, dc, rack, etc.)",
						},
						"votes": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     1,
							Description: "Number of votes this member has in elections (0 or 1). Default 1.",
						},
					},
				},
			},
			// INIT-020/021: Timeout for replica set initialization
			"init_timeout_secs": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     DefaultInitTimeoutSecs,
				Description: "Timeout in seconds for replica set initialization, and separately for each added or promoted member to reach its required state. A member still syncing at the timeout stays a non-voter and is promoted by a later apply.",
			},
			// OPLOG-001/002: Optional oplog size in megabytes
			"oplog_size_mb": {
				Type:     schema.TypeFloat,
				Optional: true,
				// OPLOG-015: the per-member resize/read connections dial the
				// member hostnames from the replica set configuration, which
				// host_override exists to avoid; reject the combination at
				// plan time instead of failing every apply and refresh.
				ConflictsWith: []string{"host_override"},
				ValidateFunc: func(val interface{}, key string) ([]string, []error) {
					v := val.(float64)
					if v <= 0 {
						return nil, []error{fmt.Errorf("%q must be > 0, got: %v", key, v)}
					}
					return nil, nil
				},
				Description: "Maximum oplog size in megabytes. Applied via replSetResizeOplog on every data-bearing member. Conflicts with host_override.",
			},
			// DISC-008: Override the shard host discovered via listShards
			"host_override": {
				Type:     schema.TypeString,
				Optional: true,
				Description: "Override the shard host:port discovered via listShards. " +
					"Use when internal hostnames from listShards are unreachable from the Terraform runner.",
			},
		},
	}
}

/*
					db.adminCommand({replSetGetConfig: 1})
					config.{members|settings}
					{
	  //_id: <string>,
	  //version: <int>, (make this computed)
	  //term: <int>,
	  //protocolVersion: <number>,
	  //writeConcernMajorityJournalDefault: <boolean>,
	  //configsvr: <boolean>,
	  members: [
	    {
	      _id: <int>,
	      host: <string>,
	      arbiterOnly: <boolean>,
	      buildIndexes: <boolean>,
	      hidden: <boolean>,
	      priority: <number>,
	      tags: <document>,
	      votes: <number>
	    },
	    ...
	  ],
	  settings: {
	    chainingAllowed : <boolean>,
	    heartbeatIntervalMillis : <int>,
	    heartbeatTimeoutSecs: <int>,
	    electionTimeoutMillis : <int>,
	    catchUpTimeoutMillis : <int>,
	    getLastErrorModes : <document>,
	    getLastErrorDefaults : <document>,
	    replicaSetId: <ObjectId>
	  }
	}

					https://www.mongodb.com/docs/manual/reference/command/replSetReconfig/#mongodb-dbcommand-dbcmd.replSetReconfig
					Change with: {replSetReconfig: {document}, force: false|true})
					{
			        "config" : {
			                "_id" : "shard01",
			                "version" : 1,
			                "protocolVersion" : NumberLong(1),
			                "members" : [
			                        {
			                                "_id" : 0,
			                                "host" : "localhost:27018",
			                                "arbiterOnly" : false,
			                                "buildIndexes" : true,
			                                "hidden" : false,
			                                "priority" : 1,
			                                "tags" : {

			                                },
			                                "slaveDelay" : NumberLong(0),
			                                "votes" : 1
			                        },
			                        {
			                                "_id" : 1,
			                                "host" : "localhost:27019",
			                                "arbiterOnly" : false,
			                                "buildIndexes" : true,
			                                "hidden" : false,
			                                "priority" : 1,
			                                "tags" : {

			                                },
			                                "slaveDelay" : NumberLong(0),
			                                "votes" : 1
			                        },
			                        {
			                                "_id" : 2,
			                                "host" : "localhost:27020",
			                                "arbiterOnly" : false,
			                                "buildIndexes" : true,
			                                "hidden" : false,
			                                "priority" : 1,
			                                "tags" : {

			                                },
			                                "slaveDelay" : NumberLong(0),
			                                "votes" : 1
			                        }
			                ],
			                "settings" : {
			                        "chainingAllowed" : true,
			                        "heartbeatIntervalMillis" : 2000,
			                        "heartbeatTimeoutSecs" : 10,
			                        "electionTimeoutMillis" : 10000,
			                        "catchUpTimeoutMillis" : -1,
			                        "catchUpTakeoverDelayMillis" : 30000,
			                        "getLastErrorModes" : {

			                        },
			                        "getLastErrorDefaults" : {
			                                "w" : 1,
			                                "wtimeout" : 0
			                        },
			                        "replicaSetId" : ObjectId("668874d02b2227c1922e0a7e")
			                }
			        },
			        "ok" : 1,
			        "operationTime" : Timestamp(1720219056, 1),
			        "$gleStats" : {
			                "lastOpTime" : Timestamp(0, 0),
			                "electionId" : ObjectId("7fffffff0000000000000002")
			        },
			        "$configServerState" : {
			                "opTime" : {
			                        "ts" : Timestamp(1720219056, 1),
			                        "t" : NumberLong(2)
			                }
			        },
			        "$clusterTime" : {
			                "clusterTime" : Timestamp(1720219057, 1),
			                "signature" : {
			                        "hash" : BinData(0,"BNjajv5gy5tmFSBhkGccuL0QZwk="),
			                        "keyId" : NumberLong("7388283642583187458")
			                }
			        }
			}
*/

//data.SetId(stateID)
//diags = nil
//return diags
