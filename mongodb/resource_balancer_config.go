package mongodb

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	balancerResourceID  = "balancer"
	balancerSettingsID  = "balancer"
	chunksizeSettingsID = "chunksize"
	minChunkSizeMB      = 1
	maxChunkSizeMB      = 1024
	balancerModeFull    = "full"
)

var hhmmRegex = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

// validateHHMM checks that a string matches HH:MM 24-hour format.
// BAL-011
func validateHHMM(val interface{}, key string) ([]string, []error) {
	v := val.(string)
	if !hhmmRegex.MatchString(v) {
		return nil, []error{fmt.Errorf("%q must be in HH:MM format (24-hour), got: %q", key, v)}
	}
	return nil, nil
}

// BAL-001
func resourceBalancerConfig() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceBalancerConfigCreate,
		ReadContext:   resourceBalancerConfigRead,
		UpdateContext: resourceBalancerConfigUpdate,
		DeleteContext: resourceBalancerConfigDelete,
		// BAL-010
		CustomizeDiff: customdiff.All(
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				start := d.Get("active_window_start").(string)
				stop := d.Get("active_window_stop").(string)
				if (start == "") != (stop == "") {
					return fmt.Errorf("active_window_start and active_window_stop must both be set or both be unset")
				}
				return nil
			},
		),
		Schema: map[string]*schema.Schema{
			"enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true, // BAL-002
			},
			"active_window_start": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validateHHMM, // BAL-011
			},
			"active_window_stop": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validateHHMM, // BAL-011
			},
			"chunk_size_mb": {
				Type:     schema.TypeInt,
				Optional: true,
				// BAL-015
				ValidateFunc: func(val interface{}, key string) ([]string, []error) {
					v := val.(int)
					if v < minChunkSizeMB || v > maxChunkSizeMB {
						return nil, []error{fmt.Errorf("%q must be between %d and %d, got: %d", key, minChunkSizeMB, maxChunkSizeMB, v)}
					}
					return nil, nil
				},
			},
			"secondary_throttle": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"wait_for_delete": {
				Type:     schema.TypeBool,
				Optional: true,
			},
		},
	}
}

// BAL-003, BAL-004, BAL-005, BAL-006, BAL-007
func resourceBalancerConfigCreate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	config := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, config)
	if err != nil {
		return diag.Errorf("Error connecting to database: %s", err)
	}

	// BAL-012
	if diags := requireMongos(ctx, client); diags.HasError() {
		return diags
	}

	// BAL-003: enable/disable balancer
	enabled := data.Get("enabled").(bool)
	if err := setBalancerEnabled(ctx, client, enabled); err != nil {
		return diag.Errorf("balancer enable/disable: %s", err)
	}

	// BAL-004: active window
	start := data.Get("active_window_start").(string)
	stop := data.Get("active_window_stop").(string)
	if err := setBalancerWindow(ctx, client, start, stop); err != nil {
		return diag.Errorf("balancer active window: %s", err)
	}

	// BAL-006: secondary throttle
	if v, ok := data.GetOk("secondary_throttle"); ok {
		if err := setBalancerField(ctx, client, "_secondaryThrottle", v.(string)); err != nil {
			return diag.Errorf("balancer secondary_throttle: %s", err)
		}
	}

	// BAL-007: wait for delete
	if v, ok := data.GetOk("wait_for_delete"); ok {
		if err := setBalancerField(ctx, client, "_waitForDelete", v.(bool)); err != nil {
			return diag.Errorf("balancer wait_for_delete: %s", err)
		}
	}

	// BAL-005: chunk size
	if v, ok := data.GetOk("chunk_size_mb"); ok {
		if err := setChunkSize(ctx, client, v.(int)); err != nil {
			return diag.Errorf("balancer chunk_size_mb: %s", err)
		}
	}

	// BAL-013
	data.SetId(balancerResourceID)
	return resourceBalancerConfigRead(ctx, data, i)
}

