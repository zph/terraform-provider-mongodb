package mongodb

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"host": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("MONGO_HOST", "127.0.0.1"),
				Description: "The mongodb server address",
			},
			"port": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("MONGO_PORT", "27017"),
				Description: "The mongodb server port",
			},
			"certificate": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				DefaultFunc: schema.EnvDefaultFunc("MONGODB_CERT", ""),
				Description: "PEM-encoded content of Mongodb host CA certificate",
			},

			"username": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("MONGO_USR", ""),
				Description: "The mongodb user",
			},
			"password": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				DefaultFunc: schema.EnvDefaultFunc("MONGO_PWD", ""),
				Description: "The mongodb password",
			},
			"auth_database": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "admin",
				Description: "The mongodb auth database",
			},
			"replica_set": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "",
				Description: "The mongodb replica set",
			},
			"insecure_skip_verify": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "ignore hostname verification",
			},
			"ssl": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "ssl activation",
			},
			"direct": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "enforces a direct connection instead of discovery",
			},
			"retrywrites": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Retryable Writes",
			},
			"proxy": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"ALL_PROXY",
					"all_proxy",
				}, nil),
				ValidateDiagFunc: validateDiagFunc(validation.StringMatch(regexp.MustCompile("^socks5h?://.*:\\d+$"), "The proxy URL is not a valid socks url.")),
			},
			// PREVIEW-001, PREVIEW-004
			"command_preview": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				DefaultFunc: schema.EnvDefaultFunc("TERRAFORM_PROVIDER_MONGODB_COMMAND_PREVIEW", false),
				Description: "When true, populate planned_commands on each resource during terraform plan showing the MongoDB commands that will execute.",
			},
			// GATE-006: HCL equivalent of TERRAFORM_PROVIDER_MONGODB_ENABLE
			"features_enabled": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
					ValidateFunc: validation.StringInSlice(
						experimentalResourceNames(), false,
					),
				},
				Description: fmt.Sprintf(
					"Set of experimental resource names to enable. "+
						"Equivalent to the %s environment variable. Both sources are merged.",
					EnableEnvVar,
				),
			},
		},
		ResourcesMap:         BuildResourceMap(AllResources()),
		DataSourcesMap:       map[string]*schema.Resource{},
		ConfigureContextFunc: providerConfigure,
	}
}

type MongoDatabaseConfiguration struct {
	Config          *ClientConfig
	MaxConnLifetime time.Duration
	CommandPreview  bool            // PREVIEW-003
	FeaturesEnabled map[string]bool // GATE-006
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	var diags diag.Diagnostics

	clientConfig := ClientConfig{
		Host:               d.Get("host").(string),
		Port:               d.Get("port").(string),
		Username:           d.Get("username").(string),
		Password:           d.Get("password").(string),
		DB:                 d.Get("auth_database").(string),
		Ssl:                d.Get("ssl").(bool),
		ReplicaSet:         d.Get("replica_set").(string),
		Certificate:        d.Get("certificate").(string),
		InsecureSkipVerify: d.Get("insecure_skip_verify").(bool),
		Direct:             d.Get("direct").(bool),
		RetryWrites:        d.Get("retrywrites").(bool),
		Proxy:              d.Get("proxy").(string),
	}

	// GATE-006: merge HCL features_enabled with env var
	hclFeatures := make(map[string]bool)
	if v, ok := d.GetOk("features_enabled"); ok {
		for _, name := range v.(*schema.Set).List() {
			hclFeatures[name.(string)] = true
		}
	}
	featuresEnabled := mergeEnableLists(parseEnableList(), hclFeatures)

	return &MongoDatabaseConfiguration{
		Config:          &clientConfig,
		MaxConnLifetime: 10,
		CommandPreview:  d.Get("command_preview").(bool), // PREVIEW-003
		FeaturesEnabled: featuresEnabled,
	}, diags
}

// experimentalResourceNames returns the names of all experimental resources
// for use in validation.
func experimentalResourceNames() []string {
	var names []string
	for _, reg := range AllResources() {
		if reg.Maturity == ResourceExperimental {
			names = append(names, reg.Name)
		}
	}
	return names
}
