package bad

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func badResource() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true, // want `ForceNew: true is banned`
			},
		},
	}
}
