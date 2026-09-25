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
  # api_key from MOJO_API_KEY env var
}

# --- Governed provisioning ---
#
# `mojo_server` wraps the `provision_server` MCP tool (destructive class,
# ADR-0031). A real (non-dry-run) provision is refused by the governance
# boundary unless an approval token is supplied.
#
# Operator flow:
#   1. terraform plan -out=plan.tfplan
#   2. terraform show -json plan.tfplan > plan.json
#   3. On the MOJO box, mint a token pinned to the plan:
#      podman exec mojo-app python /app/coordinator-django/manage.py \
#          mint_approval --tool provision_server --principal agent:terraform \
#          --on-behalf-of ops@example.com --ttl 600 \
#          --target serial:MXQ1250HZ1 --plan-file /root/plans/plan.json
#   4. terraform apply -var="approval_token=<minted-token>"
#
# The token is single-use, hash-pinned to the plan and target, and short-lived.
# It lands in state marked sensitive — rotate/expire it after the apply.

variable "approval_token" {
  type      = string
  default   = null
  sensitive = true
}

# Rehearse the full gate chain without touching hardware: set dry_run = true.
# Plan then applies cleanly with no token, status lands as "planned", and the
# ledger records a "planned" intent the auditor can diff against.
resource "mojo_server" "dl360" {
  serial_number = "MXQ1250HZ1"       # DL360 Gen10 — resolved via search_nodes
  # server_id   = "d2914e5f-..."     # or pin the node UUID directly

  profile_name    = "ubuntu-2404-lts"  # resolved via list_profiles; or pass profile_id
  # os_family     = "ubuntu"           # disambiguation hint when names overlap
  # baseline_id   = "a1b2c3d4-..."     # optional: bind firmware baseline at provision time

  dry_run             = true           # flip to false + approval_token to actuate
  approval_token      = var.approval_token
  wait_for_completion = true
  wait_timeout_seconds  = 3600
  poll_interval_seconds = 30
}

# --- Firmware baseline convergence ---
#
# `mojo_firmware_baseline` converges a node's firmware/BIOS to a baseline via
# `apply_baseline` (token-gated actuation). Read/refresh uses the last stored
# `evaluate_baseline` result (DB-only, no BMC call). Pair with the
# `mojo_baseline_evaluation` data source below to see live drift in plan output.

data "mojo_baseline_evaluation" "dl360_drift" {
  serial_number = "MXQ1250HZ1"
  baseline_id   = "a1b2c3d4-0000-4000-8000-000000000000"
}

output "dl360_baseline_status" {
  value = data.mojo_baseline_evaluation.dl360_drift.overall_status
}

# resource "mojo_firmware_baseline" "dl360" {
#   serial_number   = "MXQ1250HZ1"
#   baseline_id     = "a1b2c3d4-0000-4000-8000-000000000000"
#   approval_token  = var.approval_token   # minted for apply_baseline on this node
#   dry_run         = true                 # set false to actually converge
# }
