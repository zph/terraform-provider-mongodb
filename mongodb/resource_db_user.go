package mongodb

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/mitchellh/mapstructure"
	"go.mongodb.org/mongo-driver/bson"
)

func resourceDatabaseUser() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceDatabaseUserCreate,
		ReadContext:   resourceDatabaseUserRead,
		UpdateContext: resourceDatabaseUserUpdate,
		DeleteContext: resourceDatabaseUserDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		// PREVIEW-016, PREVIEW-017: command preview
		CustomizeDiff: customdiff.All(
			previewCommands(dbUserCommandPreview),
		),
		Schema: map[string]*schema.Schema{
			"planned_commands": commandPreviewSchema(), // PREVIEW-005
			"auth_database": {
				Type:     schema.TypeString,
				Required: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"password": {
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: true,
				// DANGER-023: an empty password is never valid and must fail
				// at plan time, not be silently skipped at apply time.
				ValidateDiagFunc: validateDiagFunc(validation.StringIsNotEmpty),
			},
			"role": {
				Type:     schema.TypeSet,
				Optional: true,
				MaxItems: 25,
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
			},
		},
	}
}

func resourceDatabaseUserDelete(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	var config = i.(*MongoDatabaseConfiguration)
	client, connectionError := MongoClientInit(ctx, config)
	if connectionError != nil {
		return diag.Errorf("Error connecting to database : %s ", connectionError)
	}
	var stateId = data.Id()
	var database = data.Get("auth_database").(string)

	userName, _, err := parseResourceId(stateId)
	if err != nil {
		return diag.Errorf("ID mismatch %s", err)
	}

	adminDB := client.Database(database)

	result := adminDB.RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: userName}})
	if result.Err() != nil {
		return diag.Errorf("%s", result.Err())
	}

	return nil
}

// resourceChangeChecker is the slice of ResourceData needed to decide
// whether an update carries a password.
type resourceChangeChecker interface {
	HasChange(string) bool
}

// includePasswordForUpdate reports whether the updateUser command should
// carry pwd: only when the planned password changed to a non-empty value.
// A change to an empty value is an error, never a silent skip.
// DANGER-021, DANGER-023
func includePasswordForUpdate(d resourceChangeChecker, password string) (bool, error) {
	if !d.HasChange("password") {
		return false, nil
	}
	if password == "" {
		return false, fmt.Errorf("password cannot be changed to an empty value")
	}
	return true, nil
}

// DANGER-001: uses updateUser for in-place modification instead of drop+recreate
func resourceDatabaseUserUpdate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	var config = i.(*MongoDatabaseConfiguration)
	client, connectionError := MongoClientInit(ctx, config)
	if connectionError != nil {
		return diag.Errorf("Error connecting to database : %s ", connectionError)
	}
	var userName = data.Get("name").(string)
	var database = data.Get("auth_database").(string)
	var userPassword = data.Get("password").(string)

	var roleList []Role
	roles := data.Get("role").(*schema.Set).List()
	roleMapErr := mapstructure.Decode(roles, &roleList)
	if roleMapErr != nil {
		return diag.Errorf("Error decoding map : %s ", roleMapErr)
	}

	includePassword, pwdErr := includePasswordForUpdate(data, userPassword)
	if pwdErr != nil {
		return diag.FromErr(pwdErr)
	}
	if err := updateUser(client, userName, userPassword, roleList, database, includePassword); err != nil {
		return diag.Errorf("Could not update the user : %s ", err)
	}

	data.SetId(formatResourceId(database, userName))
	return resourceDatabaseUserRead(ctx, data, i)
}

func resourceDatabaseUserRead(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	var config = i.(*MongoDatabaseConfiguration)
	client, connectionError := MongoClientInit(ctx, config)
	if connectionError != nil {
		return diag.Errorf("Error connecting to database : %s ", connectionError)
	}
	stateID := data.Id()
	username, database, err := resourceDatabaseUserParseId(stateID)
	if err != nil {
		return diag.Errorf("%s", err)
	}
	result, decodeError := getUser(client, username, database)
	if decodeError != nil {
		return diag.Errorf("Error decoding user : %s ", decodeError)
	}
	// DANGER-024: a user dropped out-of-band clears from state (with a
	// warning) instead of wedging every refresh; the next plan proposes
	// recreation. During the create read-back the same miss is an
	// inconsistency, not drift — fail loudly rather than orphan the
	// just-created user out of state.
	if len(result.Users) == 0 {
		if data.IsNewResource() {
			return diag.Errorf("user %s not found in database %s during post-create read-back", username, database)
		}
		tflog.Warn(ctx, "user not found in MongoDB; removing from state", map[string]interface{}{
			"user":     username,
			"database": database,
		})
		data.SetId("")
		return nil
	}
	roles := make([]interface{}, len(result.Users[0].Roles))

	for i, s := range result.Users[0].Roles {
		roles[i] = map[string]interface{}{
			"db":   s.Db,
			"role": s.Role,
		}
	}
	dataSetError := data.Set("role", roles)
	if dataSetError != nil {
		return diag.Errorf("error setting role : %s ", dataSetError)
	}
	dataSetError = data.Set("auth_database", database)
	if dataSetError != nil {
		return diag.Errorf("error setting auth_db : %s ", dataSetError)
	}
	// DANGER-022: set name so import-created state is complete and a matching
	// configuration plans no changes.
	dataSetError = data.Set("name", username)
	if dataSetError != nil {
		return diag.Errorf("error setting name : %s ", dataSetError)
	}
	data.SetId(stateID)
	return nil
}

func resourceDatabaseUserCreate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	var config = i.(*MongoDatabaseConfiguration)
	client, connectionError := MongoClientInit(ctx, config)
	if connectionError != nil {
		return diag.Errorf("Error connecting to database : %s ", connectionError)
	}
	var database = data.Get("auth_database").(string)
	var userName = data.Get("name").(string)
	var userPassword = data.Get("password").(string)
	var roleList []Role
	var user = DbUser{
		Name:     userName,
		Password: userPassword,
	}
	roles := data.Get("role").(*schema.Set).List()
	roleMapErr := mapstructure.Decode(roles, &roleList)
	if roleMapErr != nil {
		return diag.Errorf("Error decoding map : %s ", roleMapErr)
	}
	err := createUser(client, user, roleList, database)
	if err != nil {
		return diag.Errorf("Could not create the user : %s ", err)
	}
	data.SetId(formatResourceId(database, userName))
	return resourceDatabaseUserRead(ctx, data, i)
}

// IDFORMAT-005
func resourceDatabaseUserParseId(id string) (string, string, error) {
	return parseResourceId(id)
}
