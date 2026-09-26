// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package actuation implements Terraform resources that drive the mojo-mcp
// governed actuation surface (ADR-0031): provisioning, firmware baselines,
// and the approval-token flow for destructive tools.
package actuation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Metify-io/terraform-provider-mojo-ddi/internal/mcp"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// searchNodesResult mirrors the `search_nodes` tool response.
type searchNodesResult struct {
	Results []struct {
		ID           string `json:"id"`
		Hostname     string `json:"hostname"`
		Model        string `json:"model"`
		SerialNumber string `json:"serial_number"`
		BMCAddress   string `json:"bmc_address"`
		HealthStatus string `json:"health_status"`
	} `json:"results"`
	Total int `json:"total"`
}

// listProfilesResult mirrors the `list_profiles` tool response.
type listProfilesResult struct {
	Profiles []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		OSFamily string `json:"os_family"`
		Version  string `json:"version"`
		Arch     string `json:"architecture"`
		Template string `json:"kickstart_template"`
	} `json:"profiles"`
	Total  int    `json:"total"`
	Status string `json:"status"` // "delegated" when the Django engine is in use
}

// provisionResponse mirrors the `provision_server` tool response on both the
// Rust PXE path (status "queued", request_id set) and the coordinator
// delegation path (status "delegated", no request_id), plus the boundary's
// dry-run envelope (outcome "planned").
type provisionResponse struct {
	Status     string `json:"status"`
	RequestID  string `json:"request_id"`
	NodeID     string `json:"node_id"`
	ServerID   string `json:"server_id"`
	ProfileID  string `json:"profile_id"`
	BaselineID string `json:"baseline_id"`
	Progress   int64  `json:"progress_pct"`
	Engine     string `json:"engine"`
	Message    string `json:"message"`
	Outcome    string `json:"outcome"`
	DryRun     bool   `json:"dry_run"`
	Summary    string `json:"summary"`
}

// provisionStatusResponse mirrors `get_provision_status`.
type provisionStatusResponse struct {
	RequestID string `json:"request_id"`
	NodeID    string `json:"node_id"`
	ProfileID string `json:"profile_id"`
	Status    string `json:"status"`
	Progress  int64  `json:"progress_pct"`
	Error     string `json:"error"`
	UpdatedAt string `json:"updated_at"`
}

// baselineStatusResponse mirrors `get_baseline_status`.
type baselineStatusResponse struct {
	ServerID    string `json:"server_id"`
	Status      string `json:"status"` // "no_results" when nothing evaluated yet
	Evaluations []struct {
		BaselineID    string `json:"baseline_id"`
		OverallStatus string `json:"overall_status"`
		RuleCount     int64  `json:"rule_count"`
		EvaluatedAt   string `json:"evaluated_at"`
	} `json:"evaluations"`
	Total int `json:"total"`
}

// baselineApplyResponse mirrors `apply_baseline` (delegated-path JSON) and the
// dry-run envelope.
type baselineApplyResponse struct {
	Status     string `json:"status"`
	ServerID   string `json:"server_id"`
	BaselineID string `json:"baseline_id"`
	Message    string `json:"message"`
	Outcome    string `json:"outcome"`
	DryRun     bool   `json:"dry_run"`
}

