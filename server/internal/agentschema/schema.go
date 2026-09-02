// Package agentschema is the launch-field contract for every specialist
// (Task Manager forms, chat interview, GET /api/v1/agent-schemas).
//
// All agents are wired this way: schema lives with the agent and is registered
// here (or on the forthcoming module registry). Do not add a new ForBuiltin
// case plus a duplicate TypeScript SCHEMAS entry for agent N+1 — register the
// agent. See .claude/rules/agent-modules.md.
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
				{Key: "dry_run", Label: "Preview only", Default: "true"},
			},
		}
	case model.BuiltinMail:
		return Schema{
			Builtin:        model.BuiltinMail,
			SpecialistTool: "mail_sync",
			Fields: []Field{
				{Key: "senders", Label: "Watch senders", Question: "Any sender addresses to watch? Leave blank for all unread mail."},
				{Key: "subject_prefixes", Label: "Subject prefixes"},
				{Key: "labels", Label: "Gmail labels"},
				{Key: "knowledge_notes", Label: "What the agent should know", Question: "What should replies be based on? Prices, products, policies — write it here."},
				{Key: "knowledge_urls", Label: "Knowledge links", Question: "Any pricing or product pages I should read on top of that? One URL per line."},
				{Key: "research_focus", Label: "What to look for"},
				{Key: "reply_instructions", Label: "How the reply should read"},
			},
		}
	case model.BuiltinImages:
		return Schema{
			Builtin:        model.BuiltinImages,
			SpecialistTool: "image_generate",
			Fields: []Field{
				{Key: "prompt", Label: "Image prompt", Question: "What should I generate?", Required: true, MinLength: 3},
			},
		}
	case model.BuiltinCareerOps:
		return Schema{
			Builtin:        model.BuiltinCareerOps,
			SpecialistTool: "career_evaluate",
			Fields: []Field{
				{Key: "job_url", Label: "Job URL", Question: "Paste a job URL, or the job description text."},
				{Key: "jd_text", Label: "Job description"},
				{Key: "mode", Label: "Mode", Default: "full", Options: []model.ClarifyOption{
					{Label: "Full evaluation", Value: "full"},
					{Label: "Triage (fast)", Value: "triage"},
				}},
				{Key: "tailor_cv", Label: "Also tailor CV", Default: "false"},
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
