package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/mcp"
	"github.com/jobshout/server/internal/model"
)

// MCPStore lists an org's configured MCP servers. Satisfied by
// repository.MCPRepository — declared here (not imported) so the tools package
// stays decoupled from the repository layer, mirroring IntegrationConfigStore.
type MCPStore interface {
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.MCPServer, error)
}

// mcpClient is the narrow slice of *mcp.Client the tools depend on, so tests can
// substitute a fake without a live HTTP server.
type mcpClient interface {
	Initialize(ctx context.Context) error
	ListTools(ctx context.Context) ([]mcp.Tool, error)
	CallTool(ctx context.Context, name string, args map[string]any) (string, error)
}

// mcpClientFactory builds a client for a given server endpoint. Satisfied by
// the default factory that wraps mcp.NewClient; faked in tests.
type mcpClientFactory interface {
	NewClient(url, authHeader string) mcpClient
}

type defaultClientFactory struct{}

func (defaultClientFactory) NewClient(url, authHeader string) mcpClient {
	return mcp.NewClient(url, authHeader)
}

// NewMCPTools returns the agent-callable tools that bridge to the calling org's
// configured MCP servers: one tool to list the tools every enabled server
// advertises, and one to invoke a specific tool on a named server. Each tool
// resolves the org from the execution context, so a single registered instance
// serves every tenant. Register the returned tools into the shared Registry.
func NewMCPTools(store MCPStore) []Tool {
	return newMCPToolsWithFactory(store, defaultClientFactory{})
}

// newMCPToolsWithFactory is the injectable constructor used by tests.
func newMCPToolsWithFactory(store MCPStore, factory mcpClientFactory) []Tool {
	return []Tool{
		&mcpListToolsTool{store: store, factory: factory},
		&mcpCallTool{store: store, factory: factory},
	}
}

var errMCPNoOrg = fmt.Errorf("no organization in context: MCP tools require an org-scoped execution")

// enabledServers resolves the calling org's enabled MCP servers.
func enabledServers(ctx context.Context, store MCPStore) ([]model.MCPServer, error) {
	orgID, ok := OrgFromContext(ctx)
	if !ok {
		return nil, errMCPNoOrg
	}
	servers, err := store.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list mcp servers: %w", err)
	}
	enabled := make([]model.MCPServer, 0, len(servers))
	for _, s := range servers {
		if s.Enabled {
			enabled = append(enabled, s)
		}
	}
	return enabled, nil
}

// --- mcp_list_tools ---

type mcpListToolsTool struct {
	store   MCPStore
	factory mcpClientFactory
}

func (t *mcpListToolsTool) Name() string { return "mcp_list_tools" }

func (t *mcpListToolsTool) Description() string {
	return `List the tools exposed by this organization's configured MCP (Model Context Protocol) servers.
Takes no input parameters.
Returns a JSON array of {server, name, description} — use "server" and "name" with mcp_call to invoke a tool.`
}

// Parameters advertises the (empty) JSON-Schema for this tool so it is
// compatible with providers that support native function-calling.
func (t *mcpListToolsTool) Parameters() ParameterSchema {
	return ObjectSchema(map[string]any{})
}

type mcpToolEntry struct {
	Server      string `json:"server"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (t *mcpListToolsTool) Execute(ctx context.Context, _ map[string]any) (string, error) {
	servers, err := enabledServers(ctx, t.store)
	if err != nil {
		return "", err
	}

	entries := []mcpToolEntry{}
	for _, s := range servers {
		client := t.factory.NewClient(s.URL, s.AuthHeader)
		if err := client.Initialize(ctx); err != nil {
			return "", fmt.Errorf("mcp_list_tools: initialize %q: %w", s.Name, err)
		}
		mcpTools, err := client.ListTools(ctx)
		if err != nil {
			return "", fmt.Errorf("mcp_list_tools: list %q: %w", s.Name, err)
		}
		for _, mt := range mcpTools {
			entries = append(entries, mcpToolEntry{
				Server:      s.Name,
				Name:        mt.Name,
				Description: mt.Description,
			})
		}
	}

	out, _ := json.Marshal(entries)
	return string(out), nil
}

// --- mcp_call ---

type mcpCallTool struct {
	store   MCPStore
	factory mcpClientFactory
}

func (t *mcpCallTool) Name() string { return "mcp_call" }

func (t *mcpCallTool) Description() string {
	return `Invoke a tool on one of this organization's configured MCP servers.
Input parameters:
  server    (string, required) - The MCP server name (see mcp_list_tools)
  tool      (string, required) - The tool name to invoke on that server
  arguments (object, optional) - Arguments passed to the tool
Returns the tool's text result.`
}

// Parameters advertises the JSON-Schema for this tool's inputs so it works with
// providers that support native function-calling.
func (t *mcpCallTool) Parameters() ParameterSchema {
	return ObjectSchema(map[string]any{
		"server":    map[string]any{"type": "string", "description": "The MCP server name (see mcp_list_tools)"},
		"tool":      map[string]any{"type": "string", "description": "The tool name to invoke on that server"},
		"arguments": map[string]any{"type": "object", "description": "Arguments passed to the tool"},
	}, "server", "tool")
}

func (t *mcpCallTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	servers, err := enabledServers(ctx, t.store)
	if err != nil {
		return "", err
	}
	serverName, err := stringParam(input, "server", true)
	if err != nil {
		return "", err
	}
	toolName, err := stringParam(input, "tool", true)
	if err != nil {
		return "", err
	}

	var args map[string]any
	if raw, ok := input["arguments"]; ok && raw != nil {
		m, ok := raw.(map[string]any)
		if !ok {
			return "", fmt.Errorf("mcp_call: parameter \"arguments\" must be an object")
		}
		args = m
	}

	var target *model.MCPServer
	for i := range servers {
		if servers[i].Name == serverName {
			target = &servers[i]
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("mcp_call: no enabled MCP server named %q is configured for this organization", serverName)
	}

	client := t.factory.NewClient(target.URL, target.AuthHeader)
	if err := client.Initialize(ctx); err != nil {
		return "", fmt.Errorf("mcp_call: initialize %q: %w", serverName, err)
	}
	result, err := client.CallTool(ctx, toolName, args)
	if err != nil {
		return "", fmt.Errorf("mcp_call: %w", err)
	}
	return result, nil
}
