package mongodb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const zoneKeyRangeIDSeparator = "::"

// ResourceZoneKeyRange manages zone key range assignments via updateZoneKeyRange.
type ResourceZoneKeyRange struct{}

var RZoneKeyRange = ResourceZoneKeyRange{}

// formatZoneKeyRangeID builds the resource ID from namespace, min, and max. // ZONE-024
func formatZoneKeyRangeID(namespace, minJSON, maxJSON string) string {
	minB64 := base64.StdEncoding.EncodeToString([]byte(minJSON))
	maxB64 := base64.StdEncoding.EncodeToString([]byte(maxJSON))
	return namespace + zoneKeyRangeIDSeparator + minB64 + zoneKeyRangeIDSeparator + maxB64
}

// parseZoneKeyRangeID splits a resource ID into namespace, min JSON, and max JSON. // ZONE-024
func parseZoneKeyRangeID(id string) (namespace, minJSON, maxJSON string, err error) {
	parts := strings.SplitN(id, zoneKeyRangeIDSeparator, 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid zone_key_range ID %q: expected namespace::base64(min)::base64(max)", id)
	}
	namespace = parts[0]
	if namespace == "" {
		return "", "", "", fmt.Errorf("invalid zone_key_range ID %q: empty namespace", id)
	}

	minBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", "", fmt.Errorf("invalid zone_key_range ID %q: bad base64 for min: %w", id, err)
	}
	maxBytes, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", "", "", fmt.Errorf("invalid zone_key_range ID %q: bad base64 for max: %w", id, err)
	}

	return namespace, string(minBytes), string(maxBytes), nil
}

// jsonToBsonD converts a JSON string to a bson.D for ordered document representation.
func jsonToBsonD(jsonStr string) (bson.D, error) {
	var doc bson.D
	if err := bson.UnmarshalExtJSON([]byte(jsonStr), false, &doc); err != nil {
		return nil, fmt.Errorf("invalid JSON for BSON conversion: %w", err)
	}
	return doc, nil
}

// validateJSON checks that a string is valid JSON. // ZONE-016
func validateJSON(val interface{}, key string) ([]string, []error) {
	v := val.(string)
	var js json.RawMessage
	if err := json.Unmarshal([]byte(v), &js); err != nil {
		return nil, []error{fmt.Errorf("%q must be valid JSON, got: %s", key, err)}
	}
	return nil, nil
}

// validateNamespaceDotFormat checks for db.collection format. // ZONE-015
func validateNamespaceDotFormat(val interface{}, key string) ([]string, []error) {
	v := val.(string)
	dotIdx := strings.Index(v, ".")
	if dotIdx < 1 || dotIdx == len(v)-1 {
		return nil, []error{fmt.Errorf("%q must be in db.collection format, got: %q", key, v)}
	}
	return nil, nil
}

// ZONE-014
func resourceZoneKeyRange() *schema.Resource {
	return &schema.Resource{
		CreateContext: RZoneKeyRange.Create,
		ReadContext:   RZoneKeyRange.Read,
		UpdateContext: RZoneKeyRange.Update,
		DeleteContext: RZoneKeyRange.Delete,
		Importer: &schema.ResourceImporter{
			StateContext: zoneKeyRangeImportState,
		},
		// ZONE-025 through ZONE-029, ZONE-030
		CustomizeDiff: customdiff.All(
			requireFeature("mongodb_zone_key_range"),
			blockFieldChange("namespace"),
			blockFieldChange("zone"),
			blockFieldChange("min"),
			blockFieldChange("max"),
			previewCommands(zoneKeyRangeCommandPreview),
		),
		Schema: map[string]*schema.Schema{
			"planned_commands": commandPreviewSchema(), // PREVIEW-005
			"namespace": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validateNamespaceDotFormat, // ZONE-015
				Description:  "The namespace in db.collection format.",
			},
			"zone": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The zone name to assign the key range to.",
			},
			"min": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validateJSON, // ZONE-016
				Description:  "Lower bound of the shard key range (inclusive), as a JSON string.",
			},
			"max": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validateJSON, // ZONE-016
				Description:  "Upper bound of the shard key range (exclusive), as a JSON string.",
			},
		},
	}
}

// zoneKeyRangeImportState parses the import ID into state fields.
func zoneKeyRangeImportState(_ context.Context, d *schema.ResourceData, _ interface{}) ([]*schema.ResourceData, error) {
	namespace, minJSON, maxJSON, err := parseZoneKeyRangeID(d.Id())
	if err != nil {
		return nil, err
	}
	if err := d.Set("namespace", namespace); err != nil {
		return nil, err
	}
	if err := d.Set("min", minJSON); err != nil {
		return nil, err
	}
	if err := d.Set("max", maxJSON); err != nil {
		return nil, err
	}
	// zone is read back via Read
	return []*schema.ResourceData{d}, nil
}

