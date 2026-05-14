// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package ipamresources_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPrefixResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "mojo_prefix" "test" {
  cidr        = "10.200.0.0/24"
  description = "acceptance test prefix"
  is_pool     = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("mojo_prefix.test", "id"),
					resource.TestCheckResourceAttr("mojo_prefix.test", "cidr", "10.200.0.0/24"),
					resource.TestCheckResourceAttr("mojo_prefix.test", "description", "acceptance test prefix"),
					resource.TestCheckResourceAttr("mojo_prefix.test", "is_pool", "false"),
				),
			},
			{
				ResourceName:      "mojo_prefix.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: `
resource "mojo_prefix" "test" {
  cidr        = "10.200.0.0/24"
  description = "updated prefix"
  is_pool     = true
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mojo_prefix.test", "description", "updated prefix"),
					resource.TestCheckResourceAttr("mojo_prefix.test", "is_pool", "true"),
				),
			},
		},
	})
}
