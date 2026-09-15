package waflab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jobshout/server/internal/wslproxymcp"
)

// Single-record reads and traffic-split writes on the wslproxy admin API.
//
// The AB Testing agent reads a host's rule through these, and writes weights
// through them when wslproxy's MCP tools are switched off. Bodies match the
// MCP tools (update_traffic_split, promote_backend, rollback_backend), which
// call the same traffic_mgmt functions on the wslproxy side.

// GetServer GETs /api/servers/{id}. wslproxy answers {"data":{}} for an
// unknown id, which is returned as (nil, nil).
func (c *Client) GetServer(ctx context.Context, id, profileID string) (map[string]any, error) {
	return c.getRecord(ctx, "/api/servers/", id, profileID)
}

// GetRule GETs /api/rules/{id}. An unknown rule is returned as (nil, nil).
func (c *Client) GetRule(ctx context.Context, id, profileID string) (map[string]any, error) {
	return c.getRecord(ctx, "/api/rules/", id, profileID)
}

// UpdateTrafficWeights POSTs /api/traffic/backends/weights.
func (c *Client) UpdateTrafficWeights(ctx context.Context, ruleID, profileID string, backends []wslproxymcp.BackendWeight) (map[string]any, error) {
	return c.postTraffic(ctx, "/api/traffic/backends/weights", map[string]any{
		"rule_id":    ruleID,
		"profile_id": c.profileOr(profileID),
		"backends":   backends,
	})
}

// PromoteTrafficBackend POSTs /api/traffic/backends/promote.
func (c *Client) PromoteTrafficBackend(ctx context.Context, ruleID, label, profileID string) (map[string]any, error) {
	return c.postTraffic(ctx, "/api/traffic/backends/promote", map[string]any{
		"rule_id":       ruleID,
		"promote_label": label,
		"profile_id":    c.profileOr(profileID),
	})
}

// RollbackTrafficBackend POSTs /api/traffic/backends/rollback.
func (c *Client) RollbackTrafficBackend(ctx context.Context, ruleID, profileID string) (map[string]any, error) {
	return c.postTraffic(ctx, "/api/traffic/backends/rollback", map[string]any{
		"rule_id":    ruleID,
		"profile_id": c.profileOr(profileID),
	})
}

func (c *Client) profileOr(profileID string) string {
	if p := strings.TrimSpace(profileID); p != "" {
		return p
	}
	return c.cfg.Profile
}

func (c *Client) getRecord(ctx context.Context, prefix, id, profileID string) (map[string]any, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("wslproxy: empty id")
	}
	path := prefix + url.PathEscape(id) + "?envprofile=" + url.QueryEscape(c.profileOr(profileID))
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp, true); err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return nil, nil
		}
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, nil
	}
	return resp.Data, nil
}

func (c *Client) postTraffic(ctx context.Context, path string, body map[string]any) (map[string]any, error) {
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodPost, path, body, &raw, true); err != nil {
		return nil, err
	}
	var resp struct {
		Data    map[string]any `json:"data"`
		Status  int            `json:"status"`
		Message string         `json:"message"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("wslproxy decode %s: %w", path, err)
		}
	}
	// traffic_mgmt reports failures in the body; the HTTP status normally
	// matches, but do not trust a 200 that carries an error status.
	if resp.Status >= 300 {
		return nil, fmt.Errorf("wslproxy POST %s: HTTP %d: %s", path, resp.Status, resp.Message)
	}
	if resp.Data == nil {
		resp.Data = map[string]any{}
	}
	return resp.Data, nil
}
