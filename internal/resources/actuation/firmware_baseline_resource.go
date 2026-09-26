// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package actuation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Metify-io/terraform-provider-mojo-ddi/internal/mcp"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &FirmwareBaselineResource{}
var _ resource.ResourceWithValidateConfig = &FirmwareBaselineResource{}

func NewFirmwareBaselineResource() resource.Resource {
	return &FirmwareBaselineResource{}
}

// FirmwareBaselineResource implements the mojo_firmware_baseline resource:
// a baseline-to-node convergence binding driven through `apply_baseline`
// (destructive, token-gated) and refreshed via `get_baseline_status`.
type FirmwareBaselineResource struct {
	client *mcp.Client
}

type FirmwareBaselineResourceModel struct {
	ID               types.String `tfsdk:"id"`
	SerialNumber     types.String `tfsdk:"serial_number"`
	ServerID         types.String `tfsdk:"server_id"`
	NodeID           types.String `tfsdk:"node_id"`
	BaselineID       types.String `tfsdk:"baseline_id"`
	ApprovalToken    types.String `tfsdk:"approval_token"`
	PlanJSON         types.String `tfsdk:"plan_json"`
	DryRun           types.Bool   `tfsdk:"dry_run"`
	ComplianceStatus types.String `tfsdk:"compliance_status"`
	LastEvaluatedAt  types.String `tfsdk:"last_evaluated_at"`
	RuleCount        types.Int64  `tfsdk:"rule_count"`
}

func (r *FirmwareBaselineResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firmware_baseline"
}

func (r *FirmwareBaselineResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Converges a node's firmware/BIOS state to a baseline via the governed ` + "`apply_baseline`" + ` MCP tool (actuation class: destructive).

A live apply requires a single-use approval token minted on the MOJO box:

` + "```sh" + `
podman exec mojo-app python /app/coordinator-django/manage.py mint_approval \
  --tool apply_baseline --principal agent:<name> \
  --on-behalf-of <approver-email> --ttl 3600 --target serial:<SERIAL>
` + "```" + `

Supply the token via ` + "`approval_token`" + ` per apply (tfvars or ` + "`-var`" + `). ` + "`terraform refresh`" + `/plan read ` + "`get_baseline_status`" + ` (read-only, no hardware call); the ` + "`mojo_baseline_evaluation`" + ` data source runs a live ` + "`evaluate_baseline`" + ` to show drift. ` + "`dry_run = true`" + ` rehearses through all gates without actuating.

Destroy removes the binding from state only — a baseline cannot be un-applied.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`baseline:<server_id>:<baseline_id>`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"serial_number": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Chassis serial number of the node (resolved via `search_nodes`). Exactly one of `serial_number`/`server_id` must be set.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"server_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Pin a MOJO node UUID explicitly instead of resolving `serial_number`. When both are set they must agree.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"node_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resolved MOJO node UUID the baseline applies to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"baseline_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Baseline UUID to converge the node to. Part of the resource ID, so changing it replaces the binding.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"approval_token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Coordinator-minted approval token (`v1.<payload>.<sig>`) for the `apply_baseline` call — required on a live apply of this destructive tool, consumed single-use.",
			},
			"plan_json": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "JSON plan document presented verbatim as the `plan` argument on `apply_baseline`. Required when the approval token was minted with `--plan-hash`: the boundary re-hashes the plan canonically and refuses the call unless it matches the hash the approver signed.",
			},
			"dry_run": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Rehearse `apply_baseline` through every gate without actuating (the ledger intent closes `planned`). Changing this replaces the resource so a rehearsal never silently masquerades as a live converge.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"compliance_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Latest stored evaluation for this node/baseline pair (`compliant`, `non_compliant`, `no_evaluation`).",
			},
			"last_evaluated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC 3339 timestamp of the latest stored evaluation.",
			},
			"rule_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of baseline rules in the latest evaluation.",
			},
		},
	}
}

func (r *FirmwareBaselineResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config FirmwareBaselineResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.SerialNumber.IsNull() && config.ServerID.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("serial_number"),
			"Missing node identifier",
			"Set serial_number or server_id to identify the node the baseline applies to.",
		)
	}
}

