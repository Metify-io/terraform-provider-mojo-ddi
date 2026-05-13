// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package ipamresources

import (
	"context"
	"fmt"

	"github.com/Metify-io/terraform-provider-mojo-ddi/internal/mcp"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &VLANResource{}
var _ resource.ResourceWithImportState = &VLANResource{}

func NewVLANResource() resource.Resource {
	return &VLANResource{}
}

type VLANResource struct {
	client *mcp.Client
}

type VLANResourceModel struct {
	ID          types.String `tfsdk:"id"`
	VID         types.Int64  `tfsdk:"vid"`
	Name        types.String `tfsdk:"name"`
	SiteID      types.String `tfsdk:"site_id"`
	Description types.String `tfsdk:"description"`
}

type vlanAPIModel struct {
	ID          string `json:"id"`
	VID         int64  `json:"vid"`
	Name        string `json:"name"`
	SiteID      string `json:"site_id,omitempty"`
	Description string `json:"description,omitempty"`
}

func (r *VLANResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vlan"
}

func (r *VLANResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a MOJO VLAN definition.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "MOJO-assigned UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"vid": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "VLAN ID (1-4094).",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Human-readable VLAN name.",
			},
			"site_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Site UUID this VLAN belongs to.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Description.",
			},
		},
	}
}

func (r *VLANResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *VLANResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VLANResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"vid":  plan.VID.ValueInt64(),
		"name": plan.Name.ValueString(),
	}
	if v := plan.SiteID.ValueString(); v != "" {
		args["site_id"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}

	var apiObj vlanAPIModel
	if err := r.client.CallToolJSON(ctx, "ipam.create_vlan", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to create VLAN", err.Error())
		return
	}

	vlanAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VLANResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VLANResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiObj vlanAPIModel
	err := r.client.CallToolJSON(ctx, "ipam.read_vlan", map[string]any{"id": state.ID.ValueString()}, &apiObj)
	if err != nil {
		if mcp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read VLAN", err.Error())
		return
	}

	vlanAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VLANResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan VLANResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"id":   plan.ID.ValueString(),
		"name": plan.Name.ValueString(),
	}
	if v := plan.SiteID.ValueString(); v != "" {
		args["site_id"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}

	var apiObj vlanAPIModel
	if err := r.client.CallToolJSON(ctx, "ipam.update_vlan", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to update VLAN", err.Error())
		return
	}

	vlanAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VLANResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VLANResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CallTool(ctx, "ipam.delete_vlan", map[string]any{"id": state.ID.ValueString()})
	if err != nil {
		if mcp.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete VLAN", err.Error())
	}
}

func (r *VLANResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var apiObj vlanAPIModel
	err := r.client.CallToolJSON(ctx, "ipam.read_vlan", map[string]any{"id": req.ID}, &apiObj)
	if err != nil {
		resp.Diagnostics.AddError("Failed to import VLAN", err.Error())
		return
	}

	var state VLANResourceModel
	vlanAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func vlanAPIToState(api *vlanAPIModel, state *VLANResourceModel) {
	state.ID = types.StringValue(api.ID)
	state.VID = types.Int64Value(api.VID)
	state.Name = types.StringValue(api.Name)
	state.SiteID = types.StringValue(api.SiteID)
	state.Description = types.StringValue(api.Description)
}
