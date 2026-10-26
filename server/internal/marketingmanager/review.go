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

// Thresholds for review findings. Opens are pixel-based and overstated by
// privacy proxies, so the open-rate bar is a prompt to look, not a verdict.
const (
	minSentToJudge   = 20
	bounceRateHigh   = 2.0  // %
	openRateLow      = 15.0 // %, estimated
	clickToOpenLow   = 10.0 // %
	skippedShareHigh = 0.25
)

// CampaignReview is one campaign's numbers and what to do about them.
type CampaignReview struct {
	UUID     string         `json:"uuid"`
	Name     string         `json:"name"`
	Status   string         `json:"status"`
	Sent     int            `json:"sent"`
	Rates    campaignRates  `json:"rates"`
	Counts   deliveryCounts `json:"deliveries"`
	ABWinner *abWinner      `json:"ab_winner,omitempty"`
	Findings []string       `json:"findings"`
}

// Report is a Launch review of a workspace's recent campaigns.
type Report struct {
	Website   string           `json:"website"`
	Namespace string           `json:"namespace"`
	Sending   *Sending         `json:"sending,omitempty"`
	Campaigns []CampaignReview `json:"campaigns"`
	Workspace []string         `json:"workspace_findings"`
}

type campaignRates struct {
	OpenRate       float64 `json:"open_rate"`
	ClickRate      float64 `json:"click_rate"`
	ClickToOpen    float64 `json:"click_to_open_rate"`
	ConversionRate float64 `json:"conversion_rate"`
	BounceRate     float64 `json:"bounce_rate"`
}

