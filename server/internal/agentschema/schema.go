// Package agentschema is the server copy of Task Manager's agent input
// contract (web/nextjs/lib/agents/input-schemas.ts). Keep the required field
// keys and order in sync with that file.
package agentschema

import (
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/model"
)

// Field is one interview slot. Key must match Task Manager / tool JSON names.
type Field struct {
	Key       string
	Label     string
	Question  string
	Required  bool
	MinLength int
	Default   string
	Options   []model.ClarifyOption
}

// Schema is the ordered interview for one builtin (or the generic fallback).
type Schema struct {
	Builtin        string
	SpecialistTool string // platform tool to dispatch into; empty = generic execute
	Fields         []Field
}

// ForBuiltin returns the interview schema for a platform builtin marker.
// Unknown / empty builtin uses the generic prompt contract.
func ForBuiltin(builtin string) Schema {
	switch builtin {
	case model.BuiltinArticleWriter:
		return Schema{
			Builtin:        model.BuiltinArticleWriter,
			SpecialistTool: "article_generate",
			Fields: []Field{
				{Key: "topic", Label: "Topic", Question: "What should I write about?", Required: true, MinLength: 3},
				{Key: "context", Label: "Context"},
				{Key: "model", Label: "Model"},
			},
		}
	case model.BuiltinResearcher:
		return Schema{
			Builtin:        model.BuiltinResearcher,
			SpecialistTool: "research_run",
			Fields: []Field{
				{Key: "topic", Label: "Topic", Question: "What should I research?", Required: true, MinLength: 3},
				{Key: "context", Label: "Context"},
			},
		}
	case model.BuiltinPentester:
		return Schema{
			Builtin:        model.BuiltinPentester,
			SpecialistTool: "pentest_start",
			Fields: []Field{
				{Key: "target", Label: "Target", Question: "What URL or path should I test?", Required: true, MinLength: 3},
				// Required with a default, matching the TypeScript copy. The
				// two are equivalent in behaviour — NextMissing skips a field
				// that has a default — but stating it the same way on both
				// sides keeps the contract single-meaning.
				{Key: "scan_mode", Label: "Scan mode", Required: true, Default: "quick", Options: []model.ClarifyOption{
					{Label: "Quick (5–15 min)", Value: "quick"},
					{Label: "Standard (30–60 min)", Value: "standard"},
					{Label: "Deep (1–2+ hours)", Value: "deep"},
				}},
				{Key: "max_budget", Label: "Max budget"},
				{Key: "instruction", Label: "Instruction"},
			},
		}
	case model.BuiltinPRReviewer:
		return Schema{
			Builtin:        model.BuiltinPRReviewer,
			SpecialistTool: "review_pull_request",
			Fields: []Field{
				{Key: "repo", Label: "Repository", Question: "Which GitHub repo should I review? Use owner/name.", Required: true, MinLength: 3},
				{Key: "pr_number", Label: "Pull request number", Question: "Which pull request number?", Required: true},
				// KNOWN DIVERGENCE, deliberately left alone: this defaults to
				// preview-only, while input-schemas.ts defaults the same field
				// to false. So the same agent posts real comments to a public
				// pull request when launched from the Task Manager and posts
				// nothing when launched from chat.
				//
				// It is not obvious which is right. Migration 031
				// (pr_reviewer_post_by_default) reseeded the agent's system
				// prompt to say it "posts the review to the PR by default",
				// which sides with the Task Manager; review.go's tool defaults
				// dry := true, which sides with chat. Reconciling them changes
				// whether an agent writes in public, so it is a product call
				// rather than a tidy-up, and the parity test documents the
				// divergence instead of guessing at it.
				{Key: "dry_run", Label: "Preview only", Default: "true"},
			},
		}
	case model.BuiltinMail:
		// The Mail Agent's playbook: which mail to watch, which pages to
		// research it from, and how the reply should read. None of it is
		// required — an empty playbook means "unread inbox, open web when the
		// classifier asks for it", which is a reasonable default.
		//
		// This used to be an empty field list, which is why the agent was
		// usable from the Task Manager and nearly unusable from chat: the two
		// copies of this contract had drifted, and only one of them knew the
		// agent had a playbook at all.
		return Schema{
			Builtin:        model.BuiltinMail,
			SpecialistTool: "mail_sync",
			Fields: []Field{
				{Key: "senders", Label: "Watch senders",
					Question: "Which senders should I watch? Leave empty for all unread mail."},
				{Key: "subject_prefixes", Label: "Subject prefixes"},
				{Key: "labels", Label: "Gmail labels"},
				{Key: "knowledge_urls", Label: "Knowledge links",
					Question: "Which pages should I answer from? One http(s) URL per line."},
				{Key: "research_focus", Label: "What to look for in those pages"},
				{Key: "reply_instructions", Label: "How the reply should read"},
			},
		}
	default:
		return Schema{
			Fields: []Field{
				{Key: "prompt", Label: "Prompt", Question: "What should the agent do?", Required: true, MinLength: 3},
			},
		}
	}
}

