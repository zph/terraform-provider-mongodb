package mongodb

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// commandPreviewSchema returns the planned_commands schema field.
// Added to every resource that supports command preview. // PREVIEW-005
func commandPreviewSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeString,
		Computed: true,
		Description: "Preview of MongoDB commands that will execute on apply. " +
			"Populated during plan when command_preview is enabled at the provider level.",
	}
}

// commandPreviewEnabled checks if command preview is enabled in provider config.
// Returns false if meta is nil or wrong type. // PREVIEW-010
func commandPreviewEnabled(meta interface{}) bool {
	if meta == nil {
		return false
	}
	conf, ok := meta.(*MongoDatabaseConfiguration)
	if !ok || conf == nil {
		return false
	}
	return conf.CommandPreview
}

// CommandBuilder produces a preview string from a ResourceDiff.
type CommandBuilder func(d *schema.ResourceDiff) string

// previewCommands returns a CustomizeDiffFunc that populates planned_commands.
// PREVIEW-006, PREVIEW-007, PREVIEW-008
func previewCommands(builder CommandBuilder) schema.CustomizeDiffFunc {
	return func(_ context.Context, d *schema.ResourceDiff, meta interface{}) error {
		if !commandPreviewEnabled(meta) {
			return nil
		}
		preview := builder(d)
		if preview != "" {
			return d.SetNew("planned_commands", preview)
		}
		return nil
	}
}

// --- Pure builder functions (testable without ResourceDiff) ---

// previewRole is a simplified role for preview output.
type previewRole struct {
	Role string
	DB   string
}

// previewPrivilege is a simplified privilege for preview output.
type previewPrivilege struct {
	DB         string
	Collection string
	Cluster    bool
	Actions    []string
}

