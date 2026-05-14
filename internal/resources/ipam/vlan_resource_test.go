// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package ipamresources_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccVLANResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "mojo_vlan" "test" {
  vid         = 100
  name        = "acc-test-vlan"
  description = "acceptance test"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("mojo_vlan.test", "id"),
					resource.TestCheckResourceAttr("mojo_vlan.test", "vid", "100"),
					resource.TestCheckResourceAttr("mojo_vlan.test", "name", "acc-test-vlan"),
					resource.TestCheckResourceAttr("mojo_vlan.test", "description", "acceptance test"),
				),
			},
			{
				ResourceName:      "mojo_vlan.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: `
resource "mojo_vlan" "test" {
  vid         = 100
  name        = "acc-test-vlan-updated"
  description = "updated description"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mojo_vlan.test", "name", "acc-test-vlan-updated"),
					resource.TestCheckResourceAttr("mojo_vlan.test", "description", "updated description"),
				),
			},
		},
	})
}
