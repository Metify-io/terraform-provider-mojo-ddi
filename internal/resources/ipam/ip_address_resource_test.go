// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package ipamresources_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIPAddressResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "mojo_prefix" "test_net" {
  cidr = "10.201.0.0/24"
}

resource "mojo_ip_address" "test" {
  address     = "10.201.0.10/24"
  status      = "active"
  dns_name    = "acc-test.example.com"
  description = "acceptance test ip"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("mojo_ip_address.test", "id"),
					resource.TestCheckResourceAttr("mojo_ip_address.test", "address", "10.201.0.10/24"),
					resource.TestCheckResourceAttr("mojo_ip_address.test", "status", "active"),
					resource.TestCheckResourceAttr("mojo_ip_address.test", "dns_name", "acc-test.example.com"),
				),
			},
			{
				ResourceName:      "mojo_ip_address.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: `
resource "mojo_prefix" "test_net" {
  cidr = "10.201.0.0/24"
}

resource "mojo_ip_address" "test" {
  address     = "10.201.0.10/24"
  status      = "reserved"
  dns_name    = "acc-test-updated.example.com"
  description = "updated ip"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mojo_ip_address.test", "status", "reserved"),
					resource.TestCheckResourceAttr("mojo_ip_address.test", "dns_name", "acc-test-updated.example.com"),
				),
			},
		},
	})
}
