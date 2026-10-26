package marketingmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Service runs Marketing Manager operations against an org's OpsAPI
// workspace. It holds no tenant state: every call resolves the org's
// workspace afresh, so one org can never reach another's marketing.
type Service struct {
	Integrations IntegrationLister
	Ring         RingConfig
	// AllowLiveSend lets a send go out from a workspace that is NOT in test
	// mode (OpsAPI dry_run off). Off by default (MARKETING_LIVE_SEND): on such
	// a server only dry-run workspaces can send, so development and test rings
	// cannot email real people whatever the chat or the model asks.
	AllowLiveSend bool
	HTTP          *http.Client
}

// Call is one operation request, as the chat tools and Launch build it.
type Call struct {
	Action      string
	Website     string
	ID          string
	ContactUUID string
	Query       map[string]any
	Body        map[string]any
	// LiveSendApproved is the user's explicit "yes, send real email" for a
	// workspace that is not in test mode. The chat confirmation covers these
	// exact args, so the user sees and approves it.
	LiveSendApproved bool
}

// ParseCall reads tool arguments. body/query may arrive as JSON strings.
func ParseCall(in map[string]any) (Call, error) {
	c := Call{
		Action:      str(in["action"]),
		Website:     str(in["website"]),
		ID:          str(in["id"]),
		ContactUUID: str(in["contact_uuid"]),
	}
	var err error
	if c.Body, err = object(in["body"], "body"); err != nil {
		return c, err
	}
	if c.Query, err = object(in["query"], "query"); err != nil {
		return c, err
	}
	switch v := in["live_send_approved"].(type) {
	case bool:
		c.LiveSendApproved = v
	case string:
		c.LiveSendApproved = strings.EqualFold(strings.TrimSpace(v), "true")
	}
	return c, nil
}

// Result is what an operation returns to the chat or a report.
type Result struct {
	Website   string          `json:"website"`
	Namespace string          `json:"namespace"`
	Action    string          `json:"action"`
	Data      json.RawMessage `json:"data,omitempty"`
	Meta      json.RawMessage `json:"meta,omitempty"`
	// SendMode is "dry_run" or "live" for anything that delivers.
	SendMode string `json:"send_mode,omitempty"`
	Note     string `json:"note,omitempty"`
	// On status only: the brand kit to design emails with, and who test
	// sends may go to.
	Brand          map[string]any `json:"brand,omitempty"`
	TestRecipients []string       `json:"test_recipients,omitempty"`
}

// Connect resolves org's workspace for website.
func (s *Service) Connect(ctx context.Context, orgID uuid.UUID, website string) (*Client, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: the Marketing Manager is not set up on this server", ErrNotConfigured)
	}
	conn, err := resolve(ctx, s.Integrations, s.Ring, orgID, website)
	if err != nil {
		return nil, err
	}
	return newClient(withRing(conn, s.Ring), s.HTTP), nil
}

// Run performs call, refusing an action that is not of kind — each chat tool
// carries one kind, so a send can never ride in on the read or write tool and
// skip the confirmation.
func (s *Service) Run(ctx context.Context, orgID uuid.UUID, kind Kind, call Call) (*Result, error) {
	op, ok := Lookup(call.Action)
	if !ok {
		return nil, fmt.Errorf("unknown marketing action %q; use one of: %s", call.Action, strings.Join(Actions(kind), ", "))
	}
	if op.Kind != kind {
		return nil, fmt.Errorf("%s is a %s action: call it with the marketing_%s tool", op.Name, op.Kind, op.Kind)
	}
	c, err := s.Connect(ctx, orgID, call.Website)
	if err != nil {
		return nil, err
	}
	return s.run(ctx, c, orgID, op, call)
}

