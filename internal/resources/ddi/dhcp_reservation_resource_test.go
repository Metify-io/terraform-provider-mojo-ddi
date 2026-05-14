// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package ddiresources_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDHCPReservationResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "mojo_prefix" "res_net" {
  cidr    = "10.211.0.0/24"
  is_pool = true
}

resource "mojo_dhcp_scope" "res_scope" {
  prefix_id  = mojo_prefix.res_net.id
  lease_time = 3600
  gateway    = "10.211.0.1"
}

resource "mojo_dhcp_reservation" "test" {
  scope_id    = mojo_dhcp_scope.res_scope.id
  mac_address = "aa:bb:cc:dd:ee:01"
  ip_address  = "10.211.0.50"
  hostname    = "acc-test-host"
  description = "acceptance test reservation"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("mojo_dhcp_reservation.test", "id"),
					resource.TestCheckResourceAttrPair("mojo_dhcp_reservation.test", "scope_id", "mojo_dhcp_scope.res_scope", "id"),
					resource.TestCheckResourceAttr("mojo_dhcp_reservation.test", "mac_address", "aa:bb:cc:dd:ee:01"),
					resource.TestCheckResourceAttr("mojo_dhcp_reservation.test", "ip_address", "10.211.0.50"),
					resource.TestCheckResourceAttr("mojo_dhcp_reservation.test", "hostname", "acc-test-host"),
				),
			},
			{
				ResourceName:      "mojo_dhcp_reservation.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: `
resource "mojo_prefix" "res_net" {
  cidr    = "10.211.0.0/24"
  is_pool = true
}

resource "mojo_dhcp_scope" "res_scope" {
  prefix_id  = mojo_prefix.res_net.id
  lease_time = 3600
  gateway    = "10.211.0.1"
}

resource "mojo_dhcp_reservation" "test" {
  scope_id    = mojo_dhcp_scope.res_scope.id
  mac_address = "aa:bb:cc:dd:ee:02"
  ip_address  = "10.211.0.51"
  hostname    = "acc-test-host-updated"
  description = "updated reservation"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mojo_dhcp_reservation.test", "mac_address", "aa:bb:cc:dd:ee:02"),
					resource.TestCheckResourceAttr("mojo_dhcp_reservation.test", "ip_address", "10.211.0.51"),
					resource.TestCheckResourceAttr("mojo_dhcp_reservation.test", "hostname", "acc-test-host-updated"),
				),
			},
		},
	})
}
