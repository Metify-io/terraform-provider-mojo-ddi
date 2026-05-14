// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package ddiresources_test

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

func TestAccDHCPScopeResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "mojo_prefix" "dhcp_net" {
  cidr    = "10.210.0.0/24"
  is_pool = true
}

resource "mojo_dhcp_scope" "test" {
  prefix_id   = mojo_prefix.dhcp_net.id
  lease_time  = 3600
  gateway     = "10.210.0.1"
  dns_servers = "8.8.8.8,8.8.4.4"
  domain_name = "acc-test.local"
  description = "acceptance test scope"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("mojo_dhcp_scope.test", "id"),
					resource.TestCheckResourceAttrPair("mojo_dhcp_scope.test", "prefix_id", "mojo_prefix.dhcp_net", "id"),
					resource.TestCheckResourceAttr("mojo_dhcp_scope.test", "lease_time", "3600"),
					resource.TestCheckResourceAttr("mojo_dhcp_scope.test", "gateway", "10.210.0.1"),
					resource.TestCheckResourceAttr("mojo_dhcp_scope.test", "dns_servers", "8.8.8.8,8.8.4.4"),
					resource.TestCheckResourceAttr("mojo_dhcp_scope.test", "domain_name", "acc-test.local"),
				),
			},
			{
				ResourceName:      "mojo_dhcp_scope.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: `
resource "mojo_prefix" "dhcp_net" {
  cidr    = "10.210.0.0/24"
  is_pool = true
}

resource "mojo_dhcp_scope" "test" {
  prefix_id   = mojo_prefix.dhcp_net.id
  lease_time  = 7200
  gateway     = "10.210.0.1"
  dns_servers = "1.1.1.1"
  domain_name = "acc-test-updated.local"
  description = "updated scope"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mojo_dhcp_scope.test", "lease_time", "7200"),
					resource.TestCheckResourceAttr("mojo_dhcp_scope.test", "dns_servers", "1.1.1.1"),
					resource.TestCheckResourceAttr("mojo_dhcp_scope.test", "domain_name", "acc-test-updated.local"),
				),
			},
		},
	})
}
