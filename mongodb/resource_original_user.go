package mongodb

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/mitchellh/mapstructure"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	defaultAuthDatabase = "admin"
)

// ORIG-001: WHEN the provider manages the original admin user on a no-auth
// MongoDB instance, the resource SHALL carry its own connection parameters
// independent of the provider config.
func resourceOriginalUser() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceOriginalUserCreate,
		ReadContext:   resourceOriginalUserRead,
		UpdateContext: resourceOriginalUserUpdate,
		DeleteContext: resourceOriginalUserDelete,
		Schema: map[string]*schema.Schema{
			"host": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "MongoDB host to connect to without auth",
			},
			"port": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "MongoDB port",
			},
			"username": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Admin username to create",
			},
			"password": {
				Type:        schema.TypeString,
				Required:    true,
				Sensitive:   true,
				Description: "Admin password to create",
			},
			"auth_database": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     defaultAuthDatabase,
				Description: "Database to create the user in",
			},
			"role": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"db": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"role": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
				Description: "Roles to assign to the user",
			},
			"replica_set": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "Replica set name. Auto-discovered from the server if not set. When present, the driver uses discovery mode to route writes to the primary.",
			},
			"ssl": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"certificate": {
				Type:      schema.TypeString,
				Optional:  true,
				Sensitive: true,
			},
			"insecure_skip_verify": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
		},
	}
}

// buildOriginalUserConfig builds a ClientConfig from the resource's own
// connection attributes (not from the provider config).
func buildOriginalUserConfig(data *schema.ResourceData) *MongoDatabaseConfiguration {
	replicaSet := data.Get("replica_set").(string)
	cfg := &ClientConfig{
		Host:               data.Get("host").(string),
		Port:               data.Get("port").(string),
		DB:                 data.Get("auth_database").(string),
		ReplicaSet:         replicaSet,
		Direct:             resolveDirectMode(replicaSet),
		Ssl:                data.Get("ssl").(bool),
		InsecureSkipVerify: data.Get("insecure_skip_verify").(bool),
	}
	if v, ok := data.GetOk("certificate"); ok {
		cfg.Certificate = v.(string)
	}
	return &MongoDatabaseConfiguration{
		Config:          cfg,
		MaxConnLifetime: 10,
	}
}

// buildOriginalUserAuthConfig builds a ClientConfig that includes the resource's
// username/password for authenticated connections (used by Read/Update/Delete
// after the user has been created).
func buildOriginalUserAuthConfig(data *schema.ResourceData) *MongoDatabaseConfiguration {
	cfg := buildOriginalUserConfig(data)
	cfg.Config.Username = data.Get("username").(string)
	cfg.Config.Password = data.Get("password").(string)
	return cfg
}

// probeReplicaSet connects direct (no auth) to the specified host:port, runs
// isMaster, and returns the discovered replica set name (empty if standalone).
// The probe client is disconnected before returning.
func probeReplicaSet(ctx context.Context, data *schema.ResourceData) string {
	probeCfg := &ClientConfig{
		Host:               data.Get("host").(string),
		Port:               data.Get("port").(string),
		DB:                 data.Get("auth_database").(string),
		Direct:             true,
		Ssl:                data.Get("ssl").(bool),
		InsecureSkipVerify: data.Get("insecure_skip_verify").(bool),
	}
	if v, ok := data.GetOk("certificate"); ok {
		probeCfg.Certificate = v.(string)
	}
	probeConf := &MongoDatabaseConfiguration{Config: probeCfg, MaxConnLifetime: 10}

	probeClient, err := MongoClientInitNoAuth(ctx, probeConf)
	if err != nil {
		return ""
	}
	defer func() { _ = probeClient.Disconnect(ctx) }()

	var resp IsMasterResp
	result := probeClient.Database("admin").RunCommand(
		context.Background(), bson.D{{Key: "isMaster", Value: 1}},
	)
	if err := result.Decode(&resp); err == nil && resp.SetName != "" {
		return resp.SetName
	}
	return ""
}

// ensureReplicaSetDiscovered ensures the config has the replica set name,
// probing the server if needed. Updates both the config and schema data.
func ensureReplicaSetDiscovered(ctx context.Context, conf *MongoDatabaseConfiguration, data *schema.ResourceData) {
	if conf.Config.ReplicaSet != "" {
		return
	}
	rsName := probeReplicaSet(ctx, data)
	if rsName != "" {
		conf.Config.ReplicaSet = rsName
		conf.Config.Direct = false
		_ = data.Set("replica_set", rsName)
	}
}

