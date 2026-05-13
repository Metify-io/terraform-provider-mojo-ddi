// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package ipamresources

import (
	"context"
	"fmt"

	"github.com/Metify-io/terraform-provider-mojo-ddi/internal/mcp"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &IPRangeResource{}
var _ resource.ResourceWithImportState = &IPRangeResource{}

func NewIPRangeResource() resource.Resource {
	return &IPRangeResource{}
}

type IPRangeResource struct {
	client *mcp.Client
}

type IPRangeResourceModel struct {
	ID           types.String `tfsdk:"id"`
	StartAddress types.String `tfsdk:"start_address"`
	EndAddress   types.String `tfsdk:"end_address"`
	VRFID        types.String `tfsdk:"vrf_id"`
	Status       types.String `tfsdk:"status"`
	Description  types.String `tfsdk:"description"`
}

type ipRangeAPIModel struct {
	ID           string `json:"id"`
	StartAddress string `json:"start_address"`
	EndAddress   string `json:"end_address"`
	VRFID        string `json:"vrf_id,omitempty"`
	Status       string `json:"status"`
	Description  string `json:"description,omitempty"`
}

func (r *IPRangeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip_range"
}

func (r *IPRangeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a contiguous IP range within a MOJO prefix.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "MOJO-assigned UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"start_address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "First address in the range (e.g. `10.0.0.100`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"end_address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Last address in the range (e.g. `10.0.0.200`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"vrf_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "VRF this range belongs to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"status": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Status: `active`, `reserved`, `deprecated`. Defaults to `active`.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Description.",
			},
		},
	}
}

func (r *IPRangeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IPRangeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IPRangeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"start_address": plan.StartAddress.ValueString(),
		"end_address":   plan.EndAddress.ValueString(),
	}
	if v := plan.VRFID.ValueString(); v != "" {
		args["vrf_id"] = v
	}
	if v := plan.Status.ValueString(); v != "" {
		args["status"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}

	var apiObj ipRangeAPIModel
	if err := r.client.CallToolJSON(ctx, "ipam.create_ip_range", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to create IP range", err.Error())
		return
	}

	ipRangeAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IPRangeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IPRangeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiObj ipRangeAPIModel
	err := r.client.CallToolJSON(ctx, "ipam.read_ip_range", map[string]any{"id": state.ID.ValueString()}, &apiObj)
	if err != nil {
		if mcp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read IP range", err.Error())
		return
	}

	ipRangeAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IPRangeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan IPRangeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"id": plan.ID.ValueString(),
	}
	if v := plan.Status.ValueString(); v != "" {
		args["status"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}

	var apiObj ipRangeAPIModel
	if err := r.client.CallToolJSON(ctx, "ipam.update_ip_range", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to update IP range", err.Error())
		return
	}

	ipRangeAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IPRangeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IPRangeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CallTool(ctx, "ipam.delete_ip_range", map[string]any{"id": state.ID.ValueString()})
	if err != nil {
		if mcp.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete IP range", err.Error())
	}
}

func (r *IPRangeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var apiObj ipRangeAPIModel
	err := r.client.CallToolJSON(ctx, "ipam.read_ip_range", map[string]any{"id": req.ID}, &apiObj)
	if err != nil {
		resp.Diagnostics.AddError("Failed to import IP range", err.Error())
		return
	}

	var state IPRangeResourceModel
	ipRangeAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func ipRangeAPIToState(api *ipRangeAPIModel, state *IPRangeResourceModel) {
	state.ID = types.StringValue(api.ID)
	state.StartAddress = types.StringValue(api.StartAddress)
	state.EndAddress = types.StringValue(api.EndAddress)
	state.VRFID = types.StringValue(api.VRFID)
	state.Status = types.StringValue(api.Status)
	state.Description = types.StringValue(api.Description)
}
