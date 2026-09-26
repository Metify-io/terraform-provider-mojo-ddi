// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package actuation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Metify-io/terraform-provider-mojo-ddi/internal/mcp"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ServerResource{}
var _ resource.ResourceWithValidateConfig = &ServerResource{}

func NewServerResource() resource.Resource {
	return &ServerResource{}
}

// ServerResource implements the mojo_server resource: an OS-provisioning
// request driven through the governed `provision_server` MCP tool.
type ServerResource struct {
	client *mcp.Client
}

type ServerResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	SerialNumber         types.String `tfsdk:"serial_number"`
	ServerID             types.String `tfsdk:"server_id"`
	NodeID               types.String `tfsdk:"node_id"`
	ProfileID            types.String `tfsdk:"profile_id"`
	ProfileName          types.String `tfsdk:"profile_name"`
	OSFamily             types.String `tfsdk:"os_family"`
	BaselineID           types.String `tfsdk:"baseline_id"`
	ApprovalToken        types.String `tfsdk:"approval_token"`
	DestroyApprovalToken types.String `tfsdk:"destroy_approval_token"`
	DryRun               types.Bool   `tfsdk:"dry_run"`
	WaitForCompletion    types.Bool   `tfsdk:"wait_for_completion"`
	WaitTimeout          types.Int64  `tfsdk:"wait_timeout_seconds"`
	PollInterval         types.Int64  `tfsdk:"poll_interval_seconds"`
	RequestID            types.String `tfsdk:"request_id"`
	Status               types.String `tfsdk:"status"`
	ProgressPct          types.Int64  `tfsdk:"progress_pct"`
}

func (r *ServerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server"
}

func (r *ServerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Provisions a MOJO-managed server via the governed ` + "`provision_server`" + ` MCP tool (actuation class: destructive).

Destructive calls are gated by the mojo-mcp boundary: a live apply requires a single-use approval token minted on the MOJO box:

` + "```sh" + `
podman exec mojo-app python /app/coordinator-django/manage.py mint_approval \
  --tool provision_server --principal agent:<name> \
  --on-behalf-of <approver-email> --ttl 3600 --target serial:<SERIAL>
` + "```" + `

Paste the token into ` + "`approval_token`" + ` (use a ` + "`*.tfvars`" + ` or environment variable, not version control). Tokens are single-use and target-scoped; each apply that actuates needs a fresh mint. Set ` + "`dry_run = true`" + ` to rehearse the call through every gate without actuating — no token needed, the intent closes ` + "`planned`" + ` on the ledger.

When the MCP server delegates provisioning to the coordinator (Phase 1) the response carries no ` + "`request_id`" + ` and wait-for-completion cannot poll; the resource records ` + "`status = \"delegated\"`" + `.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Provision request ID (Rust PXE engine) or `provision:<server_id>` when the call was delegated to the coordinator.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"serial_number": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Chassis serial number of the node to provision (resolved via `search_nodes`). Exactly one of `serial_number`/`server_id` must be set.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"server_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Pin a MOJO node UUID explicitly instead of resolving `serial_number`. Mutually exclusive in practice with serial resolution — when both are set they must agree.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"node_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved MOJO node UUID the request targets.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"profile_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "OS profile UUID to deploy (`provision_server` `profile_id`). Required by the Rust PXE engine.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"profile_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "OS profile name, resolved to `profile_id` via `list_profiles` at apply time. Conflicts with `profile_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"os_family": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "OS family filter (`rhel`, `debian`, `ubuntu`, `rocky`) applied when resolving `profile_name`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"baseline_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Firmware baseline UUID to apply before provisioning (`provision_server` `baseline_id`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"approval_token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Coordinator-minted approval token (`v1.<payload>.<sig>`) for the `provision_server` call. Required on a live apply of this destructive tool; consumed single-use by the boundary. Supply per-apply (e.g. `-var` or tfvars).",
			},
			"destroy_approval_token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Approval token for `cancel_provision` on `terraform destroy`. When unset, destroy only removes the Terraform record — the MOJO-side request is left alone.",
			},
			"dry_run": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Rehearse the call through every gate without actuating (the ledger intent closes `planned`). Changing this replaces the resource so a rehearsal never silently masquerades as a live provision.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"wait_for_completion": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Poll `get_provision_status` until the request reaches a terminal state. Only effective when the call returns a `request_id` (Rust PXE engine).",
			},
			"wait_timeout_seconds": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(3600),
				MarkdownDescription: "Maximum seconds to wait for provisioning when `wait_for_completion` is true.",
			},
			"poll_interval_seconds": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(30),
				MarkdownDescription: "Seconds between `get_provision_status` polls.",
			},
			"request_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Provisioning request UUID (Rust PXE engine only).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Last observed request status (`queued`, `installing`, `complete`, `failed`, `canceled`, `delegated`, `planned`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"progress_pct": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Last observed progress percentage.",
			},
		},
	}
}