// ORIG-002: WHEN creating the original user, the resource SHALL connect
// without authentication and create the specified user with the given roles.
// ORIG-007: WHEN replica_set is not specified, the resource SHALL auto-discover
// the replica set name via isMaster and reconnect in discovery mode to route
// the createUser write to the primary.
func resourceOriginalUserCreate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	conf := buildOriginalUserConfig(data)
	ensureReplicaSetDiscovered(ctx, conf, data)

	client, err := MongoClientInitNoAuth(ctx, conf)
	if err != nil {
		return diag.Errorf("error connecting to MongoDB without auth: %s", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	database := data.Get("auth_database").(string)
	userName := data.Get("username").(string)
	userPassword := data.Get("password").(string)

	var roleList []Role
	roles := data.Get("role").(*schema.Set).List()
	if err := mapstructure.Decode(roles, &roleList); err != nil {
		return diag.Errorf("error decoding roles: %s", err)
	}

	// Default to root role if none specified
	if len(roleList) == 0 {
		roleList = []Role{{Role: "root", Db: defaultAuthDatabase}}
	}

	user := DbUser{Name: userName, Password: userPassword}
	if err := createUser(client, user, roleList, database); err != nil {
		// ORIG-003: WHEN createUser fails with "already exists", the resource
		// SHALL verify the user via an authenticated connection and adopt it.
		if isUserAlreadyExistsError(err) {
			return resourceOriginalUserAdopt(ctx, data)
		}
		return diag.Errorf("could not create the original user: %s", err)
	}

	id := database + "." + userName
	data.SetId(base64.StdEncoding.EncodeToString([]byte(id)))
	return resourceOriginalUserRead(ctx, data, i)
}

// ORIG-004: WHEN reading the original user, the resource SHALL connect with
// authentication and verify the user exists.
func resourceOriginalUserRead(ctx context.Context, data *schema.ResourceData, _ interface{}) diag.Diagnostics {
	conf := buildOriginalUserAuthConfig(data)
	ensureReplicaSetDiscovered(ctx, conf, data)

	client, err := MongoClientInit(ctx, conf)
	if err != nil {
		// If auth fails, try no-auth (server may have been reset)
		noAuthConf := buildOriginalUserConfig(data)
		ensureReplicaSetDiscovered(ctx, noAuthConf, data)
		client, err = MongoClientInitNoAuth(ctx, noAuthConf)
		if err != nil {
			return diag.Errorf("error connecting to MongoDB: %s", err)
		}
	}
	defer func() { _ = client.Disconnect(ctx) }()

	username, database, parseErr := resourceOriginalUserParseId(data.Id())
	if parseErr != nil {
		return diag.Errorf("%s", parseErr)
	}

	result, decodeErr := getUser(client, username, database)
	if decodeErr != nil {
		return diag.Errorf("error reading user: %s", decodeErr)
	}
	if len(result.Users) == 0 {
		data.SetId("")
		return nil
	}

	roles := make([]interface{}, len(result.Users[0].Roles))
	for i, r := range result.Users[0].Roles {
		roles[i] = map[string]interface{}{
			"db":   r.Db,
			"role": r.Role,
		}
	}

	if err := data.Set("role", roles); err != nil {
		return diag.Errorf("error setting role: %s", err)
	}
	if err := data.Set("auth_database", database); err != nil {
		return diag.Errorf("error setting auth_database: %s", err)
	}

	return nil
}

// ORIG-005: WHEN updating the original user, the resource SHALL connect with
// authentication and recreate the user with new credentials/roles.
func resourceOriginalUserUpdate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	conf := buildOriginalUserAuthConfig(data)
	ensureReplicaSetDiscovered(ctx, conf, data)

	client, err := MongoClientInit(ctx, conf)
	if err != nil {
		return diag.Errorf("error connecting to MongoDB: %s", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	username, database, parseErr := resourceOriginalUserParseId(data.Id())
	if parseErr != nil {
		return diag.Errorf("%s", parseErr)
	}

	adminDB := client.Database(database)
	result := adminDB.RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: username}})
	if result.Err() != nil {
		return diag.Errorf("error dropping user for update: %s", result.Err())
	}

	userPassword := data.Get("password").(string)
	var roleList []Role
	roles := data.Get("role").(*schema.Set).List()
	if err := mapstructure.Decode(roles, &roleList); err != nil {
		return diag.Errorf("error decoding roles: %s", err)
	}
	if len(roleList) == 0 {
		roleList = []Role{{Role: "root", Db: defaultAuthDatabase}}
	}

	user := DbUser{Name: username, Password: userPassword}
	if err := createUser(client, user, roleList, database); err != nil {
		return diag.Errorf("could not recreate user: %s", err)
	}

	newId := database + "." + username
	data.SetId(base64.StdEncoding.EncodeToString([]byte(newId)))
	return resourceOriginalUserRead(ctx, data, i)
}

