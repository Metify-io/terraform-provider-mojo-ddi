// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package ipamresources implements Terraform managed resources for MOJO IPAM objects.
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

// Ensure VRFResource satisfies the resource.Resource interface.
var _ resource.Resource = &VRFResource{}
var _ resource.ResourceWithImportState = &VRFResource{}

// NewVRFResource is the factory function registered in the provider.
func NewVRFResource() resource.Resource {
	return &VRFResource{}
}

// VRFResource manages a mojo_vrf resource.
type VRFResource struct {
	client *mcp.Client
}

// VRFResourceModel maps the Terraform schema to Go types.
type VRFResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	RD          types.String `tfsdk:"rd"`             // Route Distinguisher (optional)
	Enforce     types.Bool   `tfsdk:"enforce_unique"` // enforce unique IP space
}

// vrfAPIModel is the JSON shape returned/accepted by the MCP server.
// Field names mirror the MOJO coordinator model.
type vrfAPIModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	RD            string `json:"rd,omitempty"`
	EnforceUnique bool   `json:"enforce_unique"`
}

// -------------------------------------------------------------------
// resource.Resource interface
// -------------------------------------------------------------------

func (r *VRFResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vrf"
}

func (r *VRFResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a MOJO VRF (Virtual Routing and Forwarding) instance. VRFs provide L3 isolation for IP space within MOJO IPAM.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "MOJO-assigned UUID for this VRF.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unique name for this VRF (e.g. `production`, `staging`).",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Human-readable description.",
			},
			"rd": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "BGP Route Distinguisher in `ASN:NN` format (e.g. `65000:100`). Optional.",
			},
			"enforce_unique": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "When `true`, MOJO prevents duplicate IP assignments within this VRF. Defaults to `true`.",
			},
		},
	}
}

// Configure extracts the MCP client from the provider configuration.
func (r *VRFResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*mcp.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected resource provider data",
			fmt.Sprintf("Expected *mcp.Client, got %T. Please report this issue.", req.ProviderData),
		)
		return
	}

	r.client = client
}

// -------------------------------------------------------------------
// CRUD operations
// -------------------------------------------------------------------

// Create calls ipam.create_vrf and stores the returned object in state.
func (r *VRFResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VRFResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"name":           plan.Name.ValueString(),
		"description":    plan.Description.ValueString(),
		"rd":             plan.RD.ValueString(),
		"enforce_unique": plan.Enforce.ValueBool(),
	}
	// Don't send empty optional strings — let the server apply defaults.
	if args["description"] == "" {
		delete(args, "description")
	}
	if args["rd"] == "" {
		delete(args, "rd")
	}
	if plan.Enforce.IsNull() || plan.Enforce.IsUnknown() {
		delete(args, "enforce_unique")
	}

	var apiObj vrfAPIModel
	if err := r.client.CallToolJSON(ctx, "ipam.create_vrf", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to create VRF", err.Error())
		return
	}

	apiToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read calls ipam.read_vrf by ID.  If the VRF no longer exists, it is removed
// from state so Terraform can re-create it on the next apply.
func (r *VRFResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VRFResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiObj vrfAPIModel
	err := r.client.CallToolJSON(ctx, "ipam.read_vrf", map[string]any{
		"id": state.ID.ValueString(),
	}, &apiObj)
	if err != nil {
		if mcp.IsNotFound(err) {
			resp.State.RemoveResource(ctx) // deleted outside Terraform
			return
		}
		resp.Diagnostics.AddError("Failed to read VRF", err.Error())
		return
	}

	apiToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update calls ipam.update_vrf with only the changed attributes.
// The MCP server handles partial updates via an update_fields list.
func (r *VRFResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state VRFResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"id":             state.ID.ValueString(),
		"name":           plan.Name.ValueString(),
		"description":    plan.Description.ValueString(),
		"rd":             plan.RD.ValueString(),
		"enforce_unique": plan.Enforce.ValueBool(),
	}

	var apiObj vrfAPIModel
	if err := r.client.CallToolJSON(ctx, "ipam.update_vrf", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to update VRF", err.Error())
		return
	}

	plan.ID = state.ID // preserve ID
	apiToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete calls ipam.delete_vrf.
func (r *VRFResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VRFResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CallTool(ctx, "ipam.delete_vrf", map[string]any{
		"id": state.ID.ValueString(),
	})
	if err != nil {
		if mcp.IsNotFound(err) {
			return // already gone — success
		}
		resp.Diagnostics.AddError("Failed to delete VRF", err.Error())
		return
	}
}

// ImportState supports `terraform import mojo_vrf.example <uuid>`.
func (r *VRFResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Seed state with just the ID, then Read will fill in the rest.
	var state VRFResourceModel
	state.ID = types.StringValue(req.ID)

	var apiObj vrfAPIModel
	err := r.client.CallToolJSON(ctx, "ipam.read_vrf", map[string]any{
		"id": req.ID,
	}, &apiObj)
	if err != nil {
		resp.Diagnostics.AddError("Failed to import VRF", err.Error())
		return
	}

	apiToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

// apiToState maps an API response object back into the Terraform state model.
func apiToState(api *vrfAPIModel, state *VRFResourceModel) {
	state.ID = types.StringValue(api.ID)
	state.Name = types.StringValue(api.Name)
	state.Description = types.StringValue(api.Description)
	state.RD = types.StringValue(api.RD)
	state.Enforce = types.BoolValue(api.EnforceUnique)
}
