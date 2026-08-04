// Package mcp provides a minimal client for the Model Context Protocol (MCP)
// over the Streamable HTTP transport. It speaks JSON-RPC 2.0 to an MCP server
// so agents can discover (tools/list) and invoke (tools/call) the tools that
// server exposes. Only net/http and encoding/json are used — no external SDK.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// protocolVersion is the MCP revision this client negotiates during initialize.
const protocolVersion = "2025-06-18"

// Tool describes a single tool advertised by an MCP server via tools/list.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Client is a JSON-RPC 2.0 client for one MCP server's Streamable HTTP endpoint.
type Client struct {
	url        string
	authHeader string
	httpClient *http.Client
	nextID     atomic.Int64
}

// NewClient creates a client for the MCP server at url. If authHeader is
// non-empty it is sent verbatim as the Authorization header on every request
// (e.g. "Bearer <token>").
func NewClient(url, authHeader string) *Client {
	return &Client{
		url:        url,
		authHeader: authHeader,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// rpcRequest is a JSON-RPC 2.0 request envelope.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// rpcError is a JSON-RPC 2.0 error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("mcp rpc error %d: %s", e.Code, e.Message)
}

// rpcResponse is a JSON-RPC 2.0 response envelope with a raw result.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

// call performs a single JSON-RPC request/response round trip and unmarshals the
// result into out (when out is non-nil). The Streamable HTTP transport may reply
// with either application/json or a text/event-stream SSE frame; both are handled.
func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	reqBody, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      c.nextID.Add(1),
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return fmt.Errorf("mcp: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("mcp: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if c.authHeader != "" {
		httpReq.Header.Set("Authorization", c.authHeader)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mcp: %s: %w", method, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("mcp: %s: read response: %w", method, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mcp: %s: unexpected status %d: %s", method, resp.StatusCode, string(raw))
	}

	payload := extractJSONPayload(raw)

	var rpcResp rpcResponse
	if err := json.Unmarshal(payload, &rpcResp); err != nil {
		return fmt.Errorf("mcp: %s: decode response: %w", method, err)
	}
	if rpcResp.Error != nil {
		return rpcResp.Error
	}
	if out != nil && len(rpcResp.Result) > 0 {
		if err := json.Unmarshal(rpcResp.Result, out); err != nil {
			return fmt.Errorf("mcp: %s: decode result: %w", method, err)
		}
	}
	return nil
}

// extractJSONPayload returns the JSON-RPC object from a response body that is
// either a bare JSON object or an SSE stream whose data: lines carry the JSON.
func extractJSONPayload(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return trimmed
	}
	// Server-Sent Events: pull the last non-empty data: line's payload.
	var data []byte
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		if bytes.HasPrefix(line, []byte("data:")) {
			data = bytes.TrimSpace(line[len("data:"):])
		}
	}
	if len(data) > 0 {
		return data
	}
	return trimmed
}

// Initialize performs the MCP initialize handshake. It must be called before
// listing or calling tools.
func (c *Client) Initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "jobshout",
			"version": "1.0.0",
		},
	}
	return c.call(ctx, "initialize", params, nil)
}

// ListTools returns the tools advertised by the server via tools/list.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := c.call(ctx, "tools/list", map[string]any{}, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// contentBlock is one entry of a tools/call result's content array.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// CallTool invokes the named tool with args and returns the concatenated text
// content of the result.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	if args == nil {
		args = map[string]any{}
	}
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}
	var result struct {
		Content []contentBlock `json:"content"`
		IsError bool           `json:"isError"`
	}
	if err := c.call(ctx, "tools/call", params, &result); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	for _, block := range result.Content {
		if block.Type == "text" {
			buf.WriteString(block.Text)
		}
	}
	text := buf.String()
	if result.IsError {
		return text, fmt.Errorf("mcp: tool %q returned an error: %s", name, text)
	}
	return text, nil
}
