// Package wslproxymcp is JobShout's typed client for the wslproxy MCP server.
//
// Prefer this over the raw REST client in waflab when the operation is already
// exposed as an MCP tool (traffic split, promote/rollback, bind_waf_policy,
// create_server, …). The same MCP surface can also be used outside JobShout
// (Cursor, Claude Desktop) — see docs/wslproxy-mcp.md.
package wslproxymcp

import (
	"os"
	"strings"
	"time"
)

// Config is loaded from WSLPROXY_* / WSLPROXY_MCP_* environment variables.
type Config struct {
	Enabled      bool
	BaseURL      string // admin base, e.g. https://lon1.pop0.uk
	JSONRPCPath  string // default /mcp/jsonrpc
	APIKey       string
	APIKeyHeader string // default X-MCP-API-Key
	Timeout      time.Duration
}

// LoadConfig reads MCP settings. Falls back to WSLPROXY_BASE_URL when
// WSLPROXY_MCP_BASE_URL is unset so one admin URL serves both REST and MCP.
func LoadConfig() Config {
	enabled := true
	if v := strings.TrimSpace(os.Getenv("WSLPROXY_MCP_ENABLED")); v != "" {
		enabled = parseBool(v, true)
	} else if v := strings.TrimSpace(os.Getenv("WSLPROXY_ENABLED")); v != "" {
		enabled = parseBool(v, true)
	}

	base := strings.TrimRight(strings.TrimSpace(os.Getenv("WSLPROXY_MCP_BASE_URL")), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(os.Getenv("WSLPROXY_BASE_URL")), "/")
	}

	path := strings.TrimSpace(os.Getenv("WSLPROXY_MCP_JSONRPC_PATH"))
	if path == "" {
		path = "/mcp/jsonrpc"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	key := strings.TrimSpace(os.Getenv("WSLPROXY_MCP_API_KEY"))
	hdr := strings.TrimSpace(os.Getenv("WSLPROXY_MCP_API_KEY_HEADER"))
	if hdr == "" {
		hdr = "X-MCP-API-Key"
	}

	timeout := 30 * time.Second
	if v := strings.TrimSpace(os.Getenv("WSLPROXY_MCP_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	} else if v := strings.TrimSpace(os.Getenv("WSLPROXY_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	}

	return Config{
		Enabled:      enabled,
		BaseURL:      base,
		JSONRPCPath:  path,
		APIKey:       key,
		APIKeyHeader: hdr,
		Timeout:      timeout,
	}
}

func parseBool(v string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}
