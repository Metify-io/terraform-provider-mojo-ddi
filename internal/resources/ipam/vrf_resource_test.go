// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package ipamresources_test

import (
	"testing"

	"github.com/Metify-io/terraform-provider-mojo-ddi/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"mojo": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func TestAccVRFResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: `
resource "mojo_vrf" "test" {
  name           = "acc-test-vrf"
  description    = "acceptance test"
  rd             = "65000:1"
  enforce_unique = true
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("mojo_vrf.test", "id"),
					resource.TestCheckResourceAttr("mojo_vrf.test", "name", "acc-test-vrf"),
					resource.TestCheckResourceAttr("mojo_vrf.test", "description", "acceptance test"),
					resource.TestCheckResourceAttr("mojo_vrf.test", "rd", "65000:1"),
					resource.TestCheckResourceAttr("mojo_vrf.test", "enforce_unique", "true"),
				),
			},
			// ImportState
			{
				ResourceName:      "mojo_vrf.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update
			{
				Config: `
resource "mojo_vrf" "test" {
  name           = "acc-test-vrf-updated"
  description    = "updated description"
  rd             = "65000:2"
  enforce_unique = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mojo_vrf.test", "name", "acc-test-vrf-updated"),
					resource.TestCheckResourceAttr("mojo_vrf.test", "description", "updated description"),
					resource.TestCheckResourceAttr("mojo_vrf.test", "rd", "65000:2"),
					resource.TestCheckResourceAttr("mojo_vrf.test", "enforce_unique", "false"),
				),
			},
		},
	})
}
