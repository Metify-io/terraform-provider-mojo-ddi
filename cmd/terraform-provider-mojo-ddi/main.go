// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"log"

	"github.com/Metify-io/terraform-provider-mojo-ddi/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/metify/mojo-ddi",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
