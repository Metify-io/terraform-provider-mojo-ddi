// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package actuationdatasources implements data sources over the mojo-mcp
// governed actuation surface.
package actuationdatasources

import (
	"context"
	"fmt"

	"github.com/Metify-io/terraform-provider-mojo-ddi/internal/mcp"
	"github.com/Metify-io/terraform-provider-mojo-ddi/internal/resources/actuation"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &BaselineEvaluationDataSource{}
var _ datasource.DataSourceWithValidateConfig = &BaselineEvaluationDataSource{}

// NewBaselineEvaluationDataSource is the factory function registered in the provider.
func NewBaselineEvaluationDataSource() datasource.DataSource {
	return &BaselineEvaluationDataSource{}
}

// BaselineEvaluationDataSource implements the mojo_baseline_evaluation data
// source: a live `evaluate_baseline` run against one node.
type BaselineEvaluationDataSource struct {
	client *mcp.Client
}

// BaselineEvaluationModel maps the Terraform schema to Go types.
type BaselineEvaluationModel struct {
	ID               types.String `tfsdk:"id"`
	SerialNumber     types.String `tfsdk:"serial_number"`
	ServerID         types.String `tfsdk:"server_id"`
	BaselineID       types.String `tfsdk:"baseline_id"`
	OverallStatus    types.String `tfsdk:"overall_status"`
	RuleResultsCount types.Int64  `tfsdk:"rule_results_count"`
	EvaluatedAt      types.String `tfsdk:"evaluated_at"`
	Unreachable      types.Bool   `tfsdk:"unreachable"`
	UnreachableError types.String `tfsdk:"unreachable_error"`
}

func (d *BaselineEvaluationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_baseline_evaluation"
}

func (d *BaselineEvaluationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Runs a live ` + "`evaluate_baseline`" + ` against one node: reads the node's BIOS attributes and firmware inventory over Redfish, evaluates them against the baseline, and stores the result.

Because it reads hardware, every ` + "`terraform plan`" + `/refresh that evaluates this data source contacts the node's BMC. Use it where drift visibility at plan time matters; for a DB-only view use ` + "`mojo_firmware_baseline`" + `'s computed ` + "`compliance_status`" + ` or the ` + "`get_baseline_status`" + ` tool.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`evaluation:<server_id>:<baseline_id>`.",
			},
			"serial_number": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Chassis serial number of the node (resolved via `search_nodes`). Exactly one of `serial_number`/`server_id` must be set.",
			},
			"server_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "MOJO node UUID. Set explicitly or resolved from `serial_number`.",
			},
			"baseline_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Baseline UUID to evaluate against.",
			},
			"overall_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Evaluation outcome (`compliant`, `non_compliant`).",
			},
			"rule_results_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of rules evaluated.",
			},
			"evaluated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC 3339 timestamp of the evaluation.",
			},
			"unreachable": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "True when the node's BMC could not be reached — no evaluation was produced.",
			},
			"unreachable_error": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "BMC reachability error detail when `unreachable` is true.",
			},
		},
	}
}

func (d *BaselineEvaluationDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config BaselineEvaluationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.SerialNumber.IsNull() && config.ServerID.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("serial_number"),
			"Missing node identifier",
			"Set serial_number or server_id to identify the node to evaluate.",
		)
	}
}

func (d *BaselineEvaluationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *BaselineEvaluationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config BaselineEvaluationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serverID, err := actuation.ResolveServerID(ctx, d.client, config.ServerID.ValueString(), config.SerialNumber.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve node", err.Error())
		return
	}
	config.ServerID = types.StringValue(serverID)

	text, err := d.client.CallToolText(ctx, "evaluate_baseline", map[string]any{
		"node_ids":    []string{serverID},
		"baseline_id": config.BaselineID.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("evaluate_baseline failed", err.Error())
		return
	}

	var eval struct {
		Results []struct {
			NodeID           string `json:"node_id"`
			BaselineID       string `json:"baseline_id"`
			OverallStatus    string `json:"overall_status"`
			RuleResultsCount int64  `json:"rule_results_count"`
			EvaluatedAt      string `json:"evaluated_at"`
		} `json:"results"`
		Total       int `json:"total"`
		Unreachable []struct {
			NodeID string `json:"node_id"`
			Error  string `json:"error"`
		} `json:"unreachable"`
	}
	if err := mcp.ParseToolText("evaluate_baseline", text, &eval); err != nil {
		resp.Diagnostics.AddError("evaluate_baseline failed", err.Error())
		return
	}

	config.ID = types.StringValue(fmt.Sprintf("evaluation:%s:%s", serverID, config.BaselineID.ValueString()))

	for _, u := range eval.Unreachable {
		if u.NodeID == serverID {
			config.Unreachable = types.BoolValue(true)
			config.UnreachableError = types.StringValue(u.Error)
			config.OverallStatus = types.StringNull()
			config.RuleResultsCount = types.Int64Null()
			config.EvaluatedAt = types.StringNull()
			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	for _, r := range eval.Results {
		if r.NodeID == serverID {
			config.Unreachable = types.BoolValue(false)
			config.UnreachableError = types.StringNull()
			config.OverallStatus = types.StringValue(r.OverallStatus)
			config.RuleResultsCount = types.Int64Value(r.RuleResultsCount)
			config.EvaluatedAt = types.StringValue(r.EvaluatedAt)
			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Node missing from evaluation",
		fmt.Sprintf("evaluate_baseline returned neither a result nor an unreachable entry for node %s.", serverID),
	)
}
