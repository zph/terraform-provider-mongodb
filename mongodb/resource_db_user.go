package mongodb

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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
		Schema: map[string]*schema.Schema{
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
	var stateId = data.State().ID
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

func resourceDatabaseUserUpdate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	var config = i.(*MongoDatabaseConfiguration)
	client, connectionError := MongoClientInit(ctx, config)
	if connectionError != nil {
		return diag.Errorf("Error connecting to database : %s ", connectionError)
	}
	var userName = data.Get("name").(string)
	var database = data.Get("auth_database").(string)
	var userPassword = data.Get("password").(string)

	adminDB := client.Database(database)

	result := adminDB.RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: userName}})
	if result.Err() != nil {
		return diag.Errorf("%s", result.Err())
	}
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
	err2 := createUser(client, user, roleList, database)
	if err2 != nil {
		return diag.Errorf("Could not create the user : %s ", err2)
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
	stateID := data.State().ID
	username, database, err := resourceDatabaseUserParseId(stateID)
	if err != nil {
		return diag.Errorf("%s", err)
	}
	result, decodeError := getUser(client, username, database)
	if decodeError != nil {
		return diag.Errorf("Error decoding user : %s ", decodeError)
	}
	if len(result.Users) == 0 {
		return diag.Errorf("user does not exist")
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
	dataSetError = data.Set("password", data.Get("password"))
	if dataSetError != nil {
		return diag.Errorf("error setting password : %s ", dataSetError)
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
