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

var _ resource.Resource = &DNSRecordResource{}
var _ resource.ResourceWithImportState = &DNSRecordResource{}

func NewDNSRecordResource() resource.Resource {
	return &DNSRecordResource{}
}

type DNSRecordResource struct {
	client *mcp.Client
}

type DNSRecordResourceModel struct {
	ID         types.String `tfsdk:"id"`
	ZoneID     types.String `tfsdk:"zone_id"`
	Name       types.String `tfsdk:"name"`
	RecordType types.String `tfsdk:"record_type"`
	Value      types.String `tfsdk:"value"`
	TTL        types.Int64  `tfsdk:"ttl"`
	Priority   types.Int64  `tfsdk:"priority"`
}

type dnsRecordAPIModel struct {
	ID         string `json:"id"`
	ZoneID     string `json:"zone_id"`
	Name       string `json:"name"`
	RecordType string `json:"record_type"`
	Value      string `json:"value"`
	TTL        int64  `json:"ttl"`
	Priority   int64  `json:"priority,omitempty"`
}

func (r *DNSRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *DNSRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DNS record within a PowerDNS zone.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "MOJO-assigned UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"zone_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "DNS zone this record belongs to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record name (e.g. `www`, `@` for apex, or FQDN).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"record_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record type: `A`, `AAAA`, `CNAME`, `PTR`, `MX`, `TXT`, `SRV`, `NS`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"value": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record value (e.g. IP address, hostname, TXT content).",
			},
			"ttl": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Time to live in seconds. Defaults to `300`.",
			},
			"priority": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Priority for MX and SRV records.",
			},
		},
	}
}

func (r *DNSRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DNSRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DNSRecordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"zone_id":     plan.ZoneID.ValueString(),
		"name":        plan.Name.ValueString(),
		"record_type": plan.RecordType.ValueString(),
		"value":       plan.Value.ValueString(),
	}
	if !plan.TTL.IsNull() && !plan.TTL.IsUnknown() {
		args["ttl"] = plan.TTL.ValueInt64()
	}
	if !plan.Priority.IsNull() && !plan.Priority.IsUnknown() {
		args["priority"] = plan.Priority.ValueInt64()
	}

	var apiObj dnsRecordAPIModel
	if err := r.client.CallToolJSON(ctx, "dns.create_record", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to create DNS record", err.Error())
		return
	}

	dnsRecordAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DNSRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DNSRecordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiObj dnsRecordAPIModel
	err := r.client.CallToolJSON(ctx, "dns.read_record", map[string]any{"id": state.ID.ValueString()}, &apiObj)
	if err != nil {
		if mcp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read DNS record", err.Error())
		return
	}

	dnsRecordAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DNSRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DNSRecordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"id":    plan.ID.ValueString(),
		"value": plan.Value.ValueString(),
	}
	if !plan.TTL.IsNull() && !plan.TTL.IsUnknown() {
		args["ttl"] = plan.TTL.ValueInt64()
	}
	if !plan.Priority.IsNull() && !plan.Priority.IsUnknown() {
		args["priority"] = plan.Priority.ValueInt64()
	}

	var apiObj dnsRecordAPIModel
	if err := r.client.CallToolJSON(ctx, "dns.update_record", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to update DNS record", err.Error())
		return
	}

	dnsRecordAPIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DNSRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DNSRecordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CallTool(ctx, "dns.delete_record", map[string]any{"id": state.ID.ValueString()})
	if err != nil {
		if mcp.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete DNS record", err.Error())
	}
}

func (r *DNSRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var apiObj dnsRecordAPIModel
	err := r.client.CallToolJSON(ctx, "dns.read_record", map[string]any{"id": req.ID}, &apiObj)
	if err != nil {
		resp.Diagnostics.AddError("Failed to import DNS record", err.Error())
		return
	}

	var state DNSRecordResourceModel
	dnsRecordAPIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func dnsRecordAPIToState(api *dnsRecordAPIModel, state *DNSRecordResourceModel) {
	state.ID = types.StringValue(api.ID)
	state.ZoneID = types.StringValue(api.ZoneID)
	state.Name = types.StringValue(api.Name)
	state.RecordType = types.StringValue(api.RecordType)
	state.Value = types.StringValue(api.Value)
	state.TTL = types.Int64Value(api.TTL)
	state.Priority = types.Int64Value(api.Priority)
}
