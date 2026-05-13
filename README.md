# terraform-provider-mojo-ddi

Terraform provider for [MOJO](https://metify.io) DDI (DHCP, DNS, IPAM) resources.

Manages VRFs, VLANs, prefixes, IP addresses, DHCP scopes/reservations, and DNS
zones/records via the MOJO MCP server.

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.8
- [Go](https://golang.org/doc/install) >= 1.23 (to build the provider)
- A running MOJO instance with MCP server enabled

## Usage

```hcl
terraform {
  required_providers {
    mojo = {
      source  = "metify/mojo-ddi"
      version = "~> 0.1"
    }
  }
}

provider "mojo" {
  endpoint = "https://mojo.local:8443/mcp"
  api_key  = var.mojo_api_key
}

resource "mojo_vrf" "production" {
  name           = "production"
  description    = "Production VRF"
  enforce_unique = true
}

resource "mojo_prefix" "servers" {
  cidr        = "10.0.0.0/24"
  vrf_id      = mojo_vrf.production.id
  description = "Server management network"
}

resource "mojo_dhcp_scope" "servers" {
  prefix_id   = mojo_prefix.servers.id
  lease_time  = 3600
  gateway     = "10.0.0.1"
  pxe_enabled = true
}
```

## Building

```sh
make build      # build the provider binary
make install    # install to local Terraform plugin directory
make test       # run unit tests
make testacc    # run acceptance tests (requires a MOJO instance)
make generate   # regenerate resource stubs from MCP tool schemas
make lint       # run golangci-lint
```

## Architecture

This provider connects to the MOJO MCP server over HTTP+SSE (JSON-RPC 2.0)
rather than wrapping the REST API directly. See
[ADR-0004](https://github.com/Metify-io/mojo-installer/blob/main/docs/adr/0004-terraform-provider-mcp-first-architecture.md)
for the architectural decision and rationale.

```
Terraform Provider (Go) ──► MCP Server (Rust) ──► MOJO Core Platform
```

## Configuration

| Attribute  | Env Var          | Description                    |
|------------|------------------|--------------------------------|
| `endpoint` | `MOJO_ENDPOINT`  | MCP server URL                 |
| `api_key`  | `MOJO_API_KEY`   | API key for MCP authentication |

For air-gapped environments, mTLS support is planned (see provider schema).

## Resources

### Tier 1 — IPAM/DDI (this provider)

| Resource                     | Description                          |
|------------------------------|--------------------------------------|
| `mojo_vrf`                   | Virtual Routing and Forwarding       |
| `mojo_vlan`                  | VLAN definition                      |
| `mojo_prefix`                | IP prefix (auto-nesting by CIDR)     |
| `mojo_ip_address`            | Allocated IP address                 |
| `mojo_ip_range`              | Contiguous IP range                  |
| `mojo_dhcp_scope`            | Kea DHCP scope                       |
| `mojo_dhcp_reservation`      | Static DHCP reservation (MAC→IP)     |
| `mojo_dns_zone`              | PowerDNS zone                        |
| `mojo_dns_record`            | DNS record (A, AAAA, CNAME, PTR...) |

### Data Sources

| Data Source                  | Description                          |
|------------------------------|--------------------------------------|
| `mojo_next_available_ip`     | Next unallocated IP in a prefix      |

## License

Apache 2.0 — see [LICENSE](LICENSE).
