package marketingmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Send modes reported on anything that delivers.
const (
	ModeDryRun = "dry_run"
	ModeLive   = "live"
)

// ErrLiveSendBlocked is a send refused because it would email real people.
var ErrLiveSendBlocked = errors.New("live send blocked")

// Sending is the part of OpsAPI's GET /sending the agent reads.
type Sending struct {
	Via              string `json:"via"` // workspace | deployment | none
	FromEmail        string `json:"from_email"`
	FromName         string `json:"from_name"`
	DryRun           bool   `json:"dry_run"`
	DailyLimit       int    `json:"daily_limit"`
	SentLast24h      int    `json:"sent_last_24h"`
	PublicURL        string `json:"public_url"`
	SenderAddressSet bool   `json:"sender_address_set"`
}

// Sending reads how the workspace sends, test mode included.
func (c *Client) Sending(ctx context.Context) (*Sending, error) {
	resp, err := c.Do(ctx, http.MethodGet, marketingPrefix+"/sending", nil, nil, "")
	if err != nil {
		return nil, err
	}
	var s Sending
	if err := json.Unmarshal(resp.Data, &s); err != nil {
		return nil, fmt.Errorf("decode sending settings: %w", err)
	}
	return &s, nil
}

// gate decides whether a delivering call may go ahead, at run time — after
// the chat confirmation, never instead of it:
//
//   - the workspace is in test mode (dry_run): yes, nothing leaves OpsAPI;
//   - otherwise this server must allow live sends (MARKETING_LIVE_SEND) AND
//     the call must carry live_send_approved, which the user confirmed.
//
// If test mode can't be read, the answer is no: an unknown is treated as live.
func (s *Service) gate(ctx context.Context, c *Client, call Call) (string, error) {
	st, err := c.Sending(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: could not confirm whether the workspace is in test mode, so nothing was sent: %v", ErrLiveSendBlocked, err)
	}
	if st.DryRun {
		return ModeDryRun, nil
	}
	if !s.AllowLiveSend {
		return "", fmt.Errorf("%w: workspace %q is not in test mode, so this would email real people, and live sending is switched off on this JobShout server. "+
			"To try it safely, turn on \"Test mode: don't really send\" in OpsAPI → Workspace → Plugins → Marketing Cloud. "+
			"Live sends need an operator to set MARKETING_LIVE_SEND=true on this ring", ErrLiveSendBlocked, c.Namespace())
	}
	if !call.LiveSendApproved {
		return "", fmt.Errorf("%w: workspace %q is not in test mode — this sends real email. Ask the user to approve a live send explicitly, then call again with live_send_approved=true", ErrLiveSendBlocked, c.Namespace())
	}
	return ModeLive, nil
}