// formatPreviewRoles formats a role list for mongo shell display.
func formatPreviewRoles(roles []previewRole) string {
	if len(roles) == 0 {
		return "[]"
	}
	parts := make([]string, len(roles))
	for i, r := range roles {
		parts[i] = fmt.Sprintf("{role: %q, db: %q}", r.Role, r.DB)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// formatPreviewPrivileges formats a privilege list for mongo shell display.
func formatPreviewPrivileges(privs []previewPrivilege) string {
	if len(privs) == 0 {
		return "[]"
	}
	parts := make([]string, len(privs))
	for i, p := range privs {
		resource := fmt.Sprintf("{db: %q, collection: %q}", p.DB, p.Collection)
		if p.Cluster {
			resource = "{cluster: true}"
		}
		actions := make([]string, len(p.Actions))
		for j, a := range p.Actions {
			actions[j] = fmt.Sprintf("%q", a)
		}
		parts[i] = fmt.Sprintf("{resource: %s, actions: [%s]}", resource, strings.Join(actions, ", "))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// buildServerParameterPreview builds the setParameter command string.
// PREVIEW-013
func buildServerParameterPreview(param, value string, _ bool) string {
	return fmt.Sprintf("db.adminCommand({setParameter: 1, %q: %q})", param, value)
}

// buildProfilerPreview builds the profile command string.
// PREVIEW-012
func buildProfilerPreview(database string, level, slowms, ratelimit int) string {
	return fmt.Sprintf("db.getSiblingDB(%q).runCommand({profile: %d, slowms: %d, ratelimit: %d})",
		database, level, slowms, ratelimit)
}

// buildFCVPreview builds the setFeatureCompatibilityVersion command string.
// PREVIEW-020
func buildFCVPreview(version string) string {
	return fmt.Sprintf("db.adminCommand({setFeatureCompatibilityVersion: %q})", version)
}

// buildDbUserPreview builds the createUser/updateUser command string.
// PREVIEW-016, PREVIEW-017
func buildDbUserPreview(database, name string, roles []previewRole, isCreate bool) string {
	cmd := "updateUser"
	if isCreate {
		cmd = "createUser"
	}
	return fmt.Sprintf("db.getSiblingDB(%q).runCommand({%s: %q, pwd: [REDACTED], roles: %s})",
		database, cmd, name, formatPreviewRoles(roles))
}

// buildDbRolePreview builds the createRole/updateRole command string.
// PREVIEW-018, PREVIEW-019
func buildDbRolePreview(database, name string, privs []previewPrivilege, inherited []previewRole, isCreate bool) string {
	cmd := "updateRole"
	if isCreate {
		cmd = "createRole"
	}
	return fmt.Sprintf("db.getSiblingDB(%q).runCommand({%s: %q, privileges: %s, roles: %s})",
		database, cmd, name, formatPreviewPrivileges(privs), formatPreviewRoles(inherited))
}

// buildShardPreview builds the addShard command string.
// PREVIEW-015
func buildShardPreview(shardName string, hosts []string) string {
	connStr := BuildShardConnectionString(shardName, hosts)
	return fmt.Sprintf("db.adminCommand({addShard: %q})", connStr)
}

// buildOriginalUserPreview builds the createUser command for original_user.
// PREVIEW-024
func buildOriginalUserPreview(database, username string, roles []previewRole) string {
	return fmt.Sprintf("db.getSiblingDB(%q).runCommand({createUser: %q, pwd: [REDACTED], roles: %s})",
		database, username, formatPreviewRoles(roles))
}

// buildCollectionBalancingPreview builds the collection balancing command.
// Shows both FCV paths since FCV is unknown at plan time.
// PREVIEW-014
func buildCollectionBalancingPreview(namespace string, enabled bool, chunkSizeMB int) string {
	var sb strings.Builder
	// FCV >= 6.0 path
	fmt.Fprintf(&sb, "# If FCV >= 6.0:\n")
	fmt.Fprintf(&sb, "db.adminCommand({configureCollectionBalancing: %q, enableBalancing: %t", namespace, enabled)
	if chunkSizeMB > 0 {
		fmt.Fprintf(&sb, ", chunkSize: %d", chunkSizeMB)
	}
	sb.WriteString("})\n")
	// Legacy path
	fmt.Fprintf(&sb, "# If FCV < 6.0:\n")
	fmt.Fprintf(&sb, `db.getSiblingDB("config").collections.updateOne({_id: %q}, {$set: {noBalance: %t}})`,
		namespace, !enabled)
	return sb.String()
}

// balancerPreviewInput holds the inputs for balancer config preview.
type balancerPreviewInput struct {
	Enabled           bool
	WindowStart       string
	WindowStop        string
	ChunkSizeMB       int
	SecondaryThrottle string
	WaitForDelete     bool
	HasWaitForDelete  bool
}

// buildBalancerConfigPreview builds the multi-command balancer config preview.
// PREVIEW-021
func buildBalancerConfigPreview(in balancerPreviewInput) string {
	var cmds []string
	if in.Enabled {
		cmds = append(cmds, "db.adminCommand({balancerStart: 1})")
	} else {
		cmds = append(cmds, "db.adminCommand({balancerStop: 1})")
	}
	if in.WindowStart != "" && in.WindowStop != "" {
		cmds = append(cmds, fmt.Sprintf(
			`db.getSiblingDB("config").settings.updateOne({_id: "balancer"}, `+
				`{$set: {activeWindow: {start: %q, stop: %q}}}, {upsert: true})`,
			in.WindowStart, in.WindowStop))
	}
	if in.SecondaryThrottle != "" {
		cmds = append(cmds, fmt.Sprintf(
			`db.getSiblingDB("config").settings.updateOne({_id: "balancer"}, `+
				`{$set: {_secondaryThrottle: %q}}, {upsert: true})`,
			in.SecondaryThrottle))
	}
	if in.HasWaitForDelete {
		cmds = append(cmds, fmt.Sprintf(
			`db.getSiblingDB("config").settings.updateOne({_id: "balancer"}, `+
				`{$set: {_waitForDelete: %t}}, {upsert: true})`,
			in.WaitForDelete))
	}
	if in.ChunkSizeMB > 0 {
		cmds = append(cmds, fmt.Sprintf(
			`db.getSiblingDB("config").settings.updateOne({_id: "chunksize"}, `+
				`{$set: {value: %d}}, {upsert: true})`,
			in.ChunkSizeMB))
	}
	return strings.Join(cmds, "\n")
}

// buildShardConfigPreview builds the shard config preview.
// PREVIEW-022, PREVIEW-023
func buildShardConfigPreview(shardName string, isCreate bool) string {
	var cmds []string
	if isCreate {
		cmds = append(cmds, fmt.Sprintf(
			"db.adminCommand({replSetInitiate: {_id: %q, members: [...]}})", shardName))
	}
	cmds = append(cmds, fmt.Sprintf(
		"db.adminCommand({replSetReconfig: {_id: %q, version: <current+1>, members: [...], settings: {...}}})", shardName))
	return strings.Join(cmds, "\n")
}

// --- ResourceDiff adapters (bridge from schema to pure functions) ---

// serverParameterCommandPreview extracts fields from ResourceDiff and delegates.
func serverParameterCommandPreview(d *schema.ResourceDiff) string {
	return buildServerParameterPreview(
		d.Get("parameter").(string),
		d.Get("value").(string),
		d.Id() == "",
	)
}

// profilerCommandPreview extracts fields from ResourceDiff and delegates.
func profilerCommandPreview(d *schema.ResourceDiff) string {
	return buildProfilerPreview(
		d.Get("database").(string),
		d.Get("level").(int),
		d.Get("slowms").(int),
		d.Get("ratelimit").(int),
	)
}

// fcvCommandPreview extracts fields from ResourceDiff and delegates.
func fcvCommandPreview(d *schema.ResourceDiff) string {
	return buildFCVPreview(d.Get("version").(string))
}

// dbUserCommandPreview extracts fields from ResourceDiff and delegates.
func dbUserCommandPreview(d *schema.ResourceDiff) string {
	database := d.Get("auth_database").(string)
	name := d.Get("name").(string)
	roles := extractPreviewRoles(d.Get("role").(*schema.Set).List())
	return buildDbUserPreview(database, name, roles, d.Id() == "")
}

// dbRoleCommandPreview extracts fields from ResourceDiff and delegates.
func dbRoleCommandPreview(d *schema.ResourceDiff) string {
	database := d.Get("database").(string)
	name := d.Get("name").(string)
	privs := extractPreviewPrivileges(d.Get("privilege").(*schema.Set).List())
	inherited := extractPreviewRoles(d.Get("inherited_role").(*schema.Set).List())
	return buildDbRolePreview(database, name, privs, inherited, d.Id() == "")
}

// shardCommandPreview extracts fields from ResourceDiff and delegates.
func shardCommandPreview(d *schema.ResourceDiff) string {
	if d.Id() != "" {
		return "" // Updates are no-ops; deletes not visible in CustomizeDiff
	}
	shardName := d.Get("shard_name").(string)
	hostsRaw := d.Get("hosts").([]interface{})
	hosts := make([]string, len(hostsRaw))
	for i, h := range hostsRaw {
		hosts[i] = h.(string)
	}
	return buildShardPreview(shardName, hosts)
}

// originalUserCommandPreview extracts fields from ResourceDiff and delegates.
func originalUserCommandPreview(d *schema.ResourceDiff) string {
	if d.Id() != "" {
		return "" // Updates are blocked
	}
	database := d.Get("auth_database").(string)
	username := d.Get("username").(string)
	roles := extractPreviewRoles(d.Get("role").(*schema.Set).List())
	return buildOriginalUserPreview(database, username, roles)
}

// collectionBalancingCommandPreview extracts fields from ResourceDiff and delegates.
func collectionBalancingCommandPreview(d *schema.ResourceDiff) string {
	namespace := d.Get("namespace").(string)
	enabled := d.Get("enabled").(bool)
	chunkSizeMB := 0
	if v, ok := d.GetOk("chunk_size_mb"); ok {
		chunkSizeMB = v.(int)
	}
	return buildCollectionBalancingPreview(namespace, enabled, chunkSizeMB)
}

// balancerConfigCommandPreview extracts fields from ResourceDiff and delegates.
func balancerConfigCommandPreview(d *schema.ResourceDiff) string {
	in := balancerPreviewInput{
		Enabled:     d.Get("enabled").(bool),
		WindowStart: d.Get("active_window_start").(string),
		WindowStop:  d.Get("active_window_stop").(string),
	}
	if v, ok := d.GetOk("chunk_size_mb"); ok {
		in.ChunkSizeMB = v.(int)
	}
	if v, ok := d.GetOk("secondary_throttle"); ok {
		in.SecondaryThrottle = v.(string)
	}
	if v, ok := d.GetOk("wait_for_delete"); ok {
		in.WaitForDelete = v.(bool)
		in.HasWaitForDelete = true
	}
	return buildBalancerConfigPreview(in)
}

// shardConfigCommandPreview extracts fields from ResourceDiff and delegates.
func shardConfigCommandPreview(d *schema.ResourceDiff) string {
	shardName := d.Get("shard_name").(string)
	return buildShardConfigPreview(shardName, d.Id() == "")
}

// buildShardZonePreview builds the addShardToZone command string.
// ZONE-013
func buildShardZonePreview(shardName, zone string) string {
	return fmt.Sprintf("db.adminCommand({addShardToZone: %q, zone: %q})", shardName, zone)
}

// buildZoneKeyRangePreview builds the updateZoneKeyRange command string.
// ZONE-030
func buildZoneKeyRangePreview(namespace, zone, minJSON, maxJSON string) string {
	return fmt.Sprintf("db.adminCommand({updateZoneKeyRange: %q, min: %s, max: %s, zone: %q})",
		namespace, minJSON, maxJSON, zone)
}

// shardZoneCommandPreview extracts fields from ResourceDiff and delegates.
func shardZoneCommandPreview(d *schema.ResourceDiff) string {
	if d.Id() != "" {
		return "" // Updates are no-ops
	}
	return buildShardZonePreview(
		d.Get("shard_name").(string),
		d.Get("zone").(string),
	)
}

// zoneKeyRangeCommandPreview extracts fields from ResourceDiff and delegates.
func zoneKeyRangeCommandPreview(d *schema.ResourceDiff) string {
	if d.Id() != "" {
		return "" // Updates are no-ops
	}
	return buildZoneKeyRangePreview(
		d.Get("namespace").(string),
		d.Get("zone").(string),
		d.Get("min").(string),
		d.Get("max").(string),
	)
}

// --- Helpers ---

// extractPreviewRoles converts a schema Set list to previewRole slice.
func extractPreviewRoles(roles []interface{}) []previewRole {
	result := make([]previewRole, 0, len(roles))
	for _, r := range roles {
		m := r.(map[string]interface{})
		result = append(result, previewRole{
			Role: m["role"].(string),
			DB:   m["db"].(string),
		})
	}
	return result
}

// extractPreviewPrivileges converts a schema Set list to previewPrivilege slice.
func extractPreviewPrivileges(privs []interface{}) []previewPrivilege {
	result := make([]previewPrivilege, 0, len(privs))
	for _, p := range privs {
		m := p.(map[string]interface{})
		var actions []string
		if actionsRaw, ok := m["actions"].([]interface{}); ok {
			for _, a := range actionsRaw {
				actions = append(actions, a.(string))
			}
		}
		cluster := false
		if c, ok := m["cluster"].(bool); ok {
			cluster = c
		}
		result = append(result, previewPrivilege{
			DB:         m["db"].(string),
			Collection: m["collection"].(string),
			Cluster:    cluster,
			Actions:    actions,
		})
	}
	return result
}
