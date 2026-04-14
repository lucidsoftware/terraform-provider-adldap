package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"

	"github.com/lucidsoftware/terraform-provider-adldap/internal/provider"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// Run "go generate" to format example terraform files and generate the docs for the registry/website

// If you do not have terraform installed, you can remove the formatting command, but its suggested to
// ensure the documentation is formatted properly.
//go:generate terraform fmt -recursive ./examples/

// Run the docs generation tool, check its repository for more information on how it works and how docs
// can be customized.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name=ldap --rendered-provider-name=LDAP

var (
	// these will be set by the goreleaser configuration
	// to appropriate values for the compiled binary.
	version string = "dev"

	// goreleaser can pass other information to the main package, such as the specific commit
	// https://goreleaser.com/cookbooks/using-main.version/
)

var serveProvider = providerserver.Serve
var fatalLog = log.Fatal

func run(args []string, serve func(context.Context, func() frameworkprovider.Provider, providerserver.ServeOpts) error) error {
	var debug bool

	flagSet := flag.NewFlagSet("terraform-provider-adldap", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	flagSet.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	if err := flagSet.Parse(args); err != nil {
		return err
	}

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/lucidsoftware/adldap",
		Debug:   debug,
	}

	return serve(context.Background(), provider.New(version), opts)
}

func main() {
	if err := run(os.Args[1:], serveProvider); err != nil {
		fatalLog("provider exited with error")
	}
}