// Describe is the confirmation text for a send-class call: what will happen,
// to how many people, and whether it is real. Best-effort and read-only; a
// failure to look something up says so rather than guessing.
func (s *Service) Describe(ctx context.Context, orgID uuid.UUID, call Call) string {
	op, ok := Lookup(call.Action)
	if !ok {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c, err := s.Connect(ctx, orgID, call.Website)
	if err != nil {
		return fmt.Sprintf("%s: %s. (Workspace not resolved: %v)", op.Name, op.Desc, err)
	}
	where := fmt.Sprintf("OpsAPI workspace %q", c.Namespace())

	var st *Sending
	mode := ""
	if op.Delivers || op.Name == "schedule_campaign" {
		st, _ = c.Sending(ctx)
	}
	if op.Delivers {
		mode = " Test mode could not be read — treat this as a LIVE send; it will be refused unless test mode is confirmed or live sending is approved."
		if st != nil {
			switch {
			case st.DryRun:
				mode = " TEST MODE (dry run) is on: OpsAPI records it, but no email leaves."
			case !s.AllowLiveSend:
				mode = " LIVE workspace (test mode off). This server blocks live sends, so it will be refused."
			default:
				mode = " LIVE: real email goes to real people and cannot be recalled."
			}
		}
	}

	camp := s.campaign(ctx, c, op, call)
	name := camp.label(call.ID)
	switch op.Name {
	case "schedule_campaign":
		return s.describeSchedule(ctx, c, call, camp, st, where) + mode
	case "test_send":
		to := str(call.Body["email"])
		if to == "" {
			to = "contact " + str(call.Body["contact_uuid"])
		}
		subject := ""
		if camp.Subject != "" {
			subject = fmt.Sprintf(" (subject %q)", camp.Subject)
		}
		return fmt.Sprintf("Send one test of campaign %s%s to %s in %s. OpsAPI creates a CRM contact for a new address.%s", name, subject, to, where, mode)
	case "resume_campaign":
		return fmt.Sprintf("Resume campaign %s in %s: the rest of its audience is sent to%s.%s", name, where, s.audience(ctx, c, call), mode)
	case "cancel_campaign":
		return fmt.Sprintf("Cancel campaign %s in %s for good: queued deliveries are skipped. To send again, duplicate it.", name, where)
	case "import_contacts":
		rows := 0
		if csv := str(call.Body["csv"]); csv != "" {
			for _, l := range strings.Split(strings.TrimSpace(csv), "\n") {
				if strings.TrimSpace(l) != "" {
					rows++
				}
			}
			rows-- // header
		}
		consent := ""
		if v, _ := call.Body["grant_consent"].(bool); v {
			consent = " and records their consent"
		}
		return fmt.Sprintf("Import %d CSV rows into list %s in %s: creates CRM contacts for new addresses%s. Addresses that unsubscribed are never re-added.", rows, s.listName(ctx, c, call.ID, 0), where, consent)
	case "opt_out":
		return fmt.Sprintf("Unsubscribe contact %s in %s. Only the contact can undo this.", str(call.Body["contact_uuid"]), where)
	case "opt_in":
		return fmt.Sprintf("Record consent for contact %s in %s. This is a legal record of permission to market to them.", str(call.Body["contact_uuid"]), where)
	case "unblock_address":
		return fmt.Sprintf("Remove block %s in %s: the address can be sent to again.", call.ID, where)
	}
	return fmt.Sprintf("%s in %s: %s.%s", op.Name, where, op.Desc, mode)
}

// describeSchedule is everything the user must see before a campaign goes
// out: workspace, sender, subject, content source, audience list and size,
// and when.
func (s *Service) describeSchedule(ctx context.Context, c *Client, call Call, camp campaignInfo, st *Sending, where string) string {
	from := camp.from()
	if from == "" && st != nil {
		from = strings.TrimSpace(fmt.Sprintf("%s <%s>", st.FromName, st.FromEmail))
		if st.FromEmail == "" {
			from = ""
		} else {
			from += " (workspace default)"
		}
	}
	content := "the content saved on the campaign"
	if camp.TemplateID != 0 {
		content = "template " + s.templateName(ctx, c, camp.TemplateID)
	}
	list := str(call.Body["list_uuid"])
	audience := "no audience attached — it will be refused"
	if list != "" || camp.ListID != 0 {
		audience = "list " + s.listName(ctx, c, list, camp.ListID)
	}
	when := "now"
	if at := str(call.Body["scheduled_at"]); at != "" {
		when = "at " + at + " (it sends automatically then)"
	}
	if v, _ := call.Body["send_now"].(bool); v {
		when = "now"
	}
	limit := ""
	if n := str(call.Body["max_deliveries"]); n != "" {
		limit = "; capped at " + n + " deliveries"
	}
	return fmt.Sprintf("Send campaign %s in %s. Sender: %s. Subject: %s. Content: %s. Audience: %s%s%s. Schedule: %s.",
		camp.label(call.ID), where, nonEmpty(from, "not set"), nonEmpty(quoteOr(camp.Subject), "not set"),
		content, audience, s.audience(ctx, c, call), limit, when)
}

// campaignInfo is what a confirmation shows about a campaign.
type campaignInfo struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Subject    string `json:"subject"`
	FromEmail  string `json:"from_email"`
	FromName   string `json:"from_name"`
	ListID     flexID `json:"list_id"`
	TemplateID flexID `json:"template_id"`
}

