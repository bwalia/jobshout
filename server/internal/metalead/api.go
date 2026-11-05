package metalead

import (
	"context"
	"encoding/json"
	"errors"
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
	// Get reads a Graph object or edge ("123", "123/conversations") into dest;
	// q holds fields, limit and the edge's other parameters.
	Get(ctx context.Context, path string, q url.Values, token string, dest any) error
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

// NewClient builds a Graph client for version (e.g. v26.0).
func NewClient(version string) *Client {
	if strings.TrimSpace(version) == "" {
		version = "v26.0"
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
	var leads []LeadSummary
	for page := 0; page < maxGraphPages && len(leads) < limit; page++ {
		u := fmt.Sprintf("%s/%s/%s/leads?%s", c.base(), c.Version, url.PathEscape(formID), q.Encode())
		var out struct {
			Data []struct {
				ID          string      `json:"id"`
				CreatedTime string      `json:"created_time"`
				FieldData   []FieldData `json:"field_data"`
			} `json:"data"`
			Paging graphPaging `json:"paging"`
		}
		if err := c.getJSON(ctx, u, &out); err != nil {
			return nil, err
		}
		for _, d := range out.Data {
			leads = append(leads, LeadSummary{ID: d.ID, CreatedTime: d.CreatedTime, FieldData: d.FieldData})
		}
		if q = nextPage(out.Paging); q == nil {
			break
		}
		q.Set("access_token", token)
	}
	if len(leads) > limit {
		leads = leads[:limit]
	}
	return leads, nil
}

// maxGraphPages bounds how many pages one Graph list is read for, whatever
// the caller still wants: a guard against a list that never ends.
const maxGraphPages = 20

// graphPaging is a Graph list's paging block. Lists page by cursor or by time
// (until + __paging_token) depending on the edge; next carries either.
type graphPaging struct {
	Next string `json:"next"`
}

// nextPage is the query for the page after this one, taken from Graph's own
// next link without its access_token, or nil on the last page. Only the query
// is reused: the request still goes to our base URL and path.
func nextPage(p graphPaging) url.Values {
	if p.Next == "" {
		return nil
	}
	u, err := url.Parse(p.Next)
	if err != nil {
		return nil
	}
	q := u.Query()
	q.Del("access_token")
	return q
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

func (c *Client) Get(ctx context.Context, path string, q url.Values, token string, dest any) error {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	params := url.Values{}
	for k, v := range q {
		params[k] = v
	}
	params.Set("access_token", token)
	return c.getJSON(ctx, fmt.Sprintf("%s/%s/%s?%s", c.base(), c.Version, strings.Join(segs, "/"), params.Encode()), dest)
}

// fields is the query for a plain field read.
func fields(f string) url.Values { return url.Values{"fields": {f}} }

func (c *Client) getJSON(ctx context.Context, rawURL string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		// A *url.Error prints the URL, access_token and all; keep only its cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
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
