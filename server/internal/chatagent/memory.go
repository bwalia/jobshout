package chatagent

import (
	"fmt"
	"strings"
	"time"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
)

const (
	windowTokenBudget  = 6000
	minTurns           = 4
	charsPerToken      = 4
	entityIdle         = 24 * time.Hour
	confirmTTL         = 10 * time.Minute
	maxHistoryLoad     = 80
	toolResultMaxChars = 4000
)

// MaxHistoryLoad is how many transcript rows ChatService fetches before windowing.
const MaxHistoryLoad = maxHistoryLoad

func estimateTokens(s string) int {
	n := (len(s) + charsPerToken - 1) / charsPerToken
	if n < 1 {
		return 1
	}
	return n
}

// Window keeps the newest messages that fit the token budget, never fewer
// than minTurns (unless the transcript is shorter).
func Window(history []model.ChatMessage, budget int) (kept []model.ChatMessage, evicted []model.ChatMessage) {
	if budget <= 0 {
		budget = windowTokenBudget
	}
	if len(history) <= minTurns {
		return dropLeadingOrphanTools(history), nil
	}
	used := 0
	cut := 0
	for i := len(history) - 1; i >= 0; i-- {
		t := estimateTokens(history[i].Content)
		if used+t > budget && (len(history)-i-1) >= minTurns {
			cut = i + 1
			break
		}
		used += t
		cut = i
	}
	kept = dropLeadingOrphanTools(history[cut:])
	return kept, history[:cut]
}

func dropLeadingOrphanTools(kept []model.ChatMessage) []model.ChatMessage {
	for len(kept) > 0 && kept[0].Role == model.ChatRoleTool {
		kept = kept[1:]
	}
	return kept
}

func toLLMHistory(msgs []model.ChatMessage) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		role := llm.RoleUser
		switch m.Role {
		case model.ChatRoleAgent:
			role = llm.RoleAssistant
		case model.ChatRoleSystem:
			role = llm.RoleSystem
		case model.ChatRoleTool:
			role = llm.RoleTool
		}
		msg := llm.Message{Role: role, Content: m.Content}
		if calls := toolCallsFromMeta(m.Metadata); len(calls) > 0 {
			msg.ToolCalls = calls
		}
		if id := toolCallIDFromMeta(m.Metadata); id != "" {
			msg.ToolCallID = id
		}
		out = append(out, msg)
	}
	return out
}

func toolCallsFromMeta(meta map[string]any) []llm.ToolCall {
	if meta == nil {
		return nil
	}
	raw, ok := meta["tool_calls"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []llm.ToolCall:
		return v
	case []any:
		out := make([]llm.ToolCall, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			tc := llm.ToolCall{
				ID:        asString(m["id"]),
				Name:      asString(m["name"]),
				Arguments: map[string]any{},
			}
			if args, ok := m["arguments"].(map[string]any); ok {
				tc.Arguments = args
			}
			out = append(out, tc)
		}
		return out
	}
	return nil
}

func toolCallIDFromMeta(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	return asString(meta["tool_call_id"])
}

func toolCallsMeta(calls []llm.ToolCall) []any {
	out := make([]any, 0, len(calls))
	for _, tc := range calls {
		out = append(out, map[string]any{
			"id":        tc.ID,
			"name":      tc.Name,
			"arguments": stripSecretArgs(tc.Arguments),
		})
	}
	return out
}

func readSummary(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	s, _ := meta[model.ChatMetaSummary].(string)
	return s
}

func readEntities(meta map[string]any) map[string]model.SessionEntity {
	out := map[string]model.SessionEntity{}
	if meta == nil {
		return out
	}
	raw, ok := meta[model.ChatMetaEntities].(map[string]any)
	if !ok {
		return out
	}
	now := time.Now()
	for k, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		ent := model.SessionEntity{
			ID:    asString(m["id"]),
			Label: asString(m["label"]),
			Kind:  asString(m["kind"]),
			Href:  asString(m["href"]),
		}
		if at, err := time.Parse(time.RFC3339, asString(m["at"])); err == nil {
			ent.At = at
		} else {
			ent.At = now
		}
		if now.Sub(ent.At) > entityIdle {
			continue
		}
		out[k] = ent
	}
	return out
}

