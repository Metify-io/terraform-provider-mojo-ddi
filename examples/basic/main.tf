terraform {
  required_providers {
    mojo = {
      source  = "metify-io/mojo-ddi"
      version = "~> 0.1"
    }
  }
}

provider "mojo" {
  endpoint = "https://mojo.local:8443/mcp"
  # api_key from MOJO_API_KEY env var
}

# --- IPAM ---

resource "mojo_vrf" "staging" {
  name           = "staging"
  description    = "Staging environment VRF"
  enforce_unique = true
}

resource "mojo_vlan" "servers" {
  vid         = 100
  name        = "servers"
  description = "Server VLAN"
}

resource "mojo_prefix" "servers" {
  cidr        = "10.0.100.0/24"
  vrf_id      = mojo_vrf.staging.id
  vlan_id     = mojo_vlan.servers.id
  description = "Server subnet"
  is_pool     = true
}

resource "mojo_ip_address" "gateway" {
  address     = "10.0.100.1/24"
  vrf_id      = mojo_vrf.staging.id
  status      = "active"
  dns_name    = "gw.staging.example.com"
  description = "Default gateway"
}

resource "mojo_ip_range" "dhcp_pool" {
  start_address = "10.0.100.100"
  end_address   = "10.0.100.200"
  vrf_id        = mojo_vrf.staging.id
  status        = "active"
  description   = "DHCP dynamic pool"
}

# --- DHCP ---

resource "mojo_dhcp_scope" "servers" {
  prefix_id   = mojo_prefix.servers.id
  lease_time  = 3600
  gateway     = "10.0.100.1"
  dns_servers = "10.0.100.2,10.0.100.3"
  domain_name = "staging.example.com"
  description = "Server DHCP scope"
}

resource "mojo_dhcp_reservation" "webserver" {
  scope_id    = mojo_dhcp_scope.servers.id
  mac_address = "aa:bb:cc:dd:ee:01"
  ip_address  = "10.0.100.10"
  hostname    = "web01"
  description = "Web server 01"
}

# --- DNS ---

resource "mojo_dns_zone" "staging" {
  name        = "staging.example.com"
  kind        = "Native"
  nameservers = "ns1.example.com,ns2.example.com"
  description = "Staging DNS zone"
}

resource "mojo_dns_record" "web01_a" {
  zone_id     = mojo_dns_zone.staging.id
  name        = "web01"
  record_type = "A"
  value       = "10.0.100.10"
  ttl         = 300
}