func (r *FirmwareBaselineResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *FirmwareBaselineResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FirmwareBaselineResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve from configuration, not plan: a computed server_id carried over
	// from prior state must not pin a replacement to the previous node.
	var config FirmwareBaselineResourceModel
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

	if !r.apply(ctx, &plan, &resp.Diagnostics) {
		return
	}
	r.refreshCompliance(ctx, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *FirmwareBaselineResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FirmwareBaselineResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Confirm the node still exists; a stale baseline binding on a deleted
	// node is drift worth surfacing as removal.
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

	r.refreshCompliance(ctx, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only carries approval tokens forward — every actuation-affecting
// attribute (serial_number, server_id, baseline_id, dry_run) RequiresReplace,
// so Update never re-issues apply_baseline.
func (r *FirmwareBaselineResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FirmwareBaselineResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state FirmwareBaselineResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID
	plan.NodeID = state.NodeID
	plan.ComplianceStatus = state.ComplianceStatus
	plan.LastEvaluatedAt = state.LastEvaluatedAt
	plan.RuleCount = state.RuleCount
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the binding from state; nothing is un-applied.
func (r *FirmwareBaselineResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *FirmwareBaselineResource) apply(ctx context.Context, plan *FirmwareBaselineResourceModel, diags *diag.Diagnostics) bool {
	serverID := plan.NodeID.ValueString()

	args := map[string]any{
		"server_id":   serverID,
		"baseline_id": plan.BaselineID.ValueString(),
	}
	if v := plan.ApprovalToken.ValueString(); v != "" {
		args["approval"] = v
	}
	if plan.DryRun.ValueBool() {
		args["dry_run"] = true
	}
	if v := plan.PlanJSON.ValueString(); v != "" {
		var doc any
		if err := json.Unmarshal([]byte(v), &doc); err != nil {
			diags.AddError("Invalid plan_json", fmt.Sprintf("plan_json is not valid JSON: %s", err))
			return false
		}
		args["plan"] = doc
	}

	text, err := r.client.CallToolText(ctx, "apply_baseline", args)
	if err != nil {
		diags.AddError(
			"apply_baseline call refused or failed",
			err.Error()+"\n\napply_baseline is destructive: mint a token on the MOJO box "+
				"(manage.py mint_approval --tool apply_baseline --target serial:<SERIAL>) "+
				"or set dry_run = true to rehearse without actuating.",
		)
		return false
	}

	var ar baselineApplyResponse
	if err := mcp.ParseToolText("apply_baseline", text, &ar); err != nil {
		diags.AddError("apply_baseline failed", err.Error())
		return false
	}

	plan.ID = types.StringValue(fmt.Sprintf("baseline:%s:%s", serverID, plan.BaselineID.ValueString()))
	return true
}

// refreshCompliance reads the latest stored evaluation via
// `get_baseline_status` — a read-only DB lookup, no hardware call.
func (r *FirmwareBaselineResource) refreshCompliance(ctx context.Context, model *FirmwareBaselineResourceModel) {
	var st baselineStatusResponse
	text, err := r.client.CallToolText(ctx, "get_baseline_status", map[string]any{
		"server_id": model.NodeID.ValueString(),
	})
	if err != nil {
		model.ComplianceStatus = types.StringValue("unknown")
		model.LastEvaluatedAt = types.StringNull()
		model.RuleCount = types.Int64Null()
		return
	}
	if err := mcp.ParseToolText("get_baseline_status", text, &st); err != nil {
		model.ComplianceStatus = types.StringValue("unknown")
		model.LastEvaluatedAt = types.StringNull()
		model.RuleCount = types.Int64Null()
		return
	}

	for _, ev := range st.Evaluations {
		if ev.BaselineID == model.BaselineID.ValueString() {
			model.ComplianceStatus = types.StringValue(ev.OverallStatus)
			model.LastEvaluatedAt = types.StringValue(ev.EvaluatedAt)
			model.RuleCount = types.Int64Value(ev.RuleCount)
			return
		}
	}
	model.ComplianceStatus = types.StringValue("no_evaluation")
	model.LastEvaluatedAt = types.StringNull()
	model.RuleCount = types.Int64Null()
}