// BAL-008
func resourceBalancerConfigRead(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	config := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, config)
	if err != nil {
		return diag.Errorf("Error connecting to database: %s", err)
	}

	// Read balancer status for enabled
	var statusResp BalancerStatus
	res := client.Database("admin").RunCommand(ctx, bson.D{{Key: "balancerStatus", Value: 1}})
	if res.Err() != nil {
		return diag.Errorf("balancerStatus: %s", res.Err())
	}
	if err := res.Decode(&statusResp); err != nil {
		return diag.Errorf("balancerStatus decode: %s", err)
	}
	if err := data.Set("enabled", statusResp.Mode == balancerModeFull); err != nil {
		return diag.FromErr(err)
	}

	// Read balancer settings doc
	configDB := client.Database("config")
	settingsColl := configDB.Collection("settings")

	var balancerDoc bson.M
	err = settingsColl.FindOne(ctx, bson.M{"_id": balancerSettingsID}).Decode(&balancerDoc)
	if err != nil && err != mongo.ErrNoDocuments {
		return diag.Errorf("config.settings balancer read: %s", err)
	}

	if balancerDoc != nil {
		// Active window
		if aw, ok := balancerDoc["activeWindow"].(bson.M); ok {
			if start, ok := aw["start"].(string); ok {
				if err := data.Set("active_window_start", start); err != nil {
					return diag.FromErr(err)
				}
			}
			if stop, ok := aw["stop"].(string); ok {
				if err := data.Set("active_window_stop", stop); err != nil {
					return diag.FromErr(err)
				}
			}
		} else {
			if err := data.Set("active_window_start", ""); err != nil {
				return diag.FromErr(err)
			}
			if err := data.Set("active_window_stop", ""); err != nil {
				return diag.FromErr(err)
			}
		}

		// Secondary throttle
		if st, ok := balancerDoc["_secondaryThrottle"]; ok {
			if err := data.Set("secondary_throttle", fmt.Sprintf("%v", st)); err != nil {
				return diag.FromErr(err)
			}
		} else {
			if err := data.Set("secondary_throttle", ""); err != nil {
				return diag.FromErr(err)
			}
		}

		// Wait for delete
		if wfd, ok := balancerDoc["_waitForDelete"].(bool); ok {
			if err := data.Set("wait_for_delete", wfd); err != nil {
				return diag.FromErr(err)
			}
		}
	}

	// Read chunk size doc
	var chunksizeDoc bson.M
	err = settingsColl.FindOne(ctx, bson.M{"_id": chunksizeSettingsID}).Decode(&chunksizeDoc)
	if err != nil && err != mongo.ErrNoDocuments {
		return diag.Errorf("config.settings chunksize read: %s", err)
	}
	if chunksizeDoc != nil {
		if v, ok := chunksizeDoc["value"]; ok {
			// MongoDB may store as int32 or int64
			switch tv := v.(type) {
			case int32:
				if err := data.Set("chunk_size_mb", int(tv)); err != nil {
					return diag.FromErr(err)
				}
			case int64:
				if err := data.Set("chunk_size_mb", int(tv)); err != nil {
					return diag.FromErr(err)
				}
			case float64:
				if err := data.Set("chunk_size_mb", int(tv)); err != nil {
					return diag.FromErr(err)
				}
			}
		}
	}

	return nil
}

func resourceBalancerConfigUpdate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	return resourceBalancerConfigCreate(ctx, data, i)
}

// BAL-009
func resourceBalancerConfigDelete(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	config := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, config)
	if err != nil {
		return diag.Errorf("Error connecting to database: %s", err)
	}

	// Re-enable balancer
	if err := setBalancerEnabled(ctx, client, true); err != nil {
		return diag.Errorf("balancer re-enable on delete: %s", err)
	}

	// Unset active window, secondary throttle, wait for delete
	configDB := client.Database("config")
	settingsColl := configDB.Collection("settings")
	_, err = settingsColl.UpdateOne(ctx,
		bson.M{"_id": balancerSettingsID},
		bson.M{"$unset": bson.M{
			"activeWindow":       "",
			"_secondaryThrottle": "",
			"_waitForDelete":     "",
		}},
	)
	if err != nil {
		return diag.Errorf("balancer settings cleanup: %s", err)
	}

	// Delete chunksize doc
	_, err = settingsColl.DeleteOne(ctx, bson.M{"_id": chunksizeSettingsID})
	if err != nil {
		return diag.Errorf("chunksize cleanup: %s", err)
	}

	data.SetId("")
	return nil
}

// setBalancerEnabled runs balancerStart or balancerStop.
func setBalancerEnabled(ctx context.Context, client *mongo.Client, enabled bool) error {
	cmd := "balancerStart"
	if !enabled {
		cmd = "balancerStop"
	}
	res := client.Database("admin").RunCommand(ctx, bson.D{{Key: cmd, Value: 1}})
	if res.Err() != nil {
		return fmt.Errorf("%s: %w", cmd, res.Err())
	}
	return nil
}

// setBalancerWindow writes or unsets the activeWindow in config.settings.
func setBalancerWindow(ctx context.Context, client *mongo.Client, start, stop string) error {
	settingsColl := client.Database("config").Collection("settings")
	if start != "" && stop != "" {
		_, err := settingsColl.UpdateOne(ctx,
			bson.M{"_id": balancerSettingsID},
			bson.M{"$set": bson.M{
				"activeWindow": bson.M{"start": start, "stop": stop},
			}},
			options.Update().SetUpsert(true),
		)
		return err
	}
	// Unset if both empty
	_, err := settingsColl.UpdateOne(ctx,
		bson.M{"_id": balancerSettingsID},
		bson.M{"$unset": bson.M{"activeWindow": ""}},
	)
	// Ignore "no documents matched" — that's fine
	return err
}

// setBalancerField upserts a single field on the balancer settings doc.
func setBalancerField(ctx context.Context, client *mongo.Client, field string, value interface{}) error {
	settingsColl := client.Database("config").Collection("settings")
	_, err := settingsColl.UpdateOne(ctx,
		bson.M{"_id": balancerSettingsID},
		bson.M{"$set": bson.M{field: value}},
		options.Update().SetUpsert(true),
	)
	return err
}

// setChunkSize upserts the chunksize document in config.settings.
func setChunkSize(ctx context.Context, client *mongo.Client, sizeMB int) error {
	settingsColl := client.Database("config").Collection("settings")
	_, err := settingsColl.UpdateOne(ctx,
		bson.M{"_id": chunksizeSettingsID},
		bson.M{"$set": bson.M{"value": sizeMB}},
		options.Update().SetUpsert(true),
	)
	return err
}
