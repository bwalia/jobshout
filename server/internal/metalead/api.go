package metalead

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Graph is the Meta Graph API surface this agent needs.
type Graph interface {
	ListForms(ctx context.Context, pageID, token string) ([]FormSummary, error)
	ListLeads(ctx context.Context, formID, token string, since *time.Time, limit int) ([]LeadSummary, error)
	GetLead(ctx context.Context, leadgenID, token string) (*LeadDetail, error)
}

// FormSummary is a Lead Ads form on a Page.
type FormSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// LeadSummary is a row from GET /{form_id}/leads.
type LeadSummary struct {
	ID          string `json:"id"`
	CreatedTime string `json:"created_time"`
	FieldData   []FieldData
}

// FieldData is one Instant Form answer.
type FieldData struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// LeadDetail is GET /{leadgen_id}.
type LeadDetail struct {
	ID          string      `json:"id"`
	CreatedTime string      `json:"created_time"`
	FormID      string      `json:"form_id"`
	FieldData   []FieldData `json:"field_data"`
}

// Client talks to graph.facebook.com.
type Client struct {
	HTTP    *http.Client
	Version string
	BaseURL string // override for tests
}

// NewClient builds a Graph client for version (e.g. v21.0).
func NewClient(version string) *Client {
	if strings.TrimSpace(version) == "" {
		version = "v21.0"
	}
	return &Client{
		HTTP:    &http.Client{Timeout: 45 * time.Second},
		Version: version,
		BaseURL: "https://graph.facebook.com",
	}
}

func (c *Client) base() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return "https://graph.facebook.com"
}

func (c *Client) ListForms(ctx context.Context, pageID, token string) ([]FormSummary, error) {
	u := fmt.Sprintf("%s/%s/%s/leadgen_forms?fields=id,name&limit=100&access_token=%s",
		c.base(), c.Version, url.PathEscape(pageID), url.QueryEscape(token))
	var out struct {
		Data []FormSummary `json:"data"`
	}
	if err := c.getJSON(ctx, u, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (c *Client) ListLeads(ctx context.Context, formID, token string, since *time.Time, limit int) ([]LeadSummary, error) {
	if limit <= 0 {
		limit = 25
	}
	q := url.Values{}
	q.Set("fields", "id,created_time,field_data")
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("access_token", token)
	if since != nil && !since.IsZero() {
		q.Set("filtering", fmt.Sprintf(
			`[{"field":"time_created","operator":"GREATER_THAN","value":%d}]`,
			since.Unix(),
		))
	}
	u := fmt.Sprintf("%s/%s/%s/leads?%s", c.base(), c.Version, url.PathEscape(formID), q.Encode())
	var out struct {
		Data []struct {
			ID          string      `json:"id"`
			CreatedTime string      `json:"created_time"`
			FieldData   []FieldData `json:"field_data"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, u, &out); err != nil {
		return nil, err
	}
	leads := make([]LeadSummary, 0, len(out.Data))
	for _, d := range out.Data {
		leads = append(leads, LeadSummary{
			ID:          d.ID,
			CreatedTime: d.CreatedTime,
			FieldData:   d.FieldData,
		})
	}
	return leads, nil
}

func (c *Client) GetLead(ctx context.Context, leadgenID, token string) (*LeadDetail, error) {
	u := fmt.Sprintf("%s/%s/%s?fields=id,created_time,form_id,field_data&access_token=%s",
		c.base(), c.Version, url.PathEscape(leadgenID), url.QueryEscape(token))
	var d LeadDetail
	if err := c.getJSON(ctx, u, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) getJSON(ctx context.Context, rawURL string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("meta graph: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("meta graph HTTP %d: %s", resp.StatusCode, truncate(string(body), 400))
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("meta graph decode: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// FieldDataJSON serialises field_data for storage.
func FieldDataJSON(fields []FieldData) json.RawMessage {
	if fields == nil {
		fields = []FieldData{}
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return json.RawMessage("[]")
	}
	return b
}

// ParseCreatedTime accepts Graph timestamps.
func ParseCreatedTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05+0000", "2006-01-02T15:04:05Z0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}
