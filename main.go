// terraform-provider-seekrit is the Terraform provider for seekrit. It manages
// seekrit's structural resources (applications, groups, environments,
// composition, service tokens, DEK grants) as infrastructure-as-code.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/seekritdev/terraform-provider-seekrit/internal/provider"
)

// version is overridden at release build time (-ldflags "-X main.version=…").
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		// Placeholder registry address until the Terraform Registry namespace is
		// decided; unblocked for local dev via a dev_overrides block.
		Address: "registry.terraform.io/seekritdev/seekrit",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
