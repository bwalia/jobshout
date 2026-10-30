package marketingmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

// statusReport is the set-up check: how email goes out, test mode, and what
// the workspace has in it. Reads only.
func (s *Service) statusReport(ctx context.Context, orgID uuid.UUID, website string) (string, map[string]any, error) {
	c, err := s.Connect(ctx, orgID, website)
	if err != nil {
		return "", nil, err
	}
	st, err := c.Sending(ctx)
	if err != nil {
		return "", nil, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Marketing set-up — %s\n\n", c.Namespace())
	mode := "LIVE — sends reach real inboxes"
	if st.DryRun {
		mode = "test mode (dry run) — nothing leaves OpsAPI"
	}
	fmt.Fprintf(&b, "- Sending: %s\n- SMTP: %s; from %s <%s>\n", mode, st.Via, nonEmpty(st.FromName, "—"), nonEmpty(st.FromEmail, "—"))
	if st.DailyLimit > 0 {
		fmt.Fprintf(&b, "- Daily limit: %d (used %d in 24h)\n", st.DailyLimit, st.SentLast24h)
	}
	fmt.Fprintf(&b, "- Public tracking/unsubscribe URL: %s\n", nonEmpty(st.PublicURL, "not set"))
	if !st.DryRun && !s.AllowLiveSend {
		b.WriteString("- This JobShout server blocks live sends (MARKETING_LIVE_SEND off): turn on test mode to try sends.\n")
	}

	meta := map[string]any{"marketing_namespace": c.Namespace(), "dry_run": st.DryRun}
	var lists []struct {
		UUID     string `json:"uuid"`
		Name     string `json:"name"`
		ListType string `json:"list_type"`
	}
	if resp, err := c.Do(ctx, http.MethodGet, marketingPrefix+"/lists", url.Values{"per_page": {"50"}}, nil, ""); err == nil {
		_ = json.Unmarshal(resp.Data, &lists)
	}
	counts := map[string]int{}
	if resp, err := c.Do(ctx, http.MethodGet, marketingPrefix+"/list-counts", nil, nil, ""); err == nil {
		_ = json.Unmarshal(resp.Data, &counts)
	}
	fmt.Fprintf(&b, "\n### Lists (%d)\n", len(lists))
	for _, l := range lists {
		fmt.Fprintf(&b, "- %s (%s): %d members\n", l.Name, l.ListType, counts[l.UUID])
	}
	if total, err := c.total(ctx, "/templates"); err == nil {
		fmt.Fprintf(&b, "\n### Templates: %d\n", total)
		meta["templates"] = total
	}
	if resp, err := c.Do(ctx, http.MethodGet, marketingPrefix+"/analytics/overview", nil, nil, ""); err == nil {
		var o struct {
			Campaigns struct {
				Drafts, Active, Completed, Paused int
			} `json:"campaigns"`
			Unsubscribed int `json:"unsubscribed_30d"`
		}
		if json.Unmarshal(resp.Data, &o) == nil {
			fmt.Fprintf(&b, "\n### Campaigns\n- %d draft, %d scheduled or sending, %d paused, %d completed\n- %d unsubscribes in 30 days\n",
				o.Campaigns.Drafts, o.Campaigns.Active, o.Campaigns.Paused, o.Campaigns.Completed, o.Unsubscribed)
		}
	}
	meta["lists"] = len(lists)
	return strings.TrimSpace(b.String()), meta, nil
}

// total reads meta.total of a sdk.crud list.
func (c *Client) total(ctx context.Context, path string) (int, error) {
	resp, err := c.Do(ctx, http.MethodGet, marketingPrefix+path, url.Values{"per_page": {"1"}}, nil, "")
	if err != nil {
		return 0, err
	}
	var m struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(resp.Meta, &m); err != nil {
		return 0, err
	}
	return m.Total, nil
}