func (r *ServerResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config ServerResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.SerialNumber.IsNull() && config.ServerID.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("serial_number"),
			"Missing node identifier",
			"Set serial_number or server_id to identify the node to provision.",
		)
	}
	if !config.ProfileID.IsNull() && !config.ProfileName.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("profile_name"),
			"Conflicting profile selection",
			"Set only one of profile_id or profile_name.",
		)
	}
}

func (r *ServerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*mcp.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected resource provider data", fmt.Sprintf("Expected *mcp.Client, got %T.", req.ProviderData))
		return
	}
	r.client = client
}

func (r *ServerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ServerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve from configuration, not plan: a computed server_id carried over
	// from prior state must not pin a replacement to the previous node.
	var config ServerResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverID, err := ResolveServerID(ctx, r.client, config.ServerID.ValueString(), config.SerialNumber.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve node", err.Error())
		return
	}
	plan.NodeID = types.StringValue(serverID)

	profileID, err := resolveProfileID(ctx, r.client, plan.ProfileID.ValueString(), plan.ProfileName.ValueString(), plan.OSFamily.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve OS profile", err.Error())
		return
	}

	args := map[string]any{"server_id": serverID}
	if profileID != "" {
		args["profile_id"] = profileID
	}
	if v := plan.BaselineID.ValueString(); v != "" {
		args["baseline_id"] = v
	}
	if v := plan.ApprovalToken.ValueString(); v != "" {
		args["approval"] = v
	}
	if plan.DryRun.ValueBool() {
		args["dry_run"] = true
	}

	text, err := r.client.CallToolText(ctx, "provision_server", args)
	if err != nil {
		resp.Diagnostics.AddError(
			"provision_server call refused or failed",
			err.Error()+"\n\nA destructive call needs an approval token minted on the MOJO box "+
				"(manage.py mint_approval --tool provision_server --target serial:<SERIAL>). "+
				"quota_exceeded refusals carry the pool and dimension in this error.",
		)
		return
	}

	var pr provisionResponse
	if err := mcp.ParseToolText("provision_server", text, &pr); err != nil {
		resp.Diagnostics.AddError("provision_server failed", err.Error())
		return
	}

	plan.RequestID = stringOrNull(pr.RequestID)
	plan.Status = types.StringValue(firstNonEmpty(pr.Outcome, pr.Status))
	plan.ProgressPct = types.Int64Value(pr.Progress)
	if pr.RequestID != "" {
		plan.ID = types.StringValue(pr.RequestID)
	} else {
		plan.ID = types.StringValue("provision:" + serverID)
	}

	// A delegated call carries no request_id — there is nothing to poll.
	if plan.WaitForCompletion.ValueBool() && pr.RequestID == "" && !plan.DryRun.ValueBool() {
		resp.Diagnostics.AddWarning(
			"Cannot wait on a delegated provision",
			fmt.Sprintf("provision_server returned status %q without a request_id (coordinator-delegated path); skipping wait_for_completion.", pr.Status),
		)
	}

	// Persist the accepted request before waiting: if the wait fails or times
	// out, Terraform retains a tainted record carrying request_id, so a retry
	// can see/cancel the in-flight request instead of queuing a duplicate.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.WaitForCompletion.ValueBool() && pr.RequestID != "" {
		final, err := waitForProvision(
			ctx, r.client, pr.RequestID,
			time.Duration(plan.WaitTimeout.ValueInt64())*time.Second,
			time.Duration(plan.PollInterval.ValueInt64())*time.Second,
		)
		if err != nil {
			resp.Diagnostics.AddError("Provisioning did not complete", err.Error())
			return
		}
		plan.Status = types.StringValue(final.Status)
		plan.ProgressPct = types.Int64Value(final.Progress)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		if provisionFailed(final.Status) {
			resp.Diagnostics.AddError(
				"Provisioning failed",
				fmt.Sprintf("request %s ended in status %q: %s", final.RequestID, final.Status, final.Error),
			)
		}
	}
}

