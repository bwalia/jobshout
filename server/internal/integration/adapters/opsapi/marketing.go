package opsapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// The OpsAPI Marketing Cloud plugin (opsapi projects/opsapi-marketing) keeps
// mail-merge templates as mkt_templates under /api/v2/opsapi-marketing. Its
// routes are guarded by the RBAC module opsapi_marketing_templates, so an
// opsk_ key reaches them only when it carries that scope
// ({"opsapi_marketing_templates":["create","update","read"]}) and the plugin
// is turned on for the key's namespace.

// marketingPrefix is the plugin's API prefix.
const marketingPrefix = "/api/v2/opsapi-marketing"

// MarketingTemplatesScope is the key scope the template calls need.
const MarketingTemplatesScope = "opsapi_marketing_templates"

// ErrMarketingScope: the key may not reach the Marketing templates (403) —
// it lacks the opsapi_marketing_templates scope.
var ErrMarketingScope = errors.New("missing " + MarketingTemplatesScope + " scope")

// ErrMarketingDisabled: the Marketing plugin is off (or not installed) for
// the namespace.
var ErrMarketingDisabled = errors.New("the OpsAPI Marketing plugin is not enabled for this namespace")

// MarketingTemplateRequest is a mail-merge template's content. Merge fields
// are kept as {{…}} tags; OpsAPI refuses to publish one that names a key
// outside its merge-field registry.
type MarketingTemplateRequest struct {
	Name        string `json:"name"`
	Channel     string `json:"channel"`
	Subject     string `json:"subject,omitempty"`
	PreviewText string `json:"preview_text,omitempty"`
	BodyHTML    string `json:"body_html,omitempty"`
	BodyText    string `json:"body_text,omitempty"`
	// Status is draft, published or archived. Empty on a studio save keeps
	// the template's current status.
	Status string `json:"status,omitempty"`
}

// MarketingTemplate is the subset of an mkt_templates row JobShout stores.
type MarketingTemplate struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Channel string `json:"channel"`
	Status  string `json:"status"`
	Subject string `json:"subject"`
}

// TemplateAnalysis is what OpsAPI's studio save reports about a template.
type TemplateAnalysis struct {
	OK           bool     `json:"ok"`
	Warnings     []string `json:"warnings"`
	MergeFields  []string `json:"merge_fields"`
	UnknownMerge []string `json:"unknown_merge"`
}

// CreateMarketingTemplate creates a template (POST /templates).
func (c *Client) CreateMarketingTemplate(ctx context.Context, req MarketingTemplateRequest) (*MarketingTemplate, error) {
	if req.Channel == "" {
		req.Channel = "email"
	}
	if req.Status == "" {
		req.Status = StatusDraft
	}
	resp, err := c.pageCall(ctx, http.MethodPost, marketingPrefix+"/templates", req)
	if err != nil {
		return nil, fmt.Errorf("opsapi: create template %q: %w", req.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, c.marketingError(resp, "create template "+strconvQuote(req.Name))
	}
	var env struct {
		Success bool              `json:"success"`
		Data    MarketingTemplate `json:"data"`
		Error   string            `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("opsapi: decode create template response: %w", err)
	}
	if !env.Success || env.Data.UUID == "" {
		return nil, fmt.Errorf("opsapi: create template %q: %s", req.Name, firstNonBlank(env.Error, "no template returned"))
	}
	return &env.Data, nil
}

// SaveMarketingTemplate saves content through the studio
// (PUT /templates/{uuid}/studio), which sanitises the HTML and reports
// unknown merge fields and other warnings.
func (c *Client) SaveMarketingTemplate(ctx context.Context, templateUUID string, req MarketingTemplateRequest) (*MarketingTemplate, *TemplateAnalysis, error) {
	templateUUID = strings.TrimSpace(templateUUID)
	if templateUUID == "" {
		return nil, nil, fmt.Errorf("opsapi: save template: no template uuid")
	}
	resp, err := c.pageCall(ctx, http.MethodPut, marketingPrefix+"/templates/"+url.PathEscape(templateUUID)+"/studio", req)
	if err != nil {
		return nil, nil, fmt.Errorf("opsapi: save template %s: %w", templateUUID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, c.marketingError(resp, "save template "+templateUUID)
	}
	var env struct {
		Success bool `json:"success"`
		Data    struct {
			Template MarketingTemplate `json:"template"`
			Analysis TemplateAnalysis  `json:"analysis"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, nil, fmt.Errorf("opsapi: decode save template response: %w", err)
	}
	if !env.Success {
		return nil, nil, fmt.Errorf("opsapi: save template %s: %s", templateUUID, env.Error)
	}
	t := env.Data.Template
	if t.UUID == "" {
		t.UUID = templateUUID
	}
	return &t, &env.Data.Analysis, nil
}

// FindMarketingTemplateByName returns the email template called name (exact,
// case-insensitive), or nil when there is none. The list endpoint only offers
// a fuzzy search, so results are filtered here.
func (c *Client) FindMarketingTemplateByName(ctx context.Context, name string) (*MarketingTemplate, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("opsapi: find template: empty name")
	}
	q := url.Values{"search": {name}, "channel": {"email"}, "per_page": {"100"}}
	resp, err := c.pageCall(ctx, http.MethodGet, marketingPrefix+"/templates?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("opsapi: find template %q: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.marketingError(resp, "find template "+strconvQuote(name))
	}
	var env struct {
		Success bool                `json:"success"`
		Data    []MarketingTemplate `json:"data"`
		Error   string              `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("opsapi: decode template list: %w", err)
	}
	for i := range env.Data {
		if strings.EqualFold(strings.TrimSpace(env.Data[i].Name), name) {
			return &env.Data[i], nil
		}
	}
	return nil, nil
}

// marketingError names the cause of a rejected Marketing call. A 403 wraps
// ErrMarketingScope; a 404 from a disabled plugin wraps ErrMarketingDisabled.
func (c *Client) marketingError(resp *http.Response, what string) error {
	body := readBodySnippet(resp)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("opsapi: %s rejected (401) — the API key is invalid, revoked or expired. Response: %s", what, body)
	case http.StatusForbidden:
		return fmt.Errorf("opsapi: %s forbidden (403): %w — the key for namespace %q needs %s create/update/read. Response: %s",
			what, ErrMarketingScope, c.cfg.Namespace, MarketingTemplatesScope, body)
	case http.StatusNotFound:
		if strings.Contains(body, "PLUGIN_DISABLED") || strings.Contains(strings.ToLower(body), "turned off") {
			return fmt.Errorf("opsapi: %s: %w (%q). Response: %s", what, ErrMarketingDisabled, c.cfg.Namespace, body)
		}
		return fmt.Errorf("opsapi: %s: not found in namespace %q. Response: %s", what, c.cfg.Namespace, body)
	default:
		return fmt.Errorf("opsapi: %s failed (status %d): %s", what, resp.StatusCode, body)
	}
}

func firstNonBlank(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