func writeEntities(meta map[string]any, ents map[string]model.SessionEntity) {
	raw := map[string]any{}
	for k, e := range ents {
		raw[k] = map[string]any{
			"id": e.ID, "label": e.Label, "kind": e.Kind, "href": e.Href,
			"at": e.At.Format(time.RFC3339),
		}
	}
	meta[model.ChatMetaEntities] = raw
}

func upsertEntities(ents map[string]model.SessionEntity, refs []model.EntityRef) {
	now := time.Now()
	for _, r := range refs {
		ents["last_"+r.Kind] = model.SessionEntity{
			ID: r.ID, Label: r.Label, Kind: r.Kind, Href: r.Href, At: now,
		}
	}
}

func readPending(meta map[string]any) *model.PendingAction {
	if meta == nil {
		return nil
	}
	raw, ok := meta[model.ChatMetaPending].(map[string]any)
	if !ok {
		return nil
	}
	p := &model.PendingAction{
		Tool:     asString(raw["tool"]),
		Question: asString(raw["question"]),
		Args:     map[string]any{},
	}
	if args, ok := raw["args"].(map[string]any); ok {
		p.Args = args
	}
	if miss, ok := raw["missing"].([]any); ok {
		for _, m := range miss {
			p.Missing = append(p.Missing, asString(m))
		}
	}
	if p.Tool == "" {
		return nil
	}
	return p
}

func writePending(meta map[string]any, p *model.PendingAction) {
	if p == nil {
		delete(meta, model.ChatMetaPending)
		return
	}
	miss := make([]any, 0, len(p.Missing))
	for _, m := range p.Missing {
		miss = append(miss, m)
	}
	asked := p.AskedAt
	if asked.IsZero() {
		asked = time.Now()
	}
	meta[model.ChatMetaPending] = map[string]any{
		"tool": p.Tool, "args": p.Args, "missing": miss,
		"asked_at": asked.Format(time.RFC3339), "question": p.Question,
	}
}

func readConfirm(meta map[string]any) *model.PendingConfirmation {
	if meta == nil {
		return nil
	}
	raw, ok := meta[model.ChatMetaConfirmation].(map[string]any)
	if !ok {
		return nil
	}
	exp, _ := time.Parse(time.RFC3339, asString(raw["expires_at"]))
	if !exp.IsZero() && time.Now().After(exp) {
		return nil
	}
	args, _ := raw["args"].(map[string]any)
	return &model.PendingConfirmation{
		Token:     asString(raw["token"]),
		Tool:      asString(raw["tool"]),
		Args:      args,
		Summary:   asString(raw["summary"]),
		Effect:    asString(raw["effect"]),
		ExpiresAt: exp,
	}
}

func writeConfirm(meta map[string]any, c *model.PendingConfirmation) {
	if c == nil {
		delete(meta, model.ChatMetaConfirmation)
		return
	}
	meta[model.ChatMetaConfirmation] = map[string]any{
		"token": c.Token, "tool": c.Tool, "args": c.Args,
		"summary": c.Summary, "effect": c.Effect,
		"expires_at": c.ExpiresAt.Format(time.RFC3339),
	}
}

func readDisclosed(meta map[string]any) []string {
	if meta == nil {
		return nil
	}
	raw, ok := meta["disclosed_tools"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, v := range raw {
		if s := asString(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func addDisclosed(meta map[string]any, names []string) {
	set := map[string]bool{}
	for _, n := range readDisclosed(meta) {
		set[n] = true
	}
	for _, n := range names {
		if n != "" {
			set[n] = true
		}
	}
	out := make([]any, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	meta["disclosed_tools"] = out
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