type deliveryCounts struct {
	Total     int `json:"total"`
	Sent      int `json:"sent"`
	Bounced   int `json:"bounced"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
	Queued    int `json:"queued"`
	Opened    int `json:"opened_estimated"`
	Clicked   int `json:"clicked"`
	Converted int `json:"converted"`
}

type abWinner struct {
	Variant     string  `json:"variant"`
	Metric      string  `json:"metric"`
	Rate        float64 `json:"rate"`
	RunnerUp    string  `json:"runner_up"`
	RunnerRate  float64 `json:"runner_up_rate"`
	Confidence  float64 `json:"confidence"`
	Significant bool    `json:"significant"`
}

type campaignRow struct {
	UUID   string `json:"uuid"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Review reads recent campaigns (or one) and judges each. Reads only.
func (s *Service) Review(ctx context.Context, orgID uuid.UUID, website, campaign string, limit int) (*Report, error) {
	c, err := s.Connect(ctx, orgID, website)
	if err != nil {
		return nil, err
	}
	rep := &Report{Website: c.Website(), Namespace: c.Namespace()}
	if st, err := c.Sending(ctx); err == nil {
		rep.Sending = st
		rep.Workspace = append(rep.Workspace, sendingFindings(st)...)
	} else {
		rep.Workspace = append(rep.Workspace, "Could not read sending settings: "+err.Error())
	}

	var rows []campaignRow
	if campaign = strings.TrimSpace(campaign); campaign != "" {
		if !idRe.MatchString(campaign) {
			return nil, fmt.Errorf("%q is not a campaign id", campaign)
		}
		resp, err := c.Do(ctx, http.MethodGet, marketingPrefix+"/campaigns/"+campaign, nil, nil, "")
		if err != nil {
			return nil, err
		}
		var r campaignRow
		if err := json.Unmarshal(resp.Data, &r); err != nil {
			return nil, fmt.Errorf("decode campaign: %w", err)
		}
		rows = []campaignRow{r}
	} else {
		if limit <= 0 || limit > 20 {
			limit = 10
		}
		q := url.Values{"per_page": {fmt.Sprint(limit)}, "sort": {"updated_at"}, "order": {"desc"}}
		resp, err := c.Do(ctx, http.MethodGet, marketingPrefix+"/campaigns", q, nil, "")
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(resp.Data, &rows); err != nil {
			return nil, fmt.Errorf("decode campaigns: %w", err)
		}
	}

	waiting := 0
	for _, r := range rows {
		switch r.Status {
		case "awaiting_approval":
			waiting++
			continue
		case "draft", "validated":
			continue
		}
		cr, err := reviewCampaign(ctx, c, r)
		if err != nil {
			cr = CampaignReview{UUID: r.UUID, Name: r.Name, Status: r.Status, Findings: []string{"Analytics unavailable: " + err.Error()}}
		}
		rep.Campaigns = append(rep.Campaigns, cr)
	}
	if waiting > 0 {
		rep.Workspace = append(rep.Workspace, fmt.Sprintf("%d campaign(s) are awaiting approval: a workspace admin other than the author approves them in OpsAPI.", waiting))
	}
	if len(rep.Campaigns) == 0 {
		rep.Workspace = append(rep.Workspace, "No sent campaigns to review yet.")
	}
	return rep, nil
}

func reviewCampaign(ctx context.Context, c *Client, r campaignRow) (CampaignReview, error) {
	cr := CampaignReview{UUID: r.UUID, Name: r.Name, Status: r.Status}
	resp, err := c.Do(ctx, http.MethodGet, marketingPrefix+"/analytics/campaigns/"+url.PathEscape(r.UUID), nil, nil, "")
	if err != nil {
		return cr, err
	}
	var a struct {
		Deliveries deliveryCounts `json:"deliveries"`
		Rates      campaignRates  `json:"rates"`
		ABWinner   *abWinner      `json:"ab_winner"`
	}
	if err := json.Unmarshal(resp.Data, &a); err != nil {
		return cr, fmt.Errorf("decode analytics: %w", err)
	}
	cr.Counts, cr.Rates, cr.Sent = a.Deliveries, a.Rates, a.Deliveries.Sent
	if a.ABWinner != nil && a.ABWinner.Variant != "" {
		cr.ABWinner = a.ABWinner
	}
	cr.Findings = judge(cr)
	return cr, nil
}

// judge turns one campaign's numbers into concrete suggestions.
func judge(cr CampaignReview) []string {
	var f []string
	d, r := cr.Counts, cr.Rates
	if d.Failed > 0 {
		f = append(f, fmt.Sprintf("%d deliveries failed: check the workspace SMTP settings and the daily limit (status shows both) before sending more.", d.Failed))
	}
	if d.Queued > 0 && cr.Status != "sending" && cr.Status != "scheduled" {
		f = append(f, fmt.Sprintf("%d deliveries are still queued while the campaign is %s.", d.Queued, cr.Status))
	}
	if cr.Sent < minSentToJudge {
		if cr.Sent > 0 {
			f = append(f, fmt.Sprintf("Only %d sent — too few to judge rates.", cr.Sent))
		}
		return append(f, skippedFinding(d)...)
	}
	if r.BounceRate > bounceRateHigh {
		f = append(f, fmt.Sprintf("Bounce rate %.1f%% is above %.0f%%: clean the list (bounced addresses are suppressed automatically) and verify bought or imported contacts before the next send — high bounces hurt sender reputation.", r.BounceRate, bounceRateHigh))
	}
	if r.OpenRate < openRateLow {
		f = append(f, fmt.Sprintf("Estimated open rate %.1f%% is low: test a shorter, specific subject and a preheader that continues it (set_ab_test), and check the from name is one recipients know.", r.OpenRate))
	}
	if d.Opened > 0 && r.ClickToOpen < clickToOpenLow {
		f = append(f, fmt.Sprintf("Click-to-open %.1f%%: people open but don't click — use one clear call to action near the top and cut competing links.", r.ClickToOpen))
	}
	if w := cr.ABWinner; w != nil {
		if w.Significant {
			f = append(f, fmt.Sprintf("A/B: variant %s wins on %s (%.1f%% vs %.1f%%, %.0f%% confidence) — use it for the rest of this audience.", w.Variant, w.Metric, w.Rate, w.RunnerRate, w.Confidence))
		} else {
			f = append(f, fmt.Sprintf("A/B: %s leads %s on %s but at %.0f%% confidence that is not yet significant.", w.Variant, w.RunnerUp, w.Metric, w.Confidence))
		}
	}
	f = append(f, skippedFinding(d)...)
	if len(f) == 0 {
		f = append(f, "No problems found in the numbers.")
	}
	return f
}

func skippedFinding(d deliveryCounts) []string {
	if d.Total == 0 || float64(d.Skipped)/float64(d.Total) <= skippedShareHigh {
		return nil
	}
	return []string{fmt.Sprintf("%d of %d deliveries were skipped (no consent, suppressed, quiet hours or the frequency cap): audience_preview before sending shows who will be left out.", d.Skipped, d.Total)}
}

func sendingFindings(st *Sending) []string {
	var f []string
	if st.DryRun {
		f = append(f, "Test mode (dry run) is ON: sends are recorded but nothing leaves, so delivery numbers are not real sends.")
	}
	if st.Via == "none" {
		f = append(f, "No SMTP is configured: nothing can be delivered until Workspace → Email is set up.")
	}
	if st.PublicURL == "" {
		f = append(f, "No public tracking/unsubscribe URL: email campaigns can't be validated until one is set.")
	}
	if !st.SenderAddressSet {
		f = append(f, "No business postal address for the footer: UK PECR and US CAN-SPAM expect one in promotional email.")
	}
	if st.DailyLimit > 0 && st.SentLast24h*10 >= st.DailyLimit*9 {
		f = append(f, fmt.Sprintf("%d of the %d daily email limit used in the last 24 hours.", st.SentLast24h, st.DailyLimit))
	}
	return f
}

// Markdown renders the report for a board task.
func (r *Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Marketing review — %s\n\n", r.Namespace)
	if r.Sending != nil {
		mode := "LIVE"
		if r.Sending.DryRun {
			mode = "test mode (dry run)"
		}
		fmt.Fprintf(&b, "Sending: %s via %s, from %s.\n\n", mode, r.Sending.Via, nonEmpty(r.Sending.FromEmail, "—"))
	}
	if len(r.Workspace) > 0 {
		b.WriteString("### Workspace\n")
		for _, f := range r.Workspace {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\n")
	}
	for _, c := range r.Campaigns {
		fmt.Fprintf(&b, "### %s (%s)\n", c.Name, c.Status)
		if c.Sent > 0 {
			fmt.Fprintf(&b, "Sent %d · opens %.1f%% (est.) · clicks %.1f%% · click-to-open %.1f%% · bounces %.1f%% · failed %d · skipped %d\n",
				c.Sent, c.Rates.OpenRate, c.Rates.ClickRate, c.Rates.ClickToOpen, c.Rates.BounceRate, c.Counts.Failed, c.Counts.Skipped)
		}
		for _, f := range c.Findings {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func nonEmpty(s, alt string) string {
	if strings.TrimSpace(s) == "" {
		return alt
	}
	return s
}
