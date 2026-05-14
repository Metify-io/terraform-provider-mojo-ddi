// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package provider_test

import (
	"testing"

	"github.com/Metify-io/terraform-provider-mojo-ddi/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories is used by all acceptance tests to
// instantiate the provider under test. It returns a proto v6 provider
// server backed by the real MojoProvider, configured by environment
// variables (MOJO_ENDPOINT, MOJO_API_KEY).
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"mojo": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func TestProviderFactoriesExist(t *testing.T) {
	if len(testAccProtoV6ProviderFactories) == 0 {
		t.Fatal("expected provider factories to be populated")
	}
}
