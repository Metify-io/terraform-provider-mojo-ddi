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

var _ resource.Resource = &PrefixResource{}
var _ resource.ResourceWithImportState = &PrefixResource{}

func NewPrefixResource() resource.Resource {
	return &PrefixResource{}
}

type PrefixResource struct {
	client *mcp.Client
}

type PrefixResourceModel struct {
	ID          types.String `tfsdk:"id"`
	CIDR        types.String `tfsdk:"cidr"`
	VRFID       types.String `tfsdk:"vrf_id"`
	VLANID      types.String `tfsdk:"vlan_id"`
	Description types.String `tfsdk:"description"`
	IsPool      types.Bool   `tfsdk:"is_pool"`
}

type prefixAPIModel struct {
	ID          string `json:"id"`
	CIDR        string `json:"cidr"`
	VRFID       string `json:"vrf_id,omitempty"`
	VLANID      string `json:"vlan_id,omitempty"`
	Description string `json:"description,omitempty"`
	IsPool      bool   `json:"is_pool"`
}

func (r *PrefixResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_prefix"
}

func (r *PrefixResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a MOJO IP prefix. Prefixes auto-nest under existing parent prefixes by CIDR.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "MOJO-assigned UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cidr": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "CIDR notation (e.g. `10.0.0.0/24`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"vrf_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "VRF this prefix belongs to.",
			},
			"vlan_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "VLAN this prefix is tied to.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Description.",
			},
			"is_pool": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Mark this prefix as an address pool (for DHCP, etc.).",
			},
		},
	}
}

func (r *PrefixResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PrefixResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PrefixResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"cidr": plan.CIDR.ValueString(),
	}
	if v := plan.VRFID.ValueString(); v != "" {
		args["vrf_id"] = v
	}
	if v := plan.VLANID.ValueString(); v != "" {
		args["vlan_id"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}
	if plan.IsPool.IsNull() || plan.IsPool.IsUnknown() {
		// let server default
	} else {
		args["is_pool"] = plan.IsPool.ValueBool()
	}

	var apiObj prefixAPIModel
	if err := r.client.CallToolJSON(ctx, "ipam.create_prefix", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to create prefix", err.Error())
		return
	}

	prefixAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PrefixResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PrefixResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiObj prefixAPIModel
	err := r.client.CallToolJSON(ctx, "ipam.read_prefix", map[string]any{"id": state.ID.ValueString()}, &apiObj)
	if err != nil {
		if mcp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read prefix", err.Error())
		return
	}

	prefixAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PrefixResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PrefixResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"id": plan.ID.ValueString(),
	}
	if v := plan.VRFID.ValueString(); v != "" {
		args["vrf_id"] = v
	}
	if v := plan.VLANID.ValueString(); v != "" {
		args["vlan_id"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}
	if !plan.IsPool.IsNull() && !plan.IsPool.IsUnknown() {
		args["is_pool"] = plan.IsPool.ValueBool()
	}

	var apiObj prefixAPIModel
	if err := r.client.CallToolJSON(ctx, "ipam.update_prefix", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to update prefix", err.Error())
		return
	}

	prefixAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PrefixResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PrefixResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CallTool(ctx, "ipam.delete_prefix", map[string]any{"id": state.ID.ValueString()})
	if err != nil {
		if mcp.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete prefix", err.Error())
	}
}

func (r *PrefixResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var apiObj prefixAPIModel
	err := r.client.CallToolJSON(ctx, "ipam.read_prefix", map[string]any{"id": req.ID}, &apiObj)
	if err != nil {
		resp.Diagnostics.AddError("Failed to import prefix", err.Error())
		return
	}

	var state PrefixResourceModel
	prefixAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func prefixAPIToState(api *prefixAPIModel, state *PrefixResourceModel) {
	state.ID = types.StringValue(api.ID)
	state.CIDR = types.StringValue(api.CIDR)
	state.VRFID = types.StringValue(api.VRFID)
	state.VLANID = types.StringValue(api.VLANID)
	state.Description = types.StringValue(api.Description)
	state.IsPool = types.BoolValue(api.IsPool)
}
