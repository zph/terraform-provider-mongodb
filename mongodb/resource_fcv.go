package mongodb

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"go.mongodb.org/mongo-driver/bson"
)

const fcvResourceID = "fcv"

var fcvFormatRegex = regexp.MustCompile(`^\d+\.\d+$`)

// validateFCVFormat checks that a string matches the X.Y version pattern.
// FCV-002
func validateFCVFormat(val interface{}, key string) ([]string, []error) {
	v := val.(string)
	if !fcvFormatRegex.MatchString(v) {
		return nil, []error{fmt.Errorf("%q must be in X.Y format (e.g. \"7.0\"), got: %q", key, v)}
	}
	return nil, nil
}

// compareFCV compares two FCV strings as (major, minor) integer pairs.
// Returns -1 (a < b), 0 (a == b), or +1 (a > b).
// FCV-012
func compareFCV(a, b string) (int, error) {
	aMajor, aMinor, err := parseFCVParts(a)
	if err != nil {
		return 0, fmt.Errorf("invalid FCV %q: %w", a, err)
	}
	bMajor, bMinor, err := parseFCVParts(b)
	if err != nil {
		return 0, fmt.Errorf("invalid FCV %q: %w", b, err)
	}

	switch {
	case aMajor < bMajor:
		return -1, nil
	case aMajor > bMajor:
		return 1, nil
	case aMinor < bMinor:
		return -1, nil
	case aMinor > bMinor:
		return 1, nil
	default:
		return 0, nil
	}
}

// parseFCVParts splits "X.Y" into (major, minor) integers.
func parseFCVParts(fcv string) (int, int, error) {
	parts := strings.SplitN(fcv, ".", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected X.Y format, got %q", fcv)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid major version %q: %w", parts[0], err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid minor version %q: %w", parts[1], err)
	}
	return major, minor, nil
}

// FCV-001, FCV-013
func resourceFCV() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceFCVCreate,
		ReadContext:   resourceFCVRead,
		UpdateContext: resourceFCVUpdate,
		DeleteContext: resourceFCVDelete,
		// FCV-008, FCV-009
		CustomizeDiff: customdiff.All(
			func(ctx context.Context, d *schema.ResourceDiff, meta interface{}) error {
				// Only gate changes on existing resources (Id != "")
				if d.Id() == "" {
					return nil
				}
				if !d.HasChange("version") {
					return nil
				}
				dangerMode := d.Get("danger_mode").(bool)
				if !dangerMode {
					oldVal, newVal := d.GetChange("version")
					return fmt.Errorf(
						"changing feature_compatibility_version is a dangerous, potentially irreversible operation; "+
							"set danger_mode = true to proceed (current → %q, proposed → %q)",
						oldVal, newVal,
					)
				}
				return nil
			},
		),
		Schema: map[string]*schema.Schema{
			"version": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validateFCVFormat,
				Description:  "The target featureCompatibilityVersion (e.g. \"7.0\").",
			},
			"danger_mode": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Must be true to allow version changes. Protects against accidental FCV upgrades/downgrades.",
			},
		},
	}
}

// FCV-004, FCV-010, FCV-011, FCV-014
func resourceFCVCreate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	config := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, config)
	if err != nil {
		return diag.Errorf("setFeatureCompatibilityVersion: connect: %s", err)
	}

	version := data.Get("version").(string)

	var diags diag.Diagnostics

	// FCV-010, FCV-011: emit warnings if this is a version change on an existing resource
	if data.Id() != "" {
		oldVersion, _ := GetFCV(ctx, client)
		if oldVersion != "" && oldVersion != version {
			cmp, cmpErr := compareFCV(version, oldVersion)
			if cmpErr == nil {
				diags = append(diags, diag.Diagnostic{
					Severity: diag.Warning,
					Summary:  fmt.Sprintf("Changing featureCompatibilityVersion from %q to %q", oldVersion, version),
					Detail:   "FCV changes affect the entire cluster and may be irreversible.",
				})
				if cmp < 0 {
					diags = append(diags, diag.Diagnostic{
						Severity: diag.Warning,
						Summary:  fmt.Sprintf("Downgrading featureCompatibilityVersion from %q to %q", oldVersion, version),
						Detail:   "Downgrading FCV may disable features that depend on the higher version. Ensure all nodes are compatible before proceeding.",
					})
				}
			}
		}
	}

	// FCV-004
	res := client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "setFeatureCompatibilityVersion", Value: version},
	})
	if res.Err() != nil {
		// FCV-014
		return diag.Errorf("setFeatureCompatibilityVersion %q: %s", version, res.Err())
	}

	// FCV-003
	data.SetId(fcvResourceID)
	readDiags := resourceFCVRead(ctx, data, i)
	return append(diags, readDiags...)
}

// FCV-005
func resourceFCVRead(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	config := i.(*MongoDatabaseConfiguration)
	client, err := MongoClientInit(ctx, config)
	if err != nil {
		return diag.Errorf("getParameter featureCompatibilityVersion: connect: %s", err)
	}

	fcv, err := GetFCV(ctx, client)
	if err != nil {
		return diag.Errorf("getParameter featureCompatibilityVersion: %s", err)
	}

	if err := data.Set("version", fcv); err != nil {
		return diag.FromErr(err)
	}
	return nil
}

// FCV-006
func resourceFCVUpdate(ctx context.Context, data *schema.ResourceData, i interface{}) diag.Diagnostics {
	return resourceFCVCreate(ctx, data, i)
}

// FCV-007
func resourceFCVDelete(_ context.Context, data *schema.ResourceData, _ interface{}) diag.Diagnostics {
	data.SetId("")
	return nil
}
