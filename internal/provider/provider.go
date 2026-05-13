// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"os"

	ipamdatasources "github.com/Metify-io/terraform-provider-mojo-ddi/internal/datasources/ipam"
	imcp "github.com/Metify-io/terraform-provider-mojo-ddi/internal/mcp"
	ipamresources "github.com/Metify-io/terraform-provider-mojo-ddi/internal/resources/ipam"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure MojoProvider satisfies the provider.Provider interface.
var _ provider.Provider = &MojoProvider{}
var _ provider.ProviderWithFunctions = &MojoProvider{}

// MojoProvider is the top-level provider struct.
type MojoProvider struct {
	version string
}

// MojoProviderModel maps the provider HCL configuration block.
type MojoProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	APIKey   types.String `tfsdk:"api_key"`
}

// New constructs a provider factory for the given version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &MojoProvider{version: version}
	}
}

// Metadata returns provider type name and version.
func (p *MojoProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "mojo"
	resp.Version = p.version
}

// Schema defines the provider-level HCL configuration.
//
//	provider "mojo" {
//	  endpoint = "https://mojo.local:8443/mcp"
//	  api_key  = var.mojo_api_key
//	}
//
// Both values may also be supplied via environment variables:
//
//	MOJO_ENDPOINT  — MCP server URL
//	MOJO_API_KEY   — API key
func (p *MojoProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The **mojo-ddi** provider manages MOJO DDI resources (VRFs, VLANs, prefixes, IP addresses, DHCP scopes and reservations, DNS zones and records) via the MOJO MCP server.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "URL of the MOJO MCP server (e.g. `https://mojo.local:8443/mcp`). May also be set via the `MOJO_ENDPOINT` environment variable.",
				Optional:            true,
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "API key for authenticating with the MCP server. May also be set via the `MOJO_API_KEY` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

// Configure initialises the MCP client and stores it in resp.ResourceData
// so resources and data sources can access it.
func (p *MojoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config MojoProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve endpoint: HCL > env var
	endpoint := config.Endpoint.ValueString()
	if endpoint == "" {
		endpoint = os.Getenv("MOJO_ENDPOINT")
	}
	if endpoint == "" {
		resp.Diagnostics.AddError(
			"Missing MOJO endpoint",
			"Set the `endpoint` provider attribute or the `MOJO_ENDPOINT` environment variable.",
		)
		return
	}

	// Resolve API key: HCL > env var
	apiKey := config.APIKey.ValueString()
	if apiKey == "" {
		apiKey = os.Getenv("MOJO_API_KEY")
	}

	client, err := imcp.New(ctx, imcp.ClientConfig{
		Endpoint: endpoint,
		APIKey:   apiKey,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to connect to MOJO MCP server",
			"Could not initialise the MCP client: "+err.Error(),
		)
		return
	}

	// Pass the client to all resources and data sources via ResourceData/DataSourceData.
	resp.ResourceData = client
	resp.DataSourceData = client
}

// Resources returns all managed resource implementations.
func (p *MojoProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		ipamresources.NewVRFResource,
		// Phase 1 week 1 — add as implemented:
		// ipamresources.NewVLANResource,
		// ipamresources.NewPrefixResource,
		// ipamresources.NewIPAddressResource,
		// ipamresources.NewIPRangeResource,
		// ddiresources.NewDHCPScopeResource,
		// ddiresources.NewDHCPReservationResource,
		// ddiresources.NewDNSZoneResource,
		// ddiresources.NewDNSRecordResource,
	}
}

// DataSources returns all data source implementations.
func (p *MojoProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		ipamdatasources.NewNextAvailableIPDataSource,
		// ipamdatasources.NewPrefixDataSource,
	}
}

// Functions returns provider-defined functions (none yet).
func (p *MojoProvider) Functions(_ context.Context) []func() function.Function {
	return []func() function.Function{}
}