func (ci campaignInfo) label(id string) string {
	if ci.Name == "" {
		return id
	}
	return fmt.Sprintf("“%s” (%s)", ci.Name, ci.Status)
}

func (ci campaignInfo) from() string {
	if ci.FromEmail == "" {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%s <%s>", ci.FromName, ci.FromEmail))
}

func (s *Service) campaign(ctx context.Context, c *Client, op Op, call Call) campaignInfo {
	if !strings.HasPrefix(op.Path, "/campaigns/{id}") || call.ID == "" || !idRe.MatchString(call.ID) {
		return campaignInfo{}
	}
	resp, err := c.Do(ctx, http.MethodGet, marketingPrefix+"/campaigns/"+call.ID, nil, nil, "")
	if err != nil {
		return campaignInfo{}
	}
	var ci campaignInfo
	_ = json.Unmarshal(resp.Data, &ci)
	return ci
}

// flexID is a numeric id OpsAPI may send as a number, a string or null.
type flexID int64

func (f *flexID) UnmarshalJSON(b []byte) error {
	n, err := strconv.ParseInt(strings.Trim(string(b), `"`), 10, 64)
	if err == nil {
		*f = flexID(n)
	}
	return nil
}

// listName names a list by uuid, or by the numeric id a campaign stores.
func (s *Service) listName(ctx context.Context, c *Client, uuid string, id flexID) string {
	return s.recordName(ctx, c, "/lists", uuid, id)
}

func (s *Service) templateName(ctx context.Context, c *Client, id flexID) string {
	return s.recordName(ctx, c, "/templates", "", id)
}

// recordName finds a list's or template's name in its first page; the uuid
// (or id) itself when it can't. Show routes take only a uuid, and campaigns
// store numeric ids, hence the page scan.
func (s *Service) recordName(ctx context.Context, c *Client, path, uuid string, id flexID) string {
	fallback := uuid
	if fallback == "" {
		fallback = fmt.Sprintf("#%d", id)
	}
	resp, err := c.Do(ctx, http.MethodGet, marketingPrefix+path, url.Values{"per_page": {"100"}}, nil, "")
	if err != nil {
		return fallback
	}
	var rows []struct {
		ID   flexID `json:"id"`
		UUID string `json:"uuid"`
		Name string `json:"name"`
	}
	if json.Unmarshal(resp.Data, &rows) != nil {
		return fallback
	}
	for _, r := range rows {
		if (uuid != "" && strings.EqualFold(r.UUID, uuid)) || (uuid == "" && id != 0 && r.ID == id) {
			return fmt.Sprintf("%q", r.Name)
		}
	}
	return fallback
}

// audience is " — N eligible recipients of M (K suppressed, J without
// consent…)" from OpsAPI's read-only preview, which applies the same consent
// and suppression checks as the send.
func (s *Service) audience(ctx context.Context, c *Client, call Call) string {
	if call.ID == "" || !idRe.MatchString(call.ID) {
		return ""
	}
	body := map[string]any{"skip_quiet_hours": true}
	if l := str(call.Body["list_uuid"]); l != "" {
		body["list_uuid"] = l
	}
	resp, err := c.Do(ctx, http.MethodPost, marketingPrefix+"/campaigns/"+call.ID+"/audience-preview", nil, body, "")
	if err != nil {
		return " (audience size could not be read)"
	}
	var p struct {
		Total      int  `json:"audience_total"`
		Eligible   int  `json:"eligible"`
		Suppressed int  `json:"suppressed"`
		NoConsent  int  `json:"no_consent"`
		NoAddress  int  `json:"missing_address"`
		Other      int  `json:"other_excluded"`
		Exact      bool `json:"exact"`
	}
	if json.Unmarshal(resp.Data, &p) != nil {
		return ""
	}
	approx := ""
	if !p.Exact {
		approx = "about "
	}
	return fmt.Sprintf(" — %s%d eligible recipients of %d (excluded: %d suppressed, %d without consent, %d without an address, %d other)",
		approx, p.Eligible, p.Total, p.Suppressed, p.NoConsent, p.NoAddress, p.Other)
}

func quoteOr(s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("%q", s)
}
