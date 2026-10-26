package marketingmanager

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// Kind is how much an operation can change. It decides which chat tool
// carries it, and so whether the chat guard asks the user first.
type Kind int

const (
	// Read changes nothing.
	Read Kind = iota
	// Write changes marketing records but sends nothing and can be put back
	// (edit a template, add a list member, pause a campaign).
	Write
	// Send reaches people, or cannot be undone, or acts in bulk: the guard
	// holds it for the user's confirmation every time.
	Send
)

func (k Kind) String() string {
	switch k {
	case Read:
		return "read"
	case Write:
		return "write"
	}
	return "send"
}

// Op is one OpsAPI call the agent may make. The model chooses an Op by name
// and supplies ids, query and body; it never supplies a path or a method.
type Op struct {
	Name   string
	Kind   Kind
	Method string
	// Path is relative to the Marketing Cloud prefix unless it starts with
	// /api/v2/ (CRM). {id} and {contact_uuid} come from the call's args.
	Path string
	// Query names the query parameters passed through.
	Query []string
	// Body: the call sends args.body (a JSON object, {} when absent).
	Body bool
	// Delivers: the call can put a message in someone's inbox, so it passes
	// the live-send gate (see gate.go).
	Delivers bool
	// Idempotent creates send an Idempotency-Key.
	Idempotent bool
	// Lean drops body_html/body_text from list items: big, and fetched
	// one record at a time when needed.
	Lean bool
	Desc string
}

const crmContacts = "/api/v2/crm/contacts"

var (
	pageQ     = []string{"page", "per_page", "search", "sort", "order"}
	campaignQ = append([]string{"status", "channel", "message_type"}, pageQ...)
	templateQ = append([]string{"status", "channel"}, pageQ...)
	listQ     = append([]string{"list_type", "channel"}, pageQ...)
)