// Create runs updateZoneKeyRange to assign a key range to a zone. // ZONE-017, ZONE-018
func (r *ResourceZoneKeyRange) Create(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	providerConf := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, providerConf)
	if err != nil {
		return diag.Errorf("error connecting to MongoDB: %s", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	// ZONE-023
	if diags := requireMongos(ctx, client); diags.HasError() {
		return diags
	}

	namespace := data.Get("namespace").(string)
	zone := data.Get("zone").(string)
	minJSON := data.Get("min").(string)
	maxJSON := data.Get("max").(string)

	minDoc, err := jsonToBsonD(minJSON)
	if err != nil {
		return diag.Errorf("invalid min: %s", err)
	}
	maxDoc, err := jsonToBsonD(maxJSON)
	if err != nil {
		return diag.Errorf("invalid max: %s", err)
	}

	tflog.Info(ctx, "creating zone key range", map[string]interface{}{
		"namespace": namespace,
		"zone":      zone,
		"min":       minJSON,
		"max":       maxJSON,
	})

	// ZONE-017
	res := client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "updateZoneKeyRange", Value: namespace},
		{Key: "min", Value: minDoc},
		{Key: "max", Value: maxDoc},
		{Key: "zone", Value: zone},
	})
	if res.Err() != nil {
		return diag.FromErr(fmt.Errorf("updateZoneKeyRange failed: %w", res.Err()))
	}

	var resp OKResponse
	if err := res.Decode(&resp); err != nil {
		return diag.FromErr(fmt.Errorf("updateZoneKeyRange decode: %w", err))
	}
	if resp.OK != 1 {
		return diag.Errorf("updateZoneKeyRange failed: %s", resp.Errmsg)
	}

	// ZONE-024
	data.SetId(formatZoneKeyRangeID(namespace, minJSON, maxJSON))

	// ZONE-018
	return r.Read(ctx, data, i)
}

// Read queries config.tags to verify the zone key range exists. // ZONE-019, ZONE-020
func (r *ResourceZoneKeyRange) Read(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	providerConf := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, providerConf)
	if err != nil {
		return diag.Errorf("error connecting to MongoDB: %s", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	namespace := data.Get("namespace").(string)
	minJSON := data.Get("min").(string)
	maxJSON := data.Get("max").(string)

	// Fallback to ID parsing if fields are empty (e.g. import)
	if namespace == "" || minJSON == "" || maxJSON == "" {
		namespace, minJSON, maxJSON, err = parseZoneKeyRangeID(data.Id())
		if err != nil {
			return diag.FromErr(err)
		}
	}

	minDoc, err := jsonToBsonD(minJSON)
	if err != nil {
		return diag.Errorf("invalid min: %s", err)
	}
	maxDoc, err := jsonToBsonD(maxJSON)
	if err != nil {
		return diag.Errorf("invalid max: %s", err)
	}

	// ZONE-019: query config.tags for exact match
	filter := bson.M{
		"ns":  namespace,
		"min": minDoc,
		"max": maxDoc,
	}

	var tagDoc bson.M
	err = client.Database("config").Collection("tags").FindOne(ctx, filter).Decode(&tagDoc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// ZONE-020: range not found
			tflog.Warn(ctx, "zone key range not found in config.tags, removing from state", map[string]interface{}{
				"namespace": namespace,
				"min":       minJSON,
				"max":       maxJSON,
			})
			data.SetId("")
			return nil
		}
		return diag.Errorf("config.tags read failed: %s", err)
	}

	// Read back the zone (tag field) from the document
	tagZone, _ := tagDoc["tag"].(string)

	if err := data.Set("namespace", namespace); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("zone", tagZone); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("min", minJSON); err != nil {
		return diag.FromErr(err)
	}
	if err := data.Set("max", maxJSON); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

// Update is a no-op — all fields are identity fields blocked by CustomizeDiff. // ZONE-021
func (r *ResourceZoneKeyRange) Update(_ context.Context, _ *schema.ResourceData, _ interface{}) diag.Diagnostics {
	return nil
}

// Delete runs updateZoneKeyRange with zone: null to remove the range. // ZONE-022
func (r *ResourceZoneKeyRange) Delete(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	providerConf := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, providerConf)
	if err != nil {
		return diag.Errorf("error connecting to MongoDB: %s", err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	namespace := data.Get("namespace").(string)
	minJSON := data.Get("min").(string)
	maxJSON := data.Get("max").(string)

	minDoc, err := jsonToBsonD(minJSON)
	if err != nil {
		return diag.Errorf("invalid min: %s", err)
	}
	maxDoc, err := jsonToBsonD(maxJSON)
	if err != nil {
		return diag.Errorf("invalid max: %s", err)
	}

	tflog.Info(ctx, "removing zone key range", map[string]interface{}{
		"namespace": namespace,
		"min":       minJSON,
		"max":       maxJSON,
	})

	// ZONE-022: zone: null removes the range
	res := client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "updateZoneKeyRange", Value: namespace},
		{Key: "min", Value: minDoc},
		{Key: "max", Value: maxDoc},
		{Key: "zone", Value: nil},
	})
	if res.Err() != nil {
		return diag.FromErr(fmt.Errorf("updateZoneKeyRange (delete) failed: %w", res.Err()))
	}

	var resp OKResponse
	if err := res.Decode(&resp); err != nil {
		return diag.FromErr(fmt.Errorf("updateZoneKeyRange (delete) decode: %w", err))
	}
	if resp.OK != 1 {
		return diag.Errorf("updateZoneKeyRange (delete) failed: %s", resp.Errmsg)
	}

	return nil
}
