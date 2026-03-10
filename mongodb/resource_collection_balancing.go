package mongodb

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const fcvThresholdCollBalancing = 6

// parseFCVMajor extracts the major version number from an FCV string like "6.0".
func parseFCVMajor(fcv string) (int, error) {
	parts := strings.SplitN(fcv, ".", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid FCV format: %q", fcv)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid FCV major version: %q: %w", parts[0], err)
	}
	return major, nil
}

// CBAL-001
func resourceCollectionBalancing() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceCollectionBalancingCreate,
		ReadContext:   resourceCollectionBalancingRead,
		UpdateContext: resourceCollectionBalancingUpdate,
		DeleteContext: resourceCollectionBalancingDelete,
		// DANGER-014: block namespace identity changes at plan time
		CustomizeDiff: customdiff.All(
			blockFieldChange("namespace"),
		),
		Schema: map[string]*schema.Schema{
			"namespace": {
				Type:     schema.TypeString,
				Required: true,
				// CBAL-002
				ValidateFunc: func(val interface{}, key string) ([]string, []error) {
					v := val.(string)
					dotIdx := strings.Index(v, ".")
					if dotIdx < 1 || dotIdx == len(v)-1 {
						return nil, []error{fmt.Errorf("%q must be in db.collection format, got: %q", key, v)}
					}
					return nil, nil
				},
			},
			"enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true, // CBAL-001
			},
			"chunk_size_mb": {
				Type:     schema.TypeInt,
				Optional: true,
			},
		},
	}
}

// CBAL-003, CBAL-004
func resourceCollectionBalancingCreate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	config := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, config)
	if err != nil {
		return diag.Errorf("Error connecting to database: %s", err)
	}

	// CBAL-010
	if diags := requireMongos(ctx, client); diags.HasError() {
		return diags
	}

	namespace := data.Get("namespace").(string)
	enabled := data.Get("enabled").(bool)
	noBalance := !enabled

	fcv, err := GetFCV(ctx, client)
	if err != nil {
		return diag.Errorf("get FCV: %s", err)
	}

	major, err := parseFCVMajor(fcv)
	if err != nil {
		return diag.Errorf("parse FCV: %s", err)
	}

	var diags diag.Diagnostics

	if major >= fcvThresholdCollBalancing {
		// CBAL-003: configureCollectionBalancing command
		cmd := bson.D{
			{Key: "configureCollectionBalancing", Value: namespace},
			{Key: "enableBalancing", Value: enabled},
		}
		if v, ok := data.GetOk("chunk_size_mb"); ok {
			cmd = append(cmd, bson.E{Key: "chunkSize", Value: v.(int)})
		}
		res := client.Database("admin").RunCommand(ctx, cmd)
		if res.Err() != nil {
			return diag.Errorf("configureCollectionBalancing %q: %s", namespace, res.Err())
		}
	} else {
		// CBAL-004: direct config.collections write
		collsColl := client.Database("config").Collection("collections")
		_, err := collsColl.UpdateOne(ctx,
			bson.M{"_id": namespace},
			bson.M{"$set": bson.M{"noBalance": noBalance}},
		)
		if err != nil {
			return diag.Errorf("config.collections update %q: %s", namespace, err)
		}

		// CBAL-007: warn if chunk_size_mb set on < 6.0
		if _, ok := data.GetOk("chunk_size_mb"); ok {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Warning,
				Summary:  "chunk_size_mb ignored on MongoDB < 6.0",
				Detail:   fmt.Sprintf("Per-collection chunk size requires FCV 6.0+; current FCV is %s. The chunk_size_mb setting was ignored.", fcv),
			})
		}
	}

	// CBAL-009
	data.SetId(formatResourceId(namespace, "balancing"))
	readDiags := resourceCollectionBalancingRead(ctx, data, i)
	return append(diags, readDiags...)
}

// CBAL-005, CBAL-006
func resourceCollectionBalancingRead(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	config := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, config)
	if err != nil {
		return diag.Errorf("Error connecting to database: %s", err)
	}

	namespace := data.Get("namespace").(string)

	collsColl := client.Database("config").Collection("collections")
	var doc bson.M
	err = collsColl.FindOne(ctx, bson.M{"_id": namespace}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Collection not in config — resource gone
			data.SetId("")
			return nil
		}
		return diag.Errorf("config.collections read %q: %s", namespace, err)
	}

	// CBAL-006
	noBalance, _ := doc["noBalance"].(bool)
	if err := data.Set("enabled", !noBalance); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("namespace", namespace); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceCollectionBalancingUpdate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	return resourceCollectionBalancingCreate(ctx, data, i)
}

// CBAL-008
func resourceCollectionBalancingDelete(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	config := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, config)
	if err != nil {
		return diag.Errorf("Error connecting to database: %s", err)
	}

	namespace := data.Get("namespace").(string)

	fcv, err := GetFCV(ctx, client)
	if err != nil {
		return diag.Errorf("get FCV: %s", err)
	}

	major, err := parseFCVMajor(fcv)
	if err != nil {
		return diag.Errorf("parse FCV: %s", err)
	}

	if major >= fcvThresholdCollBalancing {
		cmd := bson.D{
			{Key: "configureCollectionBalancing", Value: namespace},
			{Key: "enableBalancing", Value: true},
			{Key: "chunkSize", Value: 0},
		}
		res := client.Database("admin").RunCommand(ctx, cmd)
		if res.Err() != nil {
			return diag.Errorf("configureCollectionBalancing delete %q: %s", namespace, res.Err())
		}
	} else {
		collsColl := client.Database("config").Collection("collections")
		_, err := collsColl.UpdateOne(ctx,
			bson.M{"_id": namespace},
			bson.M{"$unset": bson.M{"noBalance": ""}},
		)
		if err != nil {
			return diag.Errorf("config.collections unset noBalance %q: %s", namespace, err)
		}
	}

	data.SetId("")
	return nil
}