// ops is every call the Marketing Manager can make. Left out on purpose:
// approve/reject (a person signs a campaign off — four-eyes), deleting
// campaigns, lists, templates or contacts, and the plugin's settings
// (test mode, sender, rate limits): those stay with people in OpsAPI.
var ops = []Op{
	// --- Workspace ---
	{Name: "status", Kind: Read, Method: http.MethodGet, Path: "/sending",
		Desc: "How email goes out here: SMTP in use, from address, dry_run (test mode), daily limit and use, public URL"},

	// --- Templates ---
	{Name: "list_templates", Kind: Read, Method: http.MethodGet, Path: "/templates", Query: templateQ, Lean: true,
		Desc: "Templates (query: search, status, channel, page, per_page)"},
	{Name: "get_template", Kind: Read, Method: http.MethodGet, Path: "/templates/{id}", Desc: "One template with its HTML (id)"},
	{Name: "preview_template", Kind: Read, Method: http.MethodPost, Path: "/templates/preview", Body: true,
		Desc: "Render with sample data (body: template_uuid, or subject/body_html/body_text; sample)"},
	{Name: "analyse_template", Kind: Read, Method: http.MethodPost, Path: "/templates/analyse", Body: true,
		Desc: "Check merge fields, links and warnings without saving (body: channel, subject, body_html, body_text)"},
	{Name: "list_merge_fields", Kind: Read, Method: http.MethodGet, Path: "/merge-fields", Desc: "Merge fields a template may use"},
	{Name: "create_template", Kind: Write, Method: http.MethodPost, Path: "/templates", Body: true, Idempotent: true,
		Desc: "New template (body: name, channel email|sms, subject, preview_text, body_html, body_text, status draft)"},
	{Name: "update_template", Kind: Write, Method: http.MethodPut, Path: "/templates/{id}/studio", Body: true,
		Desc: "Edit a template; HTML is sanitised and analysed (id; body: name, subject, preview_text, body_html, body_text, status draft|published|archived)"},
	{Name: "generate_template", Kind: Write, Method: http.MethodPost, Path: "/templates/ai", Body: true, Idempotent: true,
		Desc: "OpsAPI writes an email design from a brief (body: brief, tone, save true to keep it as a draft template)"},
	{Name: "publish_template", Kind: Write, Method: http.MethodPost, Path: "/templates/{id}/publish", Desc: "Publish a draft template (id)"},
	{Name: "clone_template", Kind: Write, Method: http.MethodPost, Path: "/templates/{id}/clone", Body: true, Idempotent: true,
		Desc: "Copy a template (id; body: name)"},
	{Name: "seed_templates", Kind: Write, Method: http.MethodPost, Path: "/templates/seed", Desc: "Add the ready-made starter templates"},

	// --- Lists and segments ---
	{Name: "list_lists", Kind: Read, Method: http.MethodGet, Path: "/lists", Query: listQ, Desc: "Contact lists (query: search, list_type, channel)"},
	{Name: "list_counts", Kind: Read, Method: http.MethodGet, Path: "/list-counts", Desc: "Members per list {list uuid: n}"},
	{Name: "get_list", Kind: Read, Method: http.MethodGet, Path: "/lists/{id}", Desc: "One list, with its segment rule (id)"},
	{Name: "list_members", Kind: Read, Method: http.MethodGet, Path: "/lists/{id}/members", Desc: "Newest 500 members of a list; meta.total is the size (id)"},
	{Name: "segment_fields", Kind: Read, Method: http.MethodGet, Path: "/segments/fields", Desc: "Fields and operators a segment rule may use"},
	{Name: "preview_segment", Kind: Read, Method: http.MethodPost, Path: "/segments/preview", Body: true,
		Desc: "Who a rule matches, without saving (body: rule {and|or:[{field, op, value}]}, limit)"},
	{Name: "create_list", Kind: Write, Method: http.MethodPost, Path: "/lists", Body: true, Idempotent: true,
		Desc: "New list (body: name, description, list_type static|dynamic, channel email|sms)"},
	{Name: "update_list", Kind: Write, Method: http.MethodPut, Path: "/lists/{id}", Body: true, Desc: "Rename or describe a list (id; body: name, description)"},
	{Name: "set_segment", Kind: Write, Method: http.MethodPut, Path: "/lists/{id}/segment", Body: true,
		Desc: "Make a list dynamic with a segment rule (id; body: rule, sync true to refresh members now)"},
	{Name: "sync_list", Kind: Write, Method: http.MethodPost, Path: "/lists/{id}/sync", Desc: "Refresh a dynamic list's members from its rule (id)"},
	{Name: "add_list_member", Kind: Write, Method: http.MethodPost, Path: "/lists/{id}/members", Body: true,
		Desc: "Add a CRM contact to a static list (id; body: contact_uuid)"},
	{Name: "remove_list_member", Kind: Write, Method: http.MethodDelete, Path: "/lists/{id}/members/{contact_uuid}",
		Desc: "Take a contact off a list; the contact itself stays (id, contact_uuid)"},

	// --- Contacts (OpsAPI CRM) ---
	{Name: "list_contacts", Kind: Read, Method: http.MethodGet, Path: crmContacts, Query: []string{"search", "status", "account_id", "page", "per_page"},
		Desc: "CRM contacts (query: search by name or email, status, page, per_page)"},
	{Name: "get_contact", Kind: Read, Method: http.MethodGet, Path: crmContacts + "/{id}", Desc: "One contact (id = contact uuid)"},
	{Name: "create_contact", Kind: Write, Method: http.MethodPost, Path: crmContacts, Body: true, Idempotent: true,
		Desc: "New CRM contact (body: first_name required, last_name, email, phone, mobile, job_title, account_uuid)"},
	{Name: "update_contact", Kind: Write, Method: http.MethodPut, Path: crmContacts + "/{id}", Body: true,
		Desc: "Edit a contact (id; body: any of first_name, last_name, email, phone, mobile, job_title, status)"},
	{Name: "get_consent", Kind: Read, Method: http.MethodGet, Path: "/consent/{contact_uuid}", Desc: "A contact's subscriptions and suppressions (contact_uuid)"},
	{Name: "contact_timeline", Kind: Read, Method: http.MethodGet, Path: "/analytics/contacts/{contact_uuid}", Query: []string{"limit"},
		Desc: "What a contact received, opened and clicked (contact_uuid)"},

	// --- Suppression ---
	{Name: "list_suppressions", Kind: Read, Method: http.MethodGet, Path: "/suppressions", Query: append([]string{"channel", "reason"}, pageQ...),
		Desc: "Addresses that are never sent to: unsubscribes, bounces, blocks"},
	{Name: "block_address", Kind: Write, Method: http.MethodPost, Path: "/suppressions", Body: true,
		Desc: "Never send to an address (body: address, channel email|sms, notes)"},

	// --- Campaigns ---
	{Name: "list_campaigns", Kind: Read, Method: http.MethodGet, Path: "/campaigns", Query: campaignQ, Lean: true,
		Desc: "Campaigns (query: status, channel, search, sort, order, page, per_page)"},
	{Name: "get_campaign", Kind: Read, Method: http.MethodGet, Path: "/campaigns/{id}", Desc: "One campaign, content included (id)"},
	{Name: "campaign_stats", Kind: Read, Method: http.MethodGet, Path: "/campaigns/{id}/stats", Desc: "Deliveries by state: queued, accepted, delivered, bounced, failed, skipped (id)"},
	{Name: "audience_preview", Kind: Read, Method: http.MethodPost, Path: "/campaigns/{id}/audience-preview", Body: true,
		Desc: "Eligible, excluded and suppressed counts for the audience, without attaching it (id; body: list_uuid)"},
	{Name: "check_eligibility", Kind: Read, Method: http.MethodPost, Path: "/campaigns/{id}/eligibility", Body: true,
		Desc: "Whether one contact would receive the campaign, and why not (id; body: contact_uuid)"},
	{Name: "create_campaign", Kind: Write, Method: http.MethodPost, Path: "/campaigns", Body: true, Idempotent: true,
		Desc: "New draft campaign (body: name, channel email, message_type marketing, subject, preview_text, body_html, body_text, from_email, from_name, reply_to, objective, batch_size, batch_interval_minutes, max_deliveries)"},
	{Name: "update_campaign", Kind: Write, Method: http.MethodPut, Path: "/campaigns/{id}", Body: true,
		Desc: "Edit a draft/validated/paused campaign, or attach its audience (id; body: any create field, list_uuid). Content changes send it back to draft."},
	{Name: "set_ab_test", Kind: Write, Method: http.MethodPut, Path: "/campaigns/{id}/ab-config", Body: true,
		Desc: "A/B variants (id; body: variants [{key, subject, body_html}], split)"},
	{Name: "duplicate_campaign", Kind: Write, Method: http.MethodPost, Path: "/campaigns/{id}/duplicate", Body: true, Idempotent: true,
		Desc: "Copy a campaign as a new draft (id; body: name)"},
	{Name: "validate_campaign", Kind: Write, Method: http.MethodPost, Path: "/campaigns/{id}/validate", Body: true,
		Desc: "Check content, audience and compliance; draft → validated or awaiting_approval (id; body: list_uuid)"},
	{Name: "request_approval", Kind: Write, Method: http.MethodPost, Path: "/campaigns/{id}/request-approval", Body: true,
		Desc: "Ask a workspace admin to approve it in OpsAPI (id; body: notes)"},
	{Name: "pause_campaign", Kind: Write, Method: http.MethodPost, Path: "/campaigns/{id}/pause", Desc: "Stop sending or hold a schedule (id)"},

	// --- Results ---
	{Name: "analytics_overview", Kind: Read, Method: http.MethodGet, Path: "/analytics/overview",
		Desc: "Workspace totals: campaigns by state, deliveries, opens (estimated), clicks, unsubscribes, conversions"},
	{Name: "campaign_analytics", Kind: Read, Method: http.MethodGet, Path: "/analytics/campaigns/{id}",
		Desc: "One campaign's rates, funnel, A/B winner, top links, replies (id)"},
	{Name: "attribution", Kind: Read, Method: http.MethodGet, Path: "/analytics/attribution", Query: []string{"days"},
		Desc: "Per-campaign performance and attributed revenue (query: days)"},
	{Name: "list_replies", Kind: Read, Method: http.MethodGet, Path: "/replies", Query: append([]string{"classification", "campaign_id"}, pageQ...),
		Desc: "Inbound replies with their classification"},

	// --- Sends and other actions that need the user's go-ahead ---
	{Name: "test_send", Kind: Send, Method: http.MethodPost, Path: "/campaigns/{id}/test-send", Body: true, Delivers: true,
		Desc: "Send one test of the campaign (id; body: email, or contact_uuid)"},
	{Name: "schedule_campaign", Kind: Send, Method: http.MethodPost, Path: "/campaigns/{id}/schedule", Body: true, Delivers: true,
		Desc: "Send the campaign to its audience — now, or at scheduled_at (id; body: scheduled_at RFC3339, send_now, list_uuid, max_deliveries). Validates first; refused while approval is outstanding."},
	{Name: "resume_campaign", Kind: Send, Method: http.MethodPost, Path: "/campaigns/{id}/resume", Delivers: true,
		Desc: "Resume a paused campaign: the rest of its audience is sent to (id)"},
	{Name: "cancel_campaign", Kind: Send, Method: http.MethodPost, Path: "/campaigns/{id}/cancel",
		Desc: "Cancel for good; queued deliveries are skipped (id). Duplicate it to send again."},
	{Name: "import_contacts", Kind: Send, Method: http.MethodPost, Path: "/lists/{id}/import", Body: true,
		Desc: "Bulk import a CSV into a static list, creating CRM contacts (id; body: csv with header email,first_name,last_name,phone — max 2000 rows; grant_consent; legal_basis consent|legitimate_interest)"},
	{Name: "opt_in", Kind: Send, Method: http.MethodPost, Path: "/consent/opt-in", Body: true,
		Desc: "Record that a contact consented (body: contact_uuid, channel, legal_basis, evidence) — a legal record"},
	{Name: "opt_out", Kind: Send, Method: http.MethodPost, Path: "/consent/opt-out", Body: true,
		Desc: "Unsubscribe a contact; only they can undo it (body: contact_uuid, channel, reason)"},
	{Name: "unblock_address", Kind: Send, Method: http.MethodDelete, Path: "/suppressions/{id}",
		Desc: "Remove a manual block so the address can be sent to again (id = suppression uuid)"},
}

