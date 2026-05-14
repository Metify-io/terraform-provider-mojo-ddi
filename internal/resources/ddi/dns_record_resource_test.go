// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package ddiresources_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDNSRecordResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "mojo_dns_zone" "record_zone" {
  name = "acc-record-test.example.com"
  kind = "Native"
}

resource "mojo_dns_record" "test" {
  zone_id     = mojo_dns_zone.record_zone.id
  name        = "www"
  record_type = "A"
  value       = "10.0.0.1"
  ttl         = 300
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("mojo_dns_record.test", "id"),
					resource.TestCheckResourceAttrPair("mojo_dns_record.test", "zone_id", "mojo_dns_zone.record_zone", "id"),
					resource.TestCheckResourceAttr("mojo_dns_record.test", "name", "www"),
					resource.TestCheckResourceAttr("mojo_dns_record.test", "record_type", "A"),
					resource.TestCheckResourceAttr("mojo_dns_record.test", "value", "10.0.0.1"),
					resource.TestCheckResourceAttr("mojo_dns_record.test", "ttl", "300"),
				),
			},
			{
				ResourceName:      "mojo_dns_record.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: `
resource "mojo_dns_zone" "record_zone" {
  name = "acc-record-test.example.com"
  kind = "Native"
}

resource "mojo_dns_record" "test" {
  zone_id     = mojo_dns_zone.record_zone.id
  name        = "www"
  record_type = "A"
  value       = "10.0.0.2"
  ttl         = 600
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mojo_dns_record.test", "value", "10.0.0.2"),
					resource.TestCheckResourceAttr("mojo_dns_record.test", "ttl", "600"),
				),
			},
		},
	})
}
