package mongodb

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"go.mongodb.org/mongo-driver/bson"
)

// ProfileResponse represents the response from the MongoDB profile command.
// PROF-006
type ProfileResponse struct {
	Was       int `bson:"was"`
	Slowms    int `bson:"slowms"`
	Ratelimit int `bson:"ratelimit,omitempty"`
	OK        int `bson:"ok"`
}

// PROF-001
func resourceProfiler() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceProfilerCreate,
		ReadContext:   resourceProfilerRead,
		UpdateContext: resourceProfilerUpdate,
		DeleteContext: resourceProfilerDelete,
		// DANGER-015: block database identity changes at plan time
		CustomizeDiff: customdiff.All(
			blockFieldChange("database"),
		),
		Schema: map[string]*schema.Schema{
			"database": {
				Type:     schema.TypeString,
				Required: true,
			},
			"level": {
				Type:     schema.TypeInt,
				Required: true,
				// PROF-002
				ValidateFunc: func(val interface{}, key string) ([]string, []error) {
					v := val.(int)
					if v < 0 || v > 2 {
						return nil, []error{fmt.Errorf("%q must be between 0 and 2, got: %d", key, v)}
					}
					return nil, nil
				},
			},
			"slowms": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  100,
				// PROF-003
				ValidateFunc: func(val interface{}, key string) ([]string, []error) {
					v := val.(int)
					if v < 0 {
						return nil, []error{fmt.Errorf("%q must be >= 0, got: %d", key, v)}
					}
					return nil, nil
				},
			},
			"ratelimit": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1,
			},
		},
	}
}

// PROF-005
func resourceProfilerCreate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	config := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, config)
	if err != nil {
		return diag.Errorf("Error connecting to database: %s", err)
	}

	database := data.Get("database").(string)
	level := data.Get("level").(int)
	slowms := data.Get("slowms").(int)
	ratelimit := data.Get("ratelimit").(int)

	cmd := bson.D{
		{Key: "profile", Value: level},
		{Key: "slowms", Value: slowms},
		{Key: "ratelimit", Value: ratelimit},
	}

	res := client.Database(database).RunCommand(ctx, cmd)
	if res.Err() != nil {
		// PROF-009
		return diag.Errorf("profile command on %q: %s", database, res.Err())
	}

	// PROF-004
	data.SetId(formatResourceId(database, "profiler"))
	return resourceProfilerRead(ctx, data, i)
}

// PROF-006
func resourceProfilerRead(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	config := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, config)
	if err != nil {
		return diag.Errorf("Error connecting to database: %s", err)
	}

	_, database, parseErr := parseResourceId(data.Id())
	if parseErr != nil {
		return diag.FromErr(parseErr)
	}

	cmd := bson.D{{Key: "profile", Value: -1}}
	res := client.Database(database).RunCommand(ctx, cmd)
	if res.Err() != nil {
		// PROF-009
		return diag.Errorf("profile read on %q: %s", database, res.Err())
	}

	var resp ProfileResponse
	if err := res.Decode(&resp); err != nil {
		return diag.Errorf("profile decode on %q: %s", database, err)
	}

	if err := data.Set("database", database); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("level", resp.Was); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("slowms", resp.Slowms); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("ratelimit", resp.Ratelimit); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

// PROF-007
func resourceProfilerUpdate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	return resourceProfilerCreate(ctx, data, i)
}

// PROF-008
func resourceProfilerDelete(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	config := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, config)
	if err != nil {
		return diag.Errorf("Error connecting to database: %s", err)
	}

	_, database, parseErr := parseResourceId(data.Id())
	if parseErr != nil {
		return diag.FromErr(parseErr)
	}

	cmd := bson.D{{Key: "profile", Value: 0}}
	res := client.Database(database).RunCommand(ctx, cmd)
	if res.Err() != nil {
		return diag.Errorf("profile disable on %q: %s", database, res.Err())
	}

	data.SetId("")
	return nil
}
