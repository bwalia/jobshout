// Package chattools is the Marketing Manager's chat tools. It is separate
// from marketingmanager so agentmodules (which registers the agent) does not
// import platformtools; main attaches it with agentmodule.SetToolInstaller.
package chattools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/marketingmanager"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/platformtools"
	"github.com/jobshout/server/internal/tools"
)

type (
	Service = marketingmanager.Service
	Kind    = marketingmanager.Kind
	Call    = marketingmanager.Call
	Result  = marketingmanager.Result
)

const (
	Read  = marketingmanager.Read
	Write = marketingmanager.Write
	Send  = marketingmanager.Send
)

// Auditor records an action in the org's audit log.
// Satisfied by repository.AuditRepository.
type Auditor interface {
	RecordAction(ctx context.Context, log *model.AuditLog) error
}

const auditTimeout = 5 * time.Second

// Installer adds marketing_read, marketing_write and marketing_send.
// One tool per Kind: the guard confirms every marketing_send call, and Run
// refuses an action of another kind, so a send cannot ride in on read/write.
func Installer(svc *Service) func(reg, deps any) {
	return func(reg, deps any) {
		r, ok := reg.(*platformtools.Registry)
		if !ok || svc == nil {
			return
		}
		var audit Auditor
		if d, ok := deps.(platformtools.Deps); ok && d.Audit != nil {
			audit = d.Audit
		}
		for _, k := range []Kind{Read, Write, Send} {
			r.Register(&tool{svc: svc, kind: k, audit: audit})
		}
	}
}

type tool struct {
	svc   *Service
	kind  Kind
	audit Auditor
}

func (t *tool) Name() string { return "marketing_" + t.kind.String() }

func (t *tool) Description() string {
	var intro string
	switch t.kind {
	case Read:
		intro = "Read the organisation's OpsAPI email marketing (Marketing Cloud + CRM contacts). Changes nothing. Call action=status first: it says whether the workspace is in test mode, and gives the brand kit and confirmed test recipients."
	case Write:
		intro = "Change OpsAPI email marketing records — templates, contacts, lists, segments, draft campaigns. Sends nothing. Creates are idempotent; a contact whose email already exists is returned, not duplicated."
	default:
		intro = "Actions that reach people or cannot be undone: test sends, sending/scheduling a campaign, resuming, cancelling, importing contacts, consent changes. Every call waits for the user's approval. Test sends go only to confirmed test recipients. A workspace not in test mode also needs live_send_approved=true, which you set only when the user has explicitly said to send real email."
	}
	return intro + " Pass website to pick the workspace when the org has several (e.g. tenthmatrix, workstation); never mix workspaces in one task.\nActions:\n" + marketingmanager.Catalogue(t.kind)
}

func (t *tool) Schema() map[string]any {
	props := map[string]any{
		"action":       map[string]any{"type": "string", "enum": marketingmanager.Actions(t.kind), "description": "Which operation."},
		"website":      map[string]any{"type": "string", "description": "Saved OpsAPI website name or namespace slug. Omit when the org has one marketing workspace."},
		"id":           map[string]any{"type": "string", "description": "The record's uuid (campaign, template, list, contact, suppression)."},
		"contact_uuid": map[string]any{"type": "string", "description": "A CRM contact uuid, for actions that name one."},
		"query":        map[string]any{"type": "object", "description": "Query parameters for list actions (page, per_page, search, status…)."},
		"body":         map[string]any{"type": "object", "description": "Request body, as described for the action."},
	}
	if t.kind == Send {
		props["live_send_approved"] = map[string]any{"type": "boolean", "description": "True only after the user explicitly approved sending real email from a workspace that is not in test mode."}
	}
	return tools.ObjectSchema(props, "action")
}

func (t *tool) Parameters() tools.ParameterSchema { return t.Schema() }
func (t *tool) Domain() string                    { return "marketing" }
func (t *tool) Destructive() bool                 { return t.kind == Send }
func (t *tool) ReadOnly() bool                    { return t.kind == Read }

func (t *tool) Permission() string {
	if t.kind == Read {
		return model.PermAgentsRead
	}
	return model.PermAgentsExecute
}

// DescribeEffect is the approval text: what happens, to how many, live or test.
func (t *tool) DescribeEffect(ctx context.Context, args map[string]any) string {
	call, err := marketingmanager.ParseCall(args)
	if err != nil {
		return ""
	}
	desc := t.svc.Describe(ctx, platformtools.MustIdentity(ctx).OrgID, call)
	if desc != "" && call.LiveSendApproved {
		desc += " You are approving a LIVE send."
	}
	return desc
}

func (t *tool) Run(ctx context.Context, input map[string]any) (*platformtools.Result, error) {
	ident := platformtools.MustIdentity(ctx)
	call, err := marketingmanager.ParseCall(input)
	if err != nil {
		return nil, err
	}
	res, err := t.svc.Run(ctx, ident.OrgID, t.kind, call)
	t.record(ctx, ident, call, res, err)
	if err != nil {
		return nil, err
	}
	out := &platformtools.Result{Data: res}
	if t.kind == Send {
		out.Effect = fmt.Sprintf("%s in %s (%s)", call.Action, res.Namespace, sendMode(res.SendMode))
	}
	return out, nil
}

func (t *tool) Execute(ctx context.Context, input map[string]any) (string, error) {
	res, err := t.Run(ctx, input)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(res.Data)
	return string(raw), err
}

// record writes every write and send — refused ones too — to the audit log.
// Bodies are not stored (they hold message content and personal data); the
// field names are, so the trail shows what was set.
func (t *tool) record(ctx context.Context, ident platformtools.Identity, call Call, res *Result, runErr error) {
	if t.audit == nil || t.kind == Read || ident.OrgID == uuid.Nil {
		return
	}
	meta := map[string]any{
		"tool":    t.Name(),
		"website": call.Website,
		"outcome": "ok",
	}
	if call.ID != "" {
		meta["id"] = call.ID
	}
	if call.ContactUUID != "" {
		meta["contact_uuid"] = call.ContactUUID
	}
	if keys := bodyKeys(call.Body); len(keys) > 0 {
		meta["body_fields"] = keys
	}
	if call.Action == "test_send" {
		meta["to"], _ = call.Body["email"].(string)
	}
	if call.LiveSendApproved {
		meta["live_send_approved"] = true
	}
	if res != nil {
		meta["namespace"] = res.Namespace
		meta["website"] = res.Website
		if res.SendMode != "" {
			meta["send_mode"] = res.SendMode
		}
		if res.Note != "" {
			meta["note"] = res.Note
		}
	}
	if runErr != nil {
		meta["outcome"] = "error"
		meta["error"] = clip(runErr.Error(), 500)
	}
	var uid *uuid.UUID
	if ident.UserID != uuid.Nil {
		u := ident.UserID
		uid = &u
	}
	// Detached: an audit row must not be lost because the chat turn ended.
	actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), auditTimeout)
	defer cancel()
	_ = t.audit.RecordAction(actx, &model.AuditLog{
		OrgID:    ident.OrgID,
		UserID:   uid,
		Action:   "marketing." + strings.ToLower(call.Action),
		Resource: "opsapi_marketing",
		Metadata: meta,
	})
}

func bodyKeys(b map[string]any) []string {
	out := make([]string, 0, len(b))
	for k := range b {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sendMode(m string) string {
	if m == "" {
		return "no delivery"
	}
	return m
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
