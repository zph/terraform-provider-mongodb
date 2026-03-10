package good

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func goodResource() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}
