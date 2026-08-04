package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/mcp"
	"github.com/jobshout/server/internal/model"
)

// --- fakes ---

type fakeMCPStore struct {
	servers []model.MCPServer
	err     error
}

func (f *fakeMCPStore) ListByOrg(context.Context, uuid.UUID) ([]model.MCPServer, error) {
	return f.servers, f.err
}

// fakeMCPClient records calls and returns canned results. Keyed by nothing —
// the factory hands out a per-URL client so tests can assert which URL was hit.
type fakeMCPClient struct {
	url         string
	tools       []mcp.Tool
	callResult  string
	initErr     error
	listErr     error
	callErr     error
	lastCall    string
	lastArgs    map[string]any
	initialized bool
}

func (c *fakeMCPClient) Initialize(context.Context) error {
	c.initialized = true
	return c.initErr
}
func (c *fakeMCPClient) ListTools(context.Context) ([]mcp.Tool, error) {
	return c.tools, c.listErr
}
func (c *fakeMCPClient) CallTool(_ context.Context, name string, args map[string]any) (string, error) {
	c.lastCall = name
	c.lastArgs = args
	return c.callResult, c.callErr
}

// fakeFactory maps a server URL to a preconfigured fake client.
type fakeFactory struct {
	byURL map[string]*fakeMCPClient
}

func (f *fakeFactory) NewClient(url, _ string) mcpClient {
	if c, ok := f.byURL[url]; ok {
		return c
	}
	return &fakeMCPClient{url: url}
}

func mcpOrgCtx() context.Context {
	return WithOrg(context.Background(), uuid.New())
}

// --- tests ---

func TestMCPListTools(t *testing.T) {
	serverA := model.MCPServer{Name: "alpha", URL: "http://a", Enabled: true}
	serverB := model.MCPServer{Name: "beta", URL: "http://b", Enabled: true}
	serverDisabled := model.MCPServer{Name: "gamma", URL: "http://g", Enabled: false}

	factory := &fakeFactory{byURL: map[string]*fakeMCPClient{
		"http://a": {tools: []mcp.Tool{{Name: "t1", Description: "d1"}}},
		"http://b": {tools: []mcp.Tool{{Name: "t2", Description: "d2"}}},
		"http://g": {tools: []mcp.Tool{{Name: "should-not-appear"}}},
	}}

	tests := []struct {
		name        string
		ctx         context.Context
		store       MCPStore
		wantErr     string
		wantSubstrs []string
		notSubstr   string
	}{
		{
			name:        "list across servers",
			ctx:         mcpOrgCtx(),
			store:       &fakeMCPStore{servers: []model.MCPServer{serverA, serverB, serverDisabled}},
			wantSubstrs: []string{`"server":"alpha"`, `"name":"t1"`, `"server":"beta"`, `"name":"t2"`},
			notSubstr:   "should-not-appear",
		},
		{
			name:    "no org in context",
			ctx:     context.Background(),
			store:   &fakeMCPStore{servers: []model.MCPServer{serverA}},
			wantErr: "no organization in context",
		},
		{
			name:        "no servers configured",
			ctx:         mcpOrgCtx(),
			store:       &fakeMCPStore{servers: nil},
			wantSubstrs: []string{"[]"},
		},
		{
			name:    "store error propagates",
			ctx:     mcpOrgCtx(),
			store:   &fakeMCPStore{err: errors.New("db down")},
			wantErr: "db down",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool := &mcpListToolsTool{store: tc.store, factory: factory}
			out, err := tool.Execute(tc.ctx, nil)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, s := range tc.wantSubstrs {
				if !strings.Contains(out, s) {
					t.Fatalf("want output containing %q, got %q", s, out)
				}
			}
			if tc.notSubstr != "" && strings.Contains(out, tc.notSubstr) {
				t.Fatalf("output should not contain %q, got %q", tc.notSubstr, out)
			}
		})
	}
}

func TestMCPCall(t *testing.T) {
	server := model.MCPServer{Name: "alpha", URL: "http://a", Enabled: true}
	disabled := model.MCPServer{Name: "off", URL: "http://off", Enabled: false}

	tests := []struct {
		name       string
		ctx        context.Context
		store      MCPStore
		input      map[string]any
		wantErr    string
		wantResult string
		wantArgs   map[string]any
	}{
		{
			name:       "success",
			ctx:        mcpOrgCtx(),
			store:      &fakeMCPStore{servers: []model.MCPServer{server}},
			input:      map[string]any{"server": "alpha", "tool": "greet", "arguments": map[string]any{"who": "world"}},
			wantResult: "hello world",
			wantArgs:   map[string]any{"who": "world"},
		},
		{
			name:    "no org in context",
			ctx:     context.Background(),
			store:   &fakeMCPStore{servers: []model.MCPServer{server}},
			input:   map[string]any{"server": "alpha", "tool": "greet"},
			wantErr: "no organization in context",
		},
		{
			name:    "unknown server",
			ctx:     mcpOrgCtx(),
			store:   &fakeMCPStore{servers: []model.MCPServer{server}},
			input:   map[string]any{"server": "ghost", "tool": "greet"},
			wantErr: "no enabled MCP server named",
		},
		{
			name:    "disabled server not found",
			ctx:     mcpOrgCtx(),
			store:   &fakeMCPStore{servers: []model.MCPServer{disabled}},
			input:   map[string]any{"server": "off", "tool": "greet"},
			wantErr: "no enabled MCP server named",
		},
		{
			name:    "missing required tool",
			ctx:     mcpOrgCtx(),
			store:   &fakeMCPStore{servers: []model.MCPServer{server}},
			input:   map[string]any{"server": "alpha"},
			wantErr: "missing required parameter",
		},
		{
			name:    "arguments wrong type",
			ctx:     mcpOrgCtx(),
			store:   &fakeMCPStore{servers: []model.MCPServer{server}},
			input:   map[string]any{"server": "alpha", "tool": "greet", "arguments": "nope"},
			wantErr: "must be an object",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeMCPClient{callResult: "hello world"}
			factory := &fakeFactory{byURL: map[string]*fakeMCPClient{"http://a": client}}
			tool := &mcpCallTool{store: tc.store, factory: factory}
			out, err := tool.Execute(tc.ctx, tc.input)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out != tc.wantResult {
				t.Fatalf("want result %q, got %q", tc.wantResult, out)
			}
			if !client.initialized {
				t.Fatal("client was not initialized before call")
			}
			if client.lastCall != "greet" {
				t.Fatalf("want tool call greet, got %q", client.lastCall)
			}
			if tc.wantArgs != nil {
				if client.lastArgs["who"] != tc.wantArgs["who"] {
					t.Fatalf("want args %v, got %v", tc.wantArgs, client.lastArgs)
				}
			}
		})
	}
}

func TestNewMCPToolsSet(t *testing.T) {
	got := NewMCPTools(&fakeMCPStore{})
	want := map[string]bool{"mcp_list_tools": true, "mcp_call": true}
	if len(got) != len(want) {
		t.Fatalf("want %d tools, got %d", len(want), len(got))
	}
	for _, tl := range got {
		if !want[tl.Name()] {
			t.Fatalf("unexpected tool %q", tl.Name())
		}
	}
}