// ORIG-006: WHEN deleting the original user, the resource SHALL connect with
// authentication and drop the user.
func resourceOriginalUserDelete(ctx context.Context, data *schema.ResourceData, _ interface{}) diag.Diagnostics {
	conf := buildOriginalUserAuthConfig(data)
	ensureReplicaSetDiscovered(ctx, conf, data)

	client, err := MongoClientInit(ctx, conf)
	if err != nil {
		return diag.Errorf("error connecting to MongoDB: %s", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	username, database, parseErr := resourceOriginalUserParseId(data.Id())
	if parseErr != nil {
		return diag.Errorf("%s", parseErr)
	}

	adminDB := client.Database(database)
	result := adminDB.RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: username}})
	if result.Err() != nil {
		return diag.Errorf("error dropping user: %s", result.Err())
	}

	return nil
}

// resourceOriginalUserAdopt verifies the user already exists via auth and
// adopts it into Terraform state.
func resourceOriginalUserAdopt(ctx context.Context, data *schema.ResourceData) diag.Diagnostics {
	conf := buildOriginalUserAuthConfig(data)
	ensureReplicaSetDiscovered(ctx, conf, data)

	client, err := MongoClientInit(ctx, conf)
	if err != nil {
		return diag.Errorf("user already exists but cannot authenticate with provided credentials: %s", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	database := data.Get("auth_database").(string)
	userName := data.Get("username").(string)

	_, getErr := getUser(client, userName, database)
	if getErr != nil {
		return diag.Errorf("user already exists but cannot be read: %s", getErr)
	}

	id := database + "." + userName
	data.SetId(base64.StdEncoding.EncodeToString([]byte(id)))
	return resourceOriginalUserRead(ctx, data, nil)
}

func resourceOriginalUserParseId(id string) (string, string, error) {
	result, err := base64.StdEncoding.DecodeString(id)
	if err != nil {
		return "", "", fmt.Errorf("unexpected format of ID: %s", err)
	}
	parts := strings.SplitN(string(result), ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("unexpected format of ID (%s), expected database.username", id)
	}
	return parts[1], parts[0], nil
}

func isUserAlreadyExistsError(err error) bool {
	return strings.Contains(err.Error(), "already exists")
}

// resolveDirectMode returns true (direct connection) when no replica set is
// specified, and false (discovery mode) when a replica set name is provided.
func resolveDirectMode(replicaSet string) bool {
	return replicaSet == ""
}

// discoverReplicaSet determines the replica set name and direct mode.
// If an explicit replica set name is provided, it takes precedence.
// If a client is provided and no explicit name is given, it queries the
// server via isMaster to discover the replica set name.
// Returns (replicaSetName, direct).
func discoverReplicaSet(explicit string, client interface{ Database(string) isMasterRunner }) (string, bool) {
	if explicit != "" {
		return explicit, false
	}
	if client != nil {
		resp, err := getIsMaster(client)
		if err == nil && resp.SetName != "" {
			return resp.SetName, false
		}
	}
	return "", true
}

// isMasterRunner is satisfied by *mongo.Database.
type isMasterRunner interface {
	RunCommand(ctx context.Context, runCommand interface{}, opts ...interface{}) singleResultDecoder
}

// singleResultDecoder is satisfied by *mongo.SingleResult.
type singleResultDecoder interface {
	Decode(v interface{}) error
}

// getIsMaster runs the isMaster command against the admin database and
// returns the response including the setName if the node is part of a
// replica set.
func getIsMaster(client interface{ Database(string) isMasterRunner }) (IsMasterResp, error) {
	var resp IsMasterResp
	result := client.Database("admin").RunCommand(context.Background(), bson.D{{Key: "isMaster", Value: 1}})
	if err := result.Decode(&resp); err != nil {
		return resp, err
	}
	return resp, nil
}
