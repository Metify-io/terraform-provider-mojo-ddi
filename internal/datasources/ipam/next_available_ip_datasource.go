// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package ipamdatasources

import (
	"context"
	"fmt"

	"github.com/Metify-io/terraform-provider-mojo-ddi/internal/mcp"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &NextAvailableIPDataSource{}

// NewNextAvailableIPDataSource is the factory function registered in the provider.
func NewNextAvailableIPDataSource() datasource.DataSource {
	return &NextAvailableIPDataSource{}
}

// NextAvailableIPDataSource implements the mojo_next_available_ip data source.
type NextAvailableIPDataSource struct {
	client *mcp.Client
}

// NextAvailableIPModel maps the Terraform schema to Go types.
type NextAvailableIPModel struct {
	PrefixID types.String `tfsdk:"prefix_id"`
	Address  types.String `tfsdk:"address"`
}

// nextAvailableIPAPIModel is the JSON shape returned by the MCP server.
type nextAvailableIPAPIModel struct {
	Address string `json:"address"`
}

func (d *NextAvailableIPDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_next_available_ip"
}

func (d *NextAvailableIPDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Returns the next unallocated IP address within a MOJO prefix without reserving it. Use this in combination with `mojo_ip_address` to allocate the returned address.",
		Attributes: map[string]schema.Attribute{
			"prefix_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the prefix to query for the next available IP.",
			},
			"address": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The next available IP address (e.g. `10.0.0.5`).",
			},
		},
	}
}

func (d *NextAvailableIPDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*mcp.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected data source provider data",
			fmt.Sprintf("Expected *mcp.Client, got %T. Please report this issue.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *NextAvailableIPDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NextAvailableIPModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiObj nextAvailableIPAPIModel
	err := d.client.CallToolJSON(ctx, "ipam.next_available_ip", map[string]any{
		"prefix_id": config.PrefixID.ValueString(),
	}, &apiObj)
	if err != nil {
		resp.Diagnostics.AddError("Failed to query next available IP", err.Error())
		return
	}

	config.Address = types.StringValue(apiObj.Address)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
