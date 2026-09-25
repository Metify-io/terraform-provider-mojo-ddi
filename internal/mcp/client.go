// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package mcp provides a lightweight MCP client for the MOJO Terraform provider.
// It speaks the MCP protocol over HTTP+SSE (JSON-RPC 2.0) and maps MCP errors
// to Terraform diagnostics.
//
// We intentionally keep this self-contained rather than depending on a third-party
// MCP client library — the protocol is simple enough, and we avoid coupling to a
// still-maturing ecosystem.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// -------------------------------------------------------------------
// JSON-RPC 2.0 wire types
// -------------------------------------------------------------------

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("mcp rpc error %d: %s", e.Code, e.Message)
}

// -------------------------------------------------------------------
// MCP call/result types
// -------------------------------------------------------------------

// ToolCallParams is the params block for tools/call.
type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ToolResult is the parsed result from a tools/call response.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a single block inside a ToolResult.
type ContentBlock struct {
	Type string `json:"type"` // "text" | "json" | "error"
	Text string `json:"text,omitempty"`
}

// -------------------------------------------------------------------
// Client
// -------------------------------------------------------------------

// Client holds a persistent connection to a MOJO MCP server.
type Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client

	// SSE session management
	mu        sync.Mutex
	sessionID string // returned by the server on initialize

	// Auto-incrementing request IDs
	idGen atomic.Int64
}

// ClientConfig holds provider-level configuration for the MCP client.
type ClientConfig struct {
	Endpoint string
	APIKey   string
	// TLSClientCert / TLSClientKey for mTLS (air-gap) — future
}

// New creates a new MCP client and runs the initialize handshake.
func New(ctx context.Context, cfg ClientConfig) (*Client, error) {
	c := &Client{
		endpoint: strings.TrimRight(cfg.Endpoint, "/"),
		apiKey:   cfg.APIKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	if err := c.initialize(ctx); err != nil {
		return nil, fmt.Errorf("mcp initialize: %w", err)
	}

	return c, nil
}

// initialize sends the MCP initialize request and captures the session ID.
func (c *Client) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"clientInfo": map[string]any{
			"name":    "terraform-provider-mojo-ddi",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{},
	}

	result, err := c.call(ctx, "initialize", params)
	if err != nil {
		return err
	}

	var initResult struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(result, &initResult); err == nil && initResult.SessionID != "" {
		c.mu.Lock()
		c.sessionID = initResult.SessionID
		c.mu.Unlock()
	}

	// Send initialized notification (fire-and-forget, no response expected)
	_ = c.notify(ctx, "notifications/initialized", nil)

	tflog.Debug(ctx, "mcp client initialized", map[string]any{
		"endpoint":   c.endpoint,
		"session_id": initResult.SessionID,
	})

	return nil
}

// -------------------------------------------------------------------
// Public API
// -------------------------------------------------------------------

// sensitiveArgKeys are never logged: an approval token is a single-use
// actuation grant and a debug trace would leak an unconsumed one.
var sensitiveArgKeys = map[string]bool{
	"approval":       true,
	"approval_token": true,
	"api_key":        true,
	"password":       true,
	"bmc_password":   true,
	"token":          true,
}

func redactArgs(args map[string]any) map[string]any {
	redacted := make(map[string]any, len(args))
	for k, v := range args {
		if sensitiveArgKeys[strings.ToLower(k)] {
			redacted[k] = "[REDACTED]"
			continue
		}
		redacted[k] = v
	}
	return redacted
}

// CallTool invokes a named MCP tool and returns the parsed ToolResult.
func (c *Client) CallTool(ctx context.Context, tool string, args map[string]any) (*ToolResult, error) {
	tflog.Debug(ctx, "mcp tool call", map[string]any{"tool": tool, "args": redactArgs(args)})

	result, err := c.call(ctx, "tools/call", ToolCallParams{
		Name:      tool,
		Arguments: args,
	})
	if err != nil {
		return nil, err
	}

	var toolResult ToolResult
	if err := json.Unmarshal(result, &toolResult); err != nil {
		return nil, fmt.Errorf("parse tool result: %w", err)
	}

	if toolResult.IsError {
		errText := "unknown tool error"
		if len(toolResult.Content) > 0 {
			errText = toolResult.Content[0].Text
		}
		return nil, &MCPError{Code: "tool_error", Message: errText}
	}

	tflog.Debug(ctx, "mcp tool call succeeded", map[string]any{"tool": tool})
	return &toolResult, nil
}

// CallToolJSON calls a tool and unmarshals the first text content block as JSON
// into dest. This is the standard pattern for CRUD operations.
func (c *Client) CallToolJSON(ctx context.Context, tool string, args map[string]any, dest any) error {
	text, err := c.CallToolText(ctx, tool, args)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(text), dest); err != nil {
		return fmt.Errorf("unmarshal tool %q response: %w", tool, err)
	}

	return nil
}