// BuiltinOf reads metadata.builtin off an agent.
func BuiltinOf(a *model.Agent) string {
	if a == nil || a.Metadata == nil {
		return ""
	}
	s, _ := a.Metadata[model.MetadataKeyBuiltin].(string)
	return s
}

// ValuesFromArgs copies stringish tool args into a flat map.
func ValuesFromArgs(input map[string]any) map[string]string {
	out := map[string]string{}
	if input == nil {
		return out
	}
	for k, v := range input {
		if v == nil {
			continue
		}
		out[k] = strings.TrimSpace(fmt.Sprint(v))
	}
	return out
}

// NextMissing returns the first required slot that is still empty.
// Closed sets include Options (chips); free-text slots do not.
func (s Schema) NextMissing(vals map[string]string) (slot, question string, options []model.ClarifyOption) {
	for _, f := range s.Fields {
		if !f.Required {
			continue
		}
		raw := strings.TrimSpace(vals[f.Key])
		if raw == "" && f.Default != "" {
			continue
		}
		if raw == "" || (f.MinLength > 0 && len(raw) < f.MinLength) {
			q := f.Question
			if q == "" {
				q = "I need " + strings.ToLower(f.Label) + " to continue."
			}
			return f.Key, q, f.Options
		}
	}
	return "", "", nil
}

// Pick narrows a value map to the slots this schema declares.
//
// Tool calls arrive carrying plumbing the agent has no use for — the agent's
// own name, a "reason" the model volunteered — and anything left in would be
// recorded as though the user had answered it. Narrowing here is what makes a
// run started from chat and one started from a form record identical inputs.
func (s Schema) Pick(vals map[string]string) map[string]string {
	out := make(map[string]string, len(s.Fields))
	for _, f := range s.Fields {
		if v, ok := vals[f.Key]; ok && strings.TrimSpace(v) != "" {
			out[f.Key] = v
		}
	}
	return out
}

// ApplyDefaults fills empty optional/defaulted fields.
func (s Schema) ApplyDefaults(vals map[string]string) map[string]string {
	if vals == nil {
		vals = map[string]string{}
	}
	for _, f := range s.Fields {
		if strings.TrimSpace(vals[f.Key]) == "" && f.Default != "" {
			vals[f.Key] = f.Default
		}
	}
	return vals
}

// IsThinPrompt reports a tautology of "run the X agent" with no extra substance.
func IsThinPrompt(prompt, agentName string) bool {
	p := compact(prompt)
	if p == "" {
		return true
	}
	p = stripLead(p, "please ", "can you ", "could you ", "would you ")
	p = stripLead(p, "run ", "start ", "launch ", "execute ", "use ")
	p = strings.TrimPrefix(p, "the ")
	p = strings.TrimSpace(p)
	p = strings.TrimSuffix(p, " agent")
	p = strings.TrimSpace(p)
	if p == "" {
		return true
	}
	name := compact(agentName)
	name = strings.TrimSuffix(name, " agent")
	name = strings.TrimSpace(name)
	return name != "" && p == name
}

func compact(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func stripLead(s string, prefixes ...string) string {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return strings.TrimSpace(s[len(p):])
		}
	}
	return s
}
