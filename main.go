// The LakehouseBox provider for OpenTofu and Terraform (tablemere#3007, PLAN 21): the catalogs of an organisation and
// what feeds them, on the same API routes `lhbox plan|apply` uses. Tables are not resources: they come from migrations
// (`lhbox migrate`), so no apply can drop one.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/lakehousebox/terraform-provider-lakehousebox/internal/provider"
)

// set by the release build (goreleaser ldflags)
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers")
	flag.Parse()
	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.opentofu.org/lakehousebox/lakehousebox",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
