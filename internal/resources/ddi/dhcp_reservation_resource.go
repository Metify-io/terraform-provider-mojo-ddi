// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package ddiresources

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

var _ resource.Resource = &DHCPReservationResource{}
var _ resource.ResourceWithImportState = &DHCPReservationResource{}

func NewDHCPReservationResource() resource.Resource {
	return &DHCPReservationResource{}
}

type DHCPReservationResource struct {
	client *mcp.Client
}

type DHCPReservationResourceModel struct {
	ID          types.String `tfsdk:"id"`
	ScopeID     types.String `tfsdk:"scope_id"`
	MACAddress  types.String `tfsdk:"mac_address"`
	IPAddress   types.String `tfsdk:"ip_address"`
	Hostname    types.String `tfsdk:"hostname"`
	Description types.String `tfsdk:"description"`
}

type dhcpReservationAPIModel struct {
	ID          string `json:"id"`
	ScopeID     string `json:"scope_id"`
	MACAddress  string `json:"mac_address"`
	IPAddress   string `json:"ip_address"`
	Hostname    string `json:"hostname,omitempty"`
	Description string `json:"description,omitempty"`
}

func (r *DHCPReservationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dhcp_reservation"
}

func (r *DHCPReservationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a static DHCP reservation (MAC-to-IP binding) in Kea.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "MOJO-assigned UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"scope_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "DHCP scope this reservation belongs to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"mac_address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Client MAC address (e.g. `aa:bb:cc:dd:ee:ff`).",
			},
			"ip_address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Reserved IP address.",
			},
			"hostname": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Hostname to assign to the client.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Description.",
			},
		},
	}
}

func (r *DHCPReservationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DHCPReservationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DHCPReservationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"scope_id":    plan.ScopeID.ValueString(),
		"mac_address": plan.MACAddress.ValueString(),
		"ip_address":  plan.IPAddress.ValueString(),
	}
	if v := plan.Hostname.ValueString(); v != "" {
		args["hostname"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}

	var apiObj dhcpReservationAPIModel
	if err := r.client.CallToolJSON(ctx, "dhcp.create_reservation", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to create DHCP reservation", err.Error())
		return
	}

	dhcpReservationAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DHCPReservationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DHCPReservationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiObj dhcpReservationAPIModel
	err := r.client.CallToolJSON(ctx, "dhcp.read_reservation", map[string]any{"id": state.ID.ValueString()}, &apiObj)
	if err != nil {
		if mcp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read DHCP reservation", err.Error())
		return
	}

	dhcpReservationAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DHCPReservationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DHCPReservationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"id":          plan.ID.ValueString(),
		"mac_address": plan.MACAddress.ValueString(),
		"ip_address":  plan.IPAddress.ValueString(),
	}
	if v := plan.Hostname.ValueString(); v != "" {
		args["hostname"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}

	var apiObj dhcpReservationAPIModel
	if err := r.client.CallToolJSON(ctx, "dhcp.update_reservation", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to update DHCP reservation", err.Error())
		return
	}

	dhcpReservationAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DHCPReservationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DHCPReservationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CallTool(ctx, "dhcp.delete_reservation", map[string]any{"id": state.ID.ValueString()})
	if err != nil {
		if mcp.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete DHCP reservation", err.Error())
	}
}

func (r *DHCPReservationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var apiObj dhcpReservationAPIModel
	err := r.client.CallToolJSON(ctx, "dhcp.read_reservation", map[string]any{"id": req.ID}, &apiObj)
	if err != nil {
		resp.Diagnostics.AddError("Failed to import DHCP reservation", err.Error())
		return
	}

	var state DHCPReservationResourceModel
	dhcpReservationAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func dhcpReservationAPIToState(api *dhcpReservationAPIModel, state *DHCPReservationResourceModel) {
	state.ID = types.StringValue(api.ID)
	state.ScopeID = types.StringValue(api.ScopeID)
	state.MACAddress = types.StringValue(api.MACAddress)
	state.IPAddress = types.StringValue(api.IPAddress)
	state.Hostname = types.StringValue(api.Hostname)
	state.Description = types.StringValue(api.Description)
}
