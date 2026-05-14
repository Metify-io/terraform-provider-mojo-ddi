# MCP Tool Contract Specification

> Phase 0 deliverable for the MOJO Terraform Provider.
> See [ADR-0004](https://github.com/Metify-io/mojo-installer/blob/main/docs/adr/0004-terraform-provider-mcp-first-architecture.md) for architecture context.

## Overview

This document defines the **contracts** between the Terraform provider and the
MOJO MCP server's tool surface. Every Terraform resource maps 1:1 to a CRUD
tool set on the MCP server. The contracts are authoritative — the Rust MCP
server implementation must satisfy them for the provider to function.

## Transport

| Property | Value |
|----------|-------|
| Protocol | JSON-RPC 2.0 over HTTP+SSE |
| Handshake | `initialize` → server returns `sessionId` |
| Notification | `notifications/initialized` (fire-and-forget) |
| Tool invocation | `tools/call` with `{ name, arguments }` |
| Session header | `Mcp-Session-Id` on every request after initialize |
| Auth header | `Authorization: Bearer <api_key>` (optional, for token-based auth) |
| MCP version | `2024-11-05` |

## Error Codes

The MCP server MUST return standard JSON-RPC 2.0 errors. The provider maps
specific error codes to Terraform behavior:

| Code | Meaning | Terraform Behavior |
|------|---------|-------------------|
| `-32001` | Resource not found | Read: remove from state (drift). Delete: succeed silently. |
| `-32002` | Conflict / duplicate | Create: surface as diagnostic error. |
| `-32003` | Validation error | Surface as diagnostic with field-level detail from `data`. |
| `-32600` | Invalid request | Surface as diagnostic error. |
| `-32601` | Method not found | Surface as diagnostic error (tool not registered). |

### Error response shape

```json
{
  "jsonrpc": "2.0",
  "id": 42,
  "error": {
    "code": -32001,
    "message": "VRF not found",
    "data": { "id": "550e8400-e29b-41d4-a716-446655440000" }
  }
}
```

## Success response shape

Tool results follow the MCP `tools/call` result schema:

```json
{
  "jsonrpc": "2.0",
  "id": 42,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"id\": \"...\", \"name\": \"production\", ...}"
      }
    ]
  }
}
```

The `text` field contains a JSON-serialized object matching the resource's API
model. The provider deserializes this into the typed API model struct.

## Tool Contract: CRUD Pattern

Every managed resource follows this pattern. The `{domain}` is the tool
namespace (`ipam`, `dhcp`, `dns`). The `{resource}` is the object name
(`vrf`, `vlan`, `prefix`, etc.).

### Create: `{domain}.create_{resource}`

- **Input**: All writable fields (required + optional). No `id` field.
- **Output**: Full object including server-assigned `id` and any computed defaults.
- **Idempotency**: NOT idempotent. Calling twice creates two objects.
- **Contract**: Server MUST return the complete object with all fields populated
  (including defaults for omitted optional fields).

### Read: `{domain}.read_{resource}`

- **Input**: `{ "id": "<uuid>" }`
- **Output**: Full object with all fields.
- **Not found**: Return error code `-32001`.
- **Contract**: Server MUST return the current state of the object. All fields
  present in the create response MUST be present in the read response.

### Update: `{domain}.update_{resource}`

- **Input**: `{ "id": "<uuid>", ...changed_fields }`. Partial update semantics.
- **Output**: Full updated object.
- **Not found**: Return error code `-32001`.
- **Contract**: Only fields included in the request are modified. Omitted fields
  retain their current values. Server MUST return the complete updated object.

### Delete: `{domain}.delete_{resource}`

- **Input**: `{ "id": "<uuid>" }`
- **Output**: Empty result or confirmation object.
- **Not found**: Return error code `-32001` (provider handles gracefully).
- **Contract**: Idempotent. Deleting an already-deleted resource returns `-32001`,
  which the provider treats as success.

## Tier 1 DDI Resources

### IPAM Domain (`ipam.*`)

#### `mojo_vrf` — Virtual Routing and Forwarding

| Field | Type | Required | Immutable | Default | Notes |
|-------|------|----------|-----------|---------|-------|
| `id` | string (uuid) | — | yes | server-assigned | |
| `name` | string | yes | no | — | Unique within MOJO |
| `description` | string | no | no | `""` | |
| `rd` | string | no | no | `""` | BGP Route Distinguisher (`ASN:NN`) |
| `enforce_unique` | boolean | no | no | `true` | Prevent duplicate IPs in this VRF |

Tools: `ipam.create_vrf`, `ipam.read_vrf`, `ipam.update_vrf`, `ipam.delete_vrf`

#### `mojo_vlan` — VLAN Definition

| Field | Type | Required | Immutable | Default | Notes |
|-------|------|----------|-----------|---------|-------|
| `id` | string (uuid) | — | yes | server-assigned | |
| `vid` | integer | yes | **yes** | — | VLAN ID (1–4094). RequiresReplace. |
| `name` | string | yes | no | — | |
| `site_id` | string (uuid) | no | no | `""` | Site association |
| `description` | string | no | no | `""` | |

Tools: `ipam.create_vlan`, `ipam.read_vlan`, `ipam.update_vlan`, `ipam.delete_vlan`

#### `mojo_prefix` — IP Prefix

| Field | Type | Required | Immutable | Default | Notes |
|-------|------|----------|-----------|---------|-------|
| `id` | string (uuid) | — | yes | server-assigned | |
| `cidr` | string (CIDR) | yes | **yes** | — | RequiresReplace. |
| `vrf_id` | string (uuid) | no | no | `""` | |
| `vlan_id` | string (uuid) | no | no | `""` | |
| `description` | string | no | no | `""` | |
| `is_pool` | boolean | no | no | `false` | Mark as address pool |

Tools: `ipam.create_prefix`, `ipam.read_prefix`, `ipam.update_prefix`, `ipam.delete_prefix`

#### `mojo_ip_address` — IP Address Allocation

| Field | Type | Required | Immutable | Default | Notes |
|-------|------|----------|-----------|---------|-------|
| `id` | string (uuid) | — | yes | server-assigned | |
| `address` | string | yes | **yes** | — | CIDR or plain. RequiresReplace. |
| `vrf_id` | string (uuid) | no | **yes** | `""` | RequiresReplace — update API does not support VRF change. |
| `status` | string | no | no | `"active"` | `active`, `reserved`, `deprecated`, `dhcp` |
| `dns_name` | string | no | no | `""` | FQDN for DNS auto-registration |
| `description` | string | no | no | `""` | |

Tools: `ipam.create_ip_address`, `ipam.read_ip_address`, `ipam.update_ip_address`, `ipam.delete_ip_address`

#### `mojo_ip_range` — Contiguous IP Range

| Field | Type | Required | Immutable | Default | Notes |
|-------|------|----------|-----------|---------|-------|
| `id` | string (uuid) | — | yes | server-assigned | |
| `start_address` | string | yes | **yes** | — | RequiresReplace. |
| `end_address` | string | yes | **yes** | — | RequiresReplace. |
| `vrf_id` | string (uuid) | no | **yes** | `""` | RequiresReplace. |
| `status` | string | no | no | `"active"` | |
| `description` | string | no | no | `""` | |

Tools: `ipam.create_ip_range`, `ipam.read_ip_range`, `ipam.update_ip_range`, `ipam.delete_ip_range`

### DHCP Domain (`dhcp.*`)

#### `mojo_dhcp_scope` — Kea DHCP Scope

| Field | Type | Required | Immutable | Default | Notes |
|-------|------|----------|-----------|---------|-------|
| `id` | string (uuid) | — | yes | server-assigned | |
| `prefix_id` | string (uuid) | yes | **yes** | — | RequiresReplace. Bound to a prefix. |
| `lease_time` | integer | no | no | `3600` | Seconds |
| `gateway` | string | no | no | `""` | Option routers |
| `dns_servers` | string | no | no | `""` | Comma-separated |
| `domain_name` | string | no | no | `""` | |
| `pxe_enabled` | boolean | no | no | `false` | |
| `pxe_server` | string | no | no | `""` | Required if pxe_enabled |
| `pxe_filename` | string | no | no | `""` | Required if pxe_enabled |
| `description` | string | no | no | `""` | |

Tools: `dhcp.create_scope`, `dhcp.read_scope`, `dhcp.update_scope`, `dhcp.delete_scope`

#### `mojo_dhcp_reservation` — Static DHCP Reservation

| Field | Type | Required | Immutable | Default | Notes |
|-------|------|----------|-----------|---------|-------|
| `id` | string (uuid) | — | yes | server-assigned | |
| `scope_id` | string (uuid) | yes | **yes** | — | RequiresReplace. |
| `mac_address` | string | yes | no | — | `aa:bb:cc:dd:ee:ff` format |
| `ip_address` | string | yes | no | — | |
| `hostname` | string | no | no | `""` | |
| `description` | string | no | no | `""` | |

Tools: `dhcp.create_reservation`, `dhcp.read_reservation`, `dhcp.update_reservation`, `dhcp.delete_reservation`

### DNS Domain (`dns.*`)

#### `mojo_dns_zone` — PowerDNS Zone

| Field | Type | Required | Immutable | Default | Notes |
|-------|------|----------|-----------|---------|-------|
| `id` | string (uuid) | — | yes | server-assigned | |
| `name` | string | yes | **yes** | — | RequiresReplace. Zone apex FQDN. |
| `kind` | string | no | no | `"Native"` | `Native`, `Master`, `Slave` |
| `nameservers` | string | no | no | `""` | Comma-separated FQDNs |
| `description` | string | no | no | `""` | |

Tools: `dns.create_zone`, `dns.read_zone`, `dns.update_zone`, `dns.delete_zone`

#### `mojo_dns_record` — DNS Record

| Field | Type | Required | Immutable | Default | Notes |
|-------|------|----------|-----------|---------|-------|
| `id` | string (uuid) | — | yes | server-assigned | |
| `zone_id` | string (uuid) | yes | **yes** | — | RequiresReplace. |
| `name` | string | yes | **yes** | — | RequiresReplace. Record name within zone. |
| `record_type` | string | yes | **yes** | — | RequiresReplace. A/AAAA/CNAME/PTR/MX/TXT/SRV/NS |
| `value` | string | yes | no | — | |
| `ttl` | integer | no | no | `300` | |
| `priority` | integer | no | no | `0` | MX/SRV only |

Tools: `dns.create_record`, `dns.read_record`, `dns.update_record`, `dns.delete_record`

## Immutability and RequiresReplace

Fields marked **Immutable = yes** in the tables above correspond to Terraform
schema attributes with `RequiresReplace()` plan modifiers. The MCP update tool
does NOT accept these fields — attempting to change them requires destroying
and recreating the resource.

The provider enforces this at the plan level. The MCP server should reject
update requests that include immutable fields with error code `-32003`.

## Data Sources

### `mojo_next_available_ip`

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `prefix_id` | string (uuid) | yes | Prefix to allocate from |
| `vrf_id` | string (uuid) | no | Scope to a specific VRF |
| `address` | string | — | Output: the allocated address |

Tool: `ipam.next_available_ip`

This is a **read-only** data source. It calls the MCP tool to find the next
available IP within a prefix without allocating it. Use `mojo_ip_address`
to actually allocate.

## Schema Versioning

MCP tool schemas are stored in `docs/mcp-tool-schemas/`. When the Rust MCP
server adds or modifies tools, the corresponding JSON schema file must be
updated and committed. The `make validate` target checks that hand-written
resource files are consistent with the schemas.

Schema files use the format:
```
docs/mcp-tool-schemas/{domain}.json
```

## Future Tier 2+ Resources

The following resources are planned but not yet defined. Tool contracts will
be added to this document as their MCP tools are implemented:

| Resource | Domain | Priority | Notes |
|----------|--------|----------|-------|
| `mojo_server_profile` | `server` | High | Unified Flavor+Baseline+Compliance |
| `mojo_resource_pool` | `server` | Medium | Logical server grouping |
| `mojo_firmware_baseline` | `firmware` | Medium | Golden-state firmware versions |
| `mojo_blueprint` | `workflow` | Medium | Declarative workflow definition |
| `mojo_discovery_range` | `discovery` | Low | Network scan ranges |
| `mojo_site` | `dcim` | Low | Physical location |
| `mojo_rack` | `dcim` | Low | Rack within a site |
