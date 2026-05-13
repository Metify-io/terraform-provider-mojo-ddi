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

var _ resource.Resource = &IPAddressResource{}
var _ resource.ResourceWithImportState = &IPAddressResource{}

func NewIPAddressResource() resource.Resource {
	return &IPAddressResource{}
}

type IPAddressResource struct {
	client *mcp.Client
}

type IPAddressResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Address     types.String `tfsdk:"address"`
	VRFID       types.String `tfsdk:"vrf_id"`
	Status      types.String `tfsdk:"status"`
	DNSName     types.String `tfsdk:"dns_name"`
	Description types.String `tfsdk:"description"`
}

type ipAddressAPIModel struct {
	ID          string `json:"id"`
	Address     string `json:"address"`
	VRFID       string `json:"vrf_id,omitempty"`
	Status      string `json:"status"`
	DNSName     string `json:"dns_name,omitempty"`
	Description string `json:"description,omitempty"`
}

func (r *IPAddressResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip_address"
}

func (r *IPAddressResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an allocated IP address in MOJO IPAM.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "MOJO-assigned UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "IP address in CIDR notation (e.g. `10.0.0.5/24`) or plain (e.g. `10.0.0.5`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"vrf_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "VRF this address belongs to.",
			},
			"status": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Allocation status: `active`, `reserved`, `deprecated`, `dhcp`. Defaults to `active`.",
			},
			"dns_name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "FQDN for automatic forward/reverse DNS registration.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Description.",
			},
		},
	}
}

func (r *IPAddressResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IPAddressResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IPAddressResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"address": plan.Address.ValueString(),
	}
	if v := plan.VRFID.ValueString(); v != "" {
		args["vrf_id"] = v
	}
	if v := plan.Status.ValueString(); v != "" {
		args["status"] = v
	}
	if v := plan.DNSName.ValueString(); v != "" {
		args["dns_name"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}

	var apiObj ipAddressAPIModel
	if err := r.client.CallToolJSON(ctx, "ipam.create_ip_address", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to create IP address", err.Error())
		return
	}

	ipAddressAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IPAddressResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IPAddressResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiObj ipAddressAPIModel
	err := r.client.CallToolJSON(ctx, "ipam.read_ip_address", map[string]any{"id": state.ID.ValueString()}, &apiObj)
	if err != nil {
		if mcp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read IP address", err.Error())
		return
	}

	ipAddressAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IPAddressResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan IPAddressResourceModel
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
	if v := plan.DNSName.ValueString(); v != "" {
		args["dns_name"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}

	var apiObj ipAddressAPIModel
	if err := r.client.CallToolJSON(ctx, "ipam.update_ip_address", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to update IP address", err.Error())
		return
	}

	ipAddressAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IPAddressResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IPAddressResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CallTool(ctx, "ipam.delete_ip_address", map[string]any{"id": state.ID.ValueString()})
	if err != nil {
		if mcp.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete IP address", err.Error())
	}
}

func (r *IPAddressResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var apiObj ipAddressAPIModel
	err := r.client.CallToolJSON(ctx, "ipam.read_ip_address", map[string]any{"id": req.ID}, &apiObj)
	if err != nil {
		resp.Diagnostics.AddError("Failed to import IP address", err.Error())
		return
	}

	var state IPAddressResourceModel
	ipAddressAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func ipAddressAPIToState(api *ipAddressAPIModel, state *IPAddressResourceModel) {
	state.ID = types.StringValue(api.ID)
	state.Address = types.StringValue(api.Address)
	state.VRFID = types.StringValue(api.VRFID)
	state.Status = types.StringValue(api.Status)
	state.DNSName = types.StringValue(api.DNSName)
	state.Description = types.StringValue(api.Description)
}
