package mongodb

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"go.mongodb.org/mongo-driver/bson"
)

// coerceParameterValue converts a string value for setParameter.
// PARAM-008: Order: bool > int > float > string.
func coerceParameterValue(s string) interface{} {
	// Bool
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}

	// Int
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v
	}

	// Float
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v
	}

	// String fallback
	return s
}

// PARAM-001
func resourceServerParameter() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceServerParameterCreate,
		ReadContext:   resourceServerParameterRead,
		UpdateContext: resourceServerParameterUpdate,
		DeleteContext: resourceServerParameterDelete,
		// GATE-005: require feature opt-in
		// DANGER-013: block parameter identity changes at plan time
		// PREVIEW-013: command preview
		CustomizeDiff: customdiff.All(
			requireFeature("mongodb_server_parameter"),
			blockFieldChange("parameter"),
			previewCommands(serverParameterCommandPreview),
		),
		Schema: map[string]*schema.Schema{
			"planned_commands": commandPreviewSchema(), // PREVIEW-005
			"parameter": {
				Type:     schema.TypeString,
				Required: true,
			},
			"value": {
				Type:     schema.TypeString,
				Required: true,
			},
			"ignore_read": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
		},
	}
}

// PARAM-002
func resourceServerParameterCreate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	config := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, config)
	if err != nil {
		return diag.Errorf("Error connecting to database: %s", err)
	}

	param := data.Get("parameter").(string)
	value := data.Get("value").(string)
	coerced := coerceParameterValue(value)

	cmd := bson.D{
		{Key: "setParameter", Value: 1},
		{Key: param, Value: coerced},
	}

	res := client.Database("admin").RunCommand(ctx, cmd)
	if res.Err() != nil {
		// PARAM-009
		return diag.Errorf("setParameter %q: %s", param, res.Err())
	}

	// PARAM-007
	data.SetId(formatResourceId("admin", param))
	return resourceServerParameterRead(ctx, data, i)
}

// PARAM-003, PARAM-004
func resourceServerParameterRead(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	param, _, parseErr := parseResourceId(data.Id())
	if parseErr != nil {
		return diag.FromErr(parseErr)
	}

	ignoreRead := data.Get("ignore_read").(bool)

	// PARAM-004: retain configured value, no MongoDB call
	if ignoreRead {
		return nil
	}

	config := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, config)
	if err != nil {
		return diag.Errorf("Error connecting to database: %s", err)
	}

	// PARAM-003
	cmd := bson.D{
		{Key: "getParameter", Value: 1},
		{Key: param, Value: 1},
	}

	res := client.Database("admin").RunCommand(ctx, cmd)
	if res.Err() != nil {
		// PARAM-010
		return diag.Errorf("getParameter %q: %s", param, res.Err())
	}

	var result bson.M
	if err := res.Decode(&result); err != nil {
		return diag.Errorf("getParameter %q decode: %s", param, err)
	}

	val, ok := result[param]
	if !ok {
		return diag.Errorf("getParameter %q: parameter not found in response", param)
	}

	// PARAM-012
	if err := data.Set("value", fmt.Sprintf("%v", val)); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("parameter", param); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

// PARAM-005
func resourceServerParameterUpdate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	return resourceServerParameterCreate(ctx, data, i)
}

// PARAM-006
func resourceServerParameterDelete(_ context.Context, data *schema.ResourceData, _ interface{}) diag.Diagnostics {
	data.SetId("")
	return nil
}