func (r *ServerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ServerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A real request_id polls the provision status. Delegated records (and
	// dry-run rehearsals) have nothing to refresh — confirm the node still
	// exists instead.
	if rid := state.RequestID.ValueString(); rid != "" && !provisionTerminal(state.Status.ValueString()) {
		text, err := r.client.CallToolText(ctx, "get_provision_status", map[string]any{"request_id": rid})
		if err != nil {
			resp.Diagnostics.AddError("Failed to read provision status", err.Error())
			return
		}
		var st provisionStatusResponse
		if err := mcp.ParseToolText("get_provision_status", text, &st); err != nil {
			resp.Diagnostics.AddError("Failed to read provision status", err.Error())
			return
		}
		state.Status = types.StringValue(st.Status)
		state.ProgressPct = types.Int64Value(st.Progress)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}

	var node struct {
		ID string `json:"id"`
	}
	text, err := r.client.CallToolText(ctx, "get_node_details", map[string]any{"node_id": state.NodeID.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Failed to read node", err.Error())
		return
	}
	if err := mcp.ParseToolText("get_node_details", text, &node); err != nil {
		if isNotFoundMessage(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read node", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op: every actuation-affecting attribute RequiresReplace, so
// only approval tokens and wait settings can change in place.
func (r *ServerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ServerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state ServerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = state.ID
	plan.NodeID = state.NodeID
	plan.RequestID = state.RequestID
	plan.Status = state.Status
	plan.ProgressPct = state.ProgressPct
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ServerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rid := state.RequestID.ValueString()
	token := state.DestroyApprovalToken.ValueString()

	// Nothing to cancel when there is no request (delegated/dry-run record) or
	// the request already reached a terminal state.
	if rid == "" || provisionTerminal(state.Status.ValueString()) {
		return
	}
	if token == "" {
		resp.Diagnostics.AddWarning(
			"Provisioning request left running",
			fmt.Sprintf("No destroy_approval_token set — request %s was not canceled on the MOJO side; removing from state only. Mint a token with --tool cancel_provision and re-run destroy to cancel it.", rid),
		)
		return
	}

	text, err := r.client.CallToolText(ctx, "cancel_provision", map[string]any{
		"request_id": rid,
		"approval":   token,
	})
	if err != nil {
		resp.Diagnostics.AddError("cancel_provision failed", err.Error())
		return
	}

	// Handlers can answer inside the tool result, not the RPC error channel —
	// a plain-text refusal ("Request not found") must not read as success, or
	// Terraform would drop the only record of a still-running request.
	var res struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := mcp.ParseToolText("cancel_provision", text, &res); err != nil {
		resp.Diagnostics.AddError("cancel_provision refused", err.Error())
		return
	}
	switch strings.ToLower(res.Status) {
	case "cancelled", "canceled", "delegated": //nolint:misspell
		// first form: confirmed by the PXE engine; delegated: handed to the coordinator.
	default:
		resp.Diagnostics.AddError(
			"Unexpected cancel_provision response",
			fmt.Sprintf("status %q: %s — request %s left in state", res.Status, res.Message, rid),
		)
	}
}

// isNotFoundMessage treats a plain-text tool response naming "not found" as a
// missing object — mojo-mcp read tools signal absence as text, not JSON-RPC
// -32001.
func isNotFoundMessage(err error) bool {
	var mcpErr *mcp.MCPError
	if e, ok := err.(*mcp.MCPError); ok {
		mcpErr = e
	} else {
		return false
	}
	return strings.Contains(strings.ToLower(mcpErr.Message), "not found")
}

func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