var opsByName = func() map[string]Op {
	m := make(map[string]Op, len(ops))
	for _, o := range ops {
		if _, dup := m[o.Name]; dup {
			panic("marketingmanager: duplicate op " + o.Name)
		}
		m[o.Name] = o
	}
	return m
}()

// Lookup finds an operation by name.
func Lookup(name string) (Op, bool) {
	o, ok := opsByName[strings.TrimSpace(strings.ToLower(name))]
	return o, ok
}

// Actions lists the op names of one kind, sorted — the tool's action enum.
func Actions(k Kind) []string {
	var out []string
	for _, o := range ops {
		if o.Kind == k {
			out = append(out, o.Name)
		}
	}
	sort.Strings(out)
	return out
}

// Catalogue is a one-line-per-action description of a kind, for the tool
// description the model reads.
func Catalogue(k Kind) string {
	var b strings.Builder
	for _, o := range ops {
		if o.Kind == k {
			fmt.Fprintf(&b, "- %s: %s\n", o.Name, o.Desc)
		}
	}
	return b.String()
}

// fullPath is the absolute OpsAPI path for o.
func (o Op) fullPath() string {
	if strings.HasPrefix(o.Path, "/api/v2/") {
		return o.Path
	}
	return marketingPrefix + o.Path
}

// placeholders lists the {name} segments o's path needs.
func (o Op) placeholders() []string {
	var out []string
	for _, seg := range strings.Split(o.Path, "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			out = append(out, strings.Trim(seg, "{}"))
		}
	}
	return out
}
