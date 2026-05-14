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

var _ resource.Resource = &DNSZoneResource{}
var _ resource.ResourceWithImportState = &DNSZoneResource{}

func NewDNSZoneResource() resource.Resource {
	return &DNSZoneResource{}
}

type DNSZoneResource struct {
	client *mcp.Client
}

type DNSZoneResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Kind        types.String `tfsdk:"kind"`
	Nameservers types.String `tfsdk:"nameservers"`
	Description types.String `tfsdk:"description"`
}

type dnsZoneAPIModel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Nameservers string `json:"nameservers,omitempty"`
	Description string `json:"description,omitempty"`
}

func (r *DNSZoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_zone"
}

func (r *DNSZoneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a PowerDNS zone in MOJO.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "MOJO-assigned UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Zone name (e.g. `example.com` or `0.168.192.in-addr.arpa` for reverse).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"kind": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Zone kind: `Native`, `Master`, `Slave`. Defaults to `Native`.",
			},
			"nameservers": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Comma-separated nameserver FQDNs.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Description.",
			},
		},
	}
}

func (r *DNSZoneResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DNSZoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DNSZoneResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"name": plan.Name.ValueString(),
	}
	if v := plan.Kind.ValueString(); v != "" {
		args["kind"] = v
	}
	if v := plan.Nameservers.ValueString(); v != "" {
		args["nameservers"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}

	var apiObj dnsZoneAPIModel
	if err := r.client.CallToolJSON(ctx, "dns.create_zone", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to create DNS zone", err.Error())
		return
	}

	dnsZoneAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DNSZoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DNSZoneResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiObj dnsZoneAPIModel
	err := r.client.CallToolJSON(ctx, "dns.read_zone", map[string]any{"id": state.ID.ValueString()}, &apiObj)
	if err != nil {
		if mcp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read DNS zone", err.Error())
		return
	}

	dnsZoneAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DNSZoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DNSZoneResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"id": plan.ID.ValueString(),
	}
	if v := plan.Kind.ValueString(); v != "" {
		args["kind"] = v
	}
	if v := plan.Nameservers.ValueString(); v != "" {
		args["nameservers"] = v
	}
	if v := plan.Description.ValueString(); v != "" {
		args["description"] = v
	}

	var apiObj dnsZoneAPIModel
	if err := r.client.CallToolJSON(ctx, "dns.update_zone", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to update DNS zone", err.Error())
		return
	}

	dnsZoneAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DNSZoneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DNSZoneResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CallTool(ctx, "dns.delete_zone", map[string]any{"id": state.ID.ValueString()})
	if err != nil {
		if mcp.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete DNS zone", err.Error())
	}
}

func (r *DNSZoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var apiObj dnsZoneAPIModel
	err := r.client.CallToolJSON(ctx, "dns.read_zone", map[string]any{"id": req.ID}, &apiObj)
	if err != nil {
		resp.Diagnostics.AddError("Failed to import DNS zone", err.Error())
		return
	}

	var state DNSZoneResourceModel
	dnsZoneAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func dnsZoneAPIToState(api *dnsZoneAPIModel, state *DNSZoneResourceModel) {
	state.ID = types.StringValue(api.ID)
	state.Name = types.StringValue(api.Name)
	state.Kind = types.StringValue(api.Kind)
	state.Nameservers = types.StringValue(api.Nameservers)
	state.Description = types.StringValue(api.Description)
}
