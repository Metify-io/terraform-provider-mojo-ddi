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

var _ resource.Resource = &DHCPScopeResource{}
var _ resource.ResourceWithImportState = &DHCPScopeResource{}

func NewDHCPScopeResource() resource.Resource {
	return &DHCPScopeResource{}
}

type DHCPScopeResource struct {
	client *mcp.Client
}

type DHCPScopeResourceModel struct {
	ID          types.String `tfsdk:"id"`
	PrefixID    types.String `tfsdk:"prefix_id"`
	LeaseTime   types.Int64  `tfsdk:"lease_time"`
	Gateway     types.String `tfsdk:"gateway"`
	DNSServers  types.String `tfsdk:"dns_servers"`
	DomainName  types.String `tfsdk:"domain_name"`
	PXEEnabled  types.Bool   `tfsdk:"pxe_enabled"`
	PXEServer   types.String `tfsdk:"pxe_server"`
	PXEFilename types.String `tfsdk:"pxe_filename"`
	Description types.String `tfsdk:"description"`
}

type dhcpScopeAPIModel struct {
	ID          string `json:"id"`
	PrefixID    string `json:"prefix_id"`
	LeaseTime   int64  `json:"lease_time"`
	Gateway     string `json:"gateway,omitempty"`
	DNSServers  string `json:"dns_servers,omitempty"`
	DomainName  string `json:"domain_name,omitempty"`
	PXEEnabled  bool   `json:"pxe_enabled"`
	PXEServer   string `json:"pxe_server,omitempty"`
	PXEFilename string `json:"pxe_filename,omitempty"`
	Description string `json:"description,omitempty"`
}

func (r *DHCPScopeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dhcp_scope"
}

func (r *DHCPScopeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Kea DHCP scope bound to a MOJO prefix.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "MOJO-assigned UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"prefix_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "MOJO prefix UUID this scope serves.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"lease_time": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Default lease time in seconds. Defaults to `3600`.",
			},
			"gateway": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Default gateway (option routers).",
			},
			"dns_servers": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Comma-separated DNS servers (option domain-name-servers).",
			},
			"domain_name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Domain name pushed to DHCP clients.",
			},
			"pxe_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Enable PXE boot options. Defaults to `false`.",
			},
			"pxe_server": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "TFTP/HTTP server for PXE boot. Required if `pxe_enabled` is `true`.",
			},
			"pxe_filename": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "PXE boot filename (e.g. `ipxe.efi`). Required if `pxe_enabled` is `true`.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Description.",
			},
		},
	}
}

func (r *DHCPScopeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DHCPScopeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DHCPScopeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"prefix_id": plan.PrefixID.ValueString(),
	}
	if !plan.LeaseTime.IsNull() && !plan.LeaseTime.IsUnknown() {
		args["lease_time"] = plan.LeaseTime.ValueInt64()
	}
	if v := plan.Gateway.ValueString(); v != "" {
		args["gateway"] = v
	}
	if v := plan.DNSServers.ValueString(); v != "" {
		args["dns_servers"] = v
	}
	if v := plan.DomainName.ValueString(); v != "" {
		args["domain_name"] = v
	}
	if !plan.PXEEnabled.IsNull() && !plan.PXEEnabled.IsUnknown() {
		args["pxe_enabled"] = plan.PXEEnabled.ValueBool()
	}
	if v := plan.PXEServer.ValueString(); v != "" {
		args["pxe_server"] = v
	}
	if v := plan.PXEFilename.ValueString(); v != "" {
		args["pxe_filename"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}

	var apiObj dhcpScopeAPIModel
	if err := r.client.CallToolJSON(ctx, "dhcp.create_scope", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to create DHCP scope", err.Error())
		return
	}

	dhcpScopeAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DHCPScopeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DHCPScopeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiObj dhcpScopeAPIModel
	err := r.client.CallToolJSON(ctx, "dhcp.read_scope", map[string]any{"id": state.ID.ValueString()}, &apiObj)
	if err != nil {
		if mcp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read DHCP scope", err.Error())
		return
	}

	dhcpScopeAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DHCPScopeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DHCPScopeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"id": plan.ID.ValueString(),
	}
	if !plan.LeaseTime.IsNull() && !plan.LeaseTime.IsUnknown() {
		args["lease_time"] = plan.LeaseTime.ValueInt64()
	}
	if v := plan.Gateway.ValueString(); v != "" {
		args["gateway"] = v
	}
	if v := plan.DNSServers.ValueString(); v != "" {
		args["dns_servers"] = v
	}
	if v := plan.DomainName.ValueString(); v != "" {
		args["domain_name"] = v
	}
	if !plan.PXEEnabled.IsNull() && !plan.PXEEnabled.IsUnknown() {
		args["pxe_enabled"] = plan.PXEEnabled.ValueBool()
	}
	if v := plan.PXEServer.ValueString(); v != "" {
		args["pxe_server"] = v
	}
	if v := plan.PXEFilename.ValueString(); v != "" {
		args["pxe_filename"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}

	var apiObj dhcpScopeAPIModel
	if err := r.client.CallToolJSON(ctx, "dhcp.update_scope", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to update DHCP scope", err.Error())
		return
	}

	dhcpScopeAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DHCPScopeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DHCPScopeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CallTool(ctx, "dhcp.delete_scope", map[string]any{"id": state.ID.ValueString()})
	if err != nil {
		if mcp.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete DHCP scope", err.Error())
	}
}

func (r *DHCPScopeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var apiObj dhcpScopeAPIModel
	err := r.client.CallToolJSON(ctx, "dhcp.read_scope", map[string]any{"id": req.ID}, &apiObj)
	if err != nil {
		resp.Diagnostics.AddError("Failed to import DHCP scope", err.Error())
		return
	}

	var state DHCPScopeResourceModel
	dhcpScopeAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func dhcpScopeAPIToState(api *dhcpScopeAPIModel, state *DHCPScopeResourceModel) {
	state.ID = types.StringValue(api.ID)
	state.PrefixID = types.StringValue(api.PrefixID)
	state.LeaseTime = types.Int64Value(api.LeaseTime)
	state.Gateway = types.StringValue(api.Gateway)
	state.DNSServers = types.StringValue(api.DNSServers)
	state.DomainName = types.StringValue(api.DomainName)
	state.PXEEnabled = types.BoolValue(api.PXEEnabled)
	state.PXEServer = types.StringValue(api.PXEServer)
	state.PXEFilename = types.StringValue(api.PXEFilename)
	state.Description = types.StringValue(api.Description)
}
