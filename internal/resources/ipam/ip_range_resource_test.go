// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package ipamresources_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIPRangeResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "mojo_prefix" "range_net" {
  cidr = "10.202.0.0/24"
}

resource "mojo_ip_range" "test" {
  start_address = "10.202.0.100"
  end_address   = "10.202.0.200"
  status        = "active"
  description   = "acceptance test range"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("mojo_ip_range.test", "id"),
					resource.TestCheckResourceAttr("mojo_ip_range.test", "start_address", "10.202.0.100"),
					resource.TestCheckResourceAttr("mojo_ip_range.test", "end_address", "10.202.0.200"),
					resource.TestCheckResourceAttr("mojo_ip_range.test", "status", "active"),
				),
			},
			{
				ResourceName:      "mojo_ip_range.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: `
resource "mojo_prefix" "range_net" {
  cidr = "10.202.0.0/24"
}

resource "mojo_ip_range" "test" {
  start_address = "10.202.0.100"
  end_address   = "10.202.0.200"
  status        = "reserved"
  description   = "updated range"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mojo_ip_range.test", "status", "reserved"),
					resource.TestCheckResourceAttr("mojo_ip_range.test", "description", "updated range"),
				),
			},
		},
	})
}
