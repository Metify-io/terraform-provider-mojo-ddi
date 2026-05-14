// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package ddiresources_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDNSZoneResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "mojo_dns_zone" "test" {
  name        = "acc-test.example.com"
  kind        = "Native"
  nameservers = "ns1.example.com,ns2.example.com"
  description = "acceptance test zone"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("mojo_dns_zone.test", "id"),
					resource.TestCheckResourceAttr("mojo_dns_zone.test", "name", "acc-test.example.com"),
					resource.TestCheckResourceAttr("mojo_dns_zone.test", "kind", "Native"),
					resource.TestCheckResourceAttr("mojo_dns_zone.test", "nameservers", "ns1.example.com,ns2.example.com"),
				),
			},
			{
				ResourceName:      "mojo_dns_zone.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: `
resource "mojo_dns_zone" "test" {
  name        = "acc-test.example.com"
  kind        = "Master"
  nameservers = "ns1.example.com"
  description = "updated zone"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mojo_dns_zone.test", "kind", "Master"),
					resource.TestCheckResourceAttr("mojo_dns_zone.test", "nameservers", "ns1.example.com"),
					resource.TestCheckResourceAttr("mojo_dns_zone.test", "description", "updated zone"),
				),
			},
		},
	})
}