// CallToolText calls a tool and returns the first text content block verbatim.
// Several mojo-mcp tools return either a JSON document or a plain-text error
// message in the same channel, so callers that need both should use this and
// decide per payload (see ParseToolText).
func (c *Client) CallToolText(ctx context.Context, tool string, args map[string]any) (string, error) {
	result, err := c.CallTool(ctx, tool, args)
	if err != nil {
		return "", err
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("tool %q returned empty content", tool)
	}

	return result.Content[0].Text, nil
}

// ParseToolText unmarshals a text content block into dest. mojo-mcp handlers
// signal failures two ways: a JSON-RPC error (already surfaced by CallTool) or
// a plain-text message inside a successful result ("Invalid UUID ...",
// "Baseline ... not found"). When the payload is not JSON it is returned as a
// tool-level error so the whole message reaches the Terraform diagnostic.
func ParseToolText(tool, text string, dest any) error {
	if err := json.Unmarshal([]byte(text), dest); err != nil {
		return &MCPError{Code: "tool_message", Message: strings.TrimSpace(text)}
	}
	return nil
}

// -------------------------------------------------------------------
// Transport
// -------------------------------------------------------------------

// call makes a JSON-RPC call to the MCP endpoint and returns the raw result.
func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.idGen.Add(1)

	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal rpc request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	c.mu.Lock()
	if c.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	c.mu.Unlock()

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(b))
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "text/event-stream") {
		return c.readSSEResponse(resp.Body, id)
	}

	// Plain JSON response
	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("decode rpc response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, mapRPCError(rpcResp.Error)
	}

	return rpcResp.Result, nil
}

// notify sends a JSON-RPC notification (no response expected).
func (c *Client) notify(ctx context.Context, method string, params any) error {
	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}

	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build notify request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	c.mu.Lock()
	if c.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	c.mu.Unlock()

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// readSSEResponse reads a Server-Sent Events stream and returns the result
// for the matching request ID.
func (c *Client) readSSEResponse(body io.Reader, id int64) (json.RawMessage, error) {
	scanner := bufio.NewScanner(body)
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// End of SSE event — try to parse what we have
			if len(dataLines) > 0 {
				data := strings.Join(dataLines, "\n")
				dataLines = nil

				var rpcResp rpcResponse
				if err := json.Unmarshal([]byte(data), &rpcResp); err != nil {
					continue // not a JSON-RPC response, skip
				}

				if rpcResp.ID != id {
					continue // different request, skip
				}

				if rpcResp.Error != nil {
					return nil, mapRPCError(rpcResp.Error)
				}

				return rpcResp.Result, nil
			}
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
		// Ignore "event:", "id:", "retry:" lines for now
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("sse read: %w", err)
	}

	return nil, fmt.Errorf("sse stream ended without a matching response for id %d", id)
}

// -------------------------------------------------------------------
// Error types
// -------------------------------------------------------------------

// MCPError represents a structured error returned by the MCP server.
type MCPError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *MCPError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// IsNotFound returns true if the error indicates the resource was not found.
func IsNotFound(err error) bool {
	var mcpErr *MCPError
	if ok := asMCPError(err, &mcpErr); ok {
		return mcpErr.Code == "resource_not_found" || mcpErr.Code == "not_found"
	}
	return false
}

func asMCPError(err error, target **MCPError) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*MCPError); ok {
		*target = e
		return true
	}
	return false
}

// mapRPCError converts a JSON-RPC error to a typed MCPError.
func mapRPCError(e *rpcError) error {
	// Try to parse a MOJO-specific error payload from Data. The actuation
	// boundary carries structured refusals here — e.g. the quota gate sends
	// {"error": "quota_exceeded", "dimension": ..., "pool": ...}.
	var mojoErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if e.Data != nil {
		_ = json.Unmarshal(e.Data, &mojoErr)
	}

	code := mojoErr.Code
	if code == "" {
		code = mojoErr.Error
	}
	if code != "" {
		msg := mojoErr.Message
		if msg == "" {
			msg = e.Message
		}
		return &MCPError{Code: code, Message: msg}
	}

	// Fall back to generic mapping by RPC error code
	code = "unknown"
	switch e.Code {
	case -32001:
		code = "resource_not_found"
	case -32002:
		code = "conflict"
	case -32003:
		code = "validation_error"
	case -32004:
		code = "permission_denied"
	case -32005:
		code = "confirmation_required"
	case -32029:
		code = "rate_limited"
	}

	return &MCPError{Code: code, Message: e.Message}
}
