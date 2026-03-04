package main

import (
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
	"github.com/zph/terraform-provider-mongodb/mongodb"
)

func main() {
	log.Printf("[INFO] terraform-provider-mongodb %s (%s)", version, commit)

	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: mongodb.Provider,
	})
}