func (s *Service) run(ctx context.Context, c *Client, orgID uuid.UUID, op Op, call Call) (*Result, error) {
	path, err := fillPath(op, call)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	for _, k := range op.Query {
		if v, ok := call.Query[k]; ok && v != nil {
			q.Set(k, fmt.Sprint(v))
		}
	}
	var body any
	if op.Body {
		b := call.Body
		if b == nil {
			b = map[string]any{}
		}
		body = b
	}

	res := &Result{Website: c.Website(), Namespace: c.Namespace(), Action: op.Name}
	done, err := s.precheck(ctx, c, op, &call, res)
	if err != nil {
		return nil, err
	}
	if done {
		return res, nil
	}
	if op.Delivers {
		mode, err := s.gate(ctx, c, call)
		if err != nil {
			return nil, err
		}
		res.SendMode = mode
		if mode == ModeDryRun {
			res.Note = "Test mode (dry run) is on in this workspace: OpsAPI records the send, but no email or SMS leaves."
		}
	}

	idem := ""
	if op.Idempotent {
		idem = idempotencyKey(orgID, op.Name, path, body)
	}
	resp, err := c.Do(ctx, op.Method, path, q, body, idem)
	if err != nil {
		return nil, err
	}
	res.Data, res.Meta = resp.Data, resp.Meta
	if op.Lean {
		res.Data = lean(res.Data)
	}
	if op.Name == "status" {
		res.Brand, res.TestRecipients = c.conn.Brand, c.conn.TestRecipients
		if len(res.TestRecipients) == 0 {
			res.Note = strings.TrimSpace(res.Note + " No confirmed test recipients: test sends are refused until an admin adds some.")
		}
	}
	res.Data, err = capItems(res.Data, &res.Note)
	if err != nil {
		return nil, err
	}
	return res, nil
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func fillPath(op Op, call Call) (string, error) {
	path := op.fullPath()
	for _, name := range op.placeholders() {
		v := call.ID
		if name == "contact_uuid" {
			v = call.ContactUUID
			if v == "" && !strings.Contains(op.Path, "{id}") {
				v = call.ID // a single-id call may name it either way
			}
		}
		if v == "" {
			return "", fmt.Errorf("%s needs %s", op.Name, name)
		}
		if !idRe.MatchString(v) {
			return "", fmt.Errorf("%s: %q is not a valid %s", op.Name, v, name)
		}
		path = strings.Replace(path, "{"+name+"}", url.PathEscape(v), 1)
	}
	return path, nil
}

// idempotencyKey is stable for one org, operation, path and body: a create
// the model repeats returns the first record instead of making another.
func idempotencyKey(orgID uuid.UUID, action, path string, body any) string {
	h := sha256.New()
	raw, _ := json.Marshal(body)
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00", orgID, action, path)
	h.Write(raw)
	return "jobshout-mm-" + hex.EncodeToString(h.Sum(nil))[:40]
}

// maxItems bounds a list in a tool result; the model pages for more.
const maxItems = 100

// capItems trims an over-long top-level array and says so in note.
func capItems(data json.RawMessage, note *string) (json.RawMessage, error) {
	var arr []json.RawMessage
	if len(data) == 0 || data[0] != '[' || json.Unmarshal(data, &arr) != nil || len(arr) <= maxItems {
		return data, nil
	}
	out, err := json.Marshal(arr[:maxItems])
	if err != nil {
		return data, err
	}
	*note = strings.TrimSpace(*note + fmt.Sprintf(" Showing the first %d of %d; use page/per_page or search for the rest.", maxItems, len(arr)))
	return out, nil
}

// lean drops message bodies from a list of records.
func lean(data json.RawMessage) json.RawMessage {
	var arr []map[string]json.RawMessage
	if json.Unmarshal(data, &arr) != nil {
		return data
	}
	for _, m := range arr {
		delete(m, "body_html")
		delete(m, "body_text")
	}
	out, err := json.Marshal(arr)
	if err != nil {
		return data
	}
	return out
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%f", t), "0"), ".")
	}
	return ""
}

func object(v any, name string) (map[string]any, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return t, nil
	case string:
		if strings.TrimSpace(t) == "" {
			return nil, nil
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(t), &m); err != nil {
			return nil, fmt.Errorf("%s must be a JSON object", name)
		}
		return m, nil
	}
	return nil, fmt.Errorf("%s must be a JSON object", name)
}