// ResolveServerID turns the configured node identifier into a node UUID.
// Pass the *configured* server_id (req.Config), not the planned value: a
// computed server_id carried from prior state must not survive a
// serial_number change — the serial always resolves through `search_nodes`
// (exact match to disambiguate), and a configured server_id that disagrees
// with the resolved serial is an error rather than a silent retarget.
func ResolveServerID(ctx context.Context, client *mcp.Client, serverID, serial string) (string, error) {
	if serial == "" {
		if serverID == "" {
			return "", fmt.Errorf("one of server_id or serial_number is required")
		}
		return serverID, nil
	}

	text, err := client.CallToolText(ctx, "search_nodes", map[string]any{
		"query": serial,
		"limit": 50,
	})
	if err != nil {
		return "", err
	}

	var res searchNodesResult
	if err := mcp.ParseToolText("search_nodes", text, &res); err != nil {
		return "", err
	}

	var matches []string
	for _, r := range res.Results {
		if strings.EqualFold(r.SerialNumber, serial) {
			matches = append(matches, r.ID)
		}
	}
	var resolved string
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no MOJO node with serial_number %q", serial)
	case 1:
		resolved = matches[0]
	default:
		return "", fmt.Errorf("serial_number %q matches %d nodes; use server_id to disambiguate", serial, len(matches))
	}
	if serverID != "" && !strings.EqualFold(resolved, serverID) {
		return "", fmt.Errorf(
			"serial_number %q resolves to node %s, which does not match server_id %s; refusing to actuate a different node",
			serial, resolved, serverID)
	}
	return resolved, nil
}

// resolveProfileID turns profile_name (+ optional os_family filter) into a
// profile UUID via `list_profiles`. An explicit profile_id wins. When the
// server delegates profile storage to the coordinator there is no name
// catalog to search — the operator must supply profile_id.
func resolveProfileID(ctx context.Context, client *mcp.Client, profileID, profileName, osFamily string) (string, error) {
	if profileID != "" {
		return profileID, nil
	}
	if profileName == "" {
		return "", nil
	}

	args := map[string]any{}
	if osFamily != "" {
		args["os_family"] = osFamily
	}
	text, err := client.CallToolText(ctx, "list_profiles", args)
	if err != nil {
		return "", err
	}

	var res listProfilesResult
	if err := mcp.ParseToolText("list_profiles", text, &res); err != nil {
		return "", err
	}
	if res.Status == "delegated" {
		return "", fmt.Errorf("profile_name lookup requires the Rust PXE engine (list_profiles is delegated); supply profile_id instead")
	}

	var matches []string
	for _, p := range res.Profiles {
		if strings.EqualFold(p.Name, profileName) {
			matches = append(matches, p.ID)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no OS profile named %q", profileName)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("profile_name %q matches %d profiles; use profile_id to disambiguate", profileName, len(matches))
	}
}

// provisionTerminal reports whether a `get_provision_status` status is final.
// Vocabulary is mojo-pxe ProvisionStatus (snake_case): queued,
// dhcp_configured, pxe_booting, installing, post_install, complete, failed,
// and the crate's British-spelled terminal state.
func provisionTerminal(status string) bool {
	switch strings.ToLower(status) {
	case "complete", "completed", "failed", "cancelled", "canceled", "error": //nolint:misspell
		return true
	}
	return false
}

func provisionFailed(status string) bool {
	switch strings.ToLower(status) {
	case "failed", "cancelled", "canceled", "error": //nolint:misspell
		return true
	}
	return false
}

// waitForProvision polls `get_provision_status` until the request reaches a
// terminal status or the timeout elapses. Returns the last observed status.
func waitForProvision(ctx context.Context, client *mcp.Client, requestID string, timeout, interval time.Duration) (*provisionStatusResponse, error) {
	deadline := time.Now().Add(timeout)
	var last provisionStatusResponse

	for {
		text, err := client.CallToolText(ctx, "get_provision_status", map[string]any{
			"request_id": requestID,
		})
		if err != nil {
			return nil, err
		}
		if err := mcp.ParseToolText("get_provision_status", text, &last); err != nil {
			return nil, err
		}

		tflog.Debug(ctx, "provision poll", map[string]any{
			"request_id": requestID,
			"status":     last.Status,
			"progress":   last.Progress,
		})

		if provisionTerminal(last.Status) {
			return &last, nil
		}
		if time.Now().After(deadline) {
			return &last, fmt.Errorf("timed out after %s waiting for provisioning to complete (last status %q)", timeout, last.Status)
		}

		select {
		case <-ctx.Done():
			return &last, ctx.Err()
		case <-time.After(interval):
		}
	}
}
