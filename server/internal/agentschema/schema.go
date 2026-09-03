// Package agentschema is the launch-field contract for every specialist
// (Task Manager forms, chat interview, GET /api/v1/agent-schemas).
//
// All agents are wired this way: schema lives on the module and is registered
// with agentmodule. Do not add a ForBuiltin case or a TypeScript SCHEMAS entry
// for agent N+1 — register the agent. See .claude/rules/agent-modules.md.
package agentschema

import (
	"fmt"
	"strings"
	"sync"

	"github.com/jobshout/server/internal/model"
)

// Field is one interview / form slot. Key must match Task Manager / tool JSON names.
type Field struct {
	Key         string
	Label       string
	Question    string
	Type        string // text, textarea, tags, number, select, checkbox, repo
	Required    bool
	MinLength   int
	Min         int
	Default     string
	Placeholder string
	Help        string
	Group       string
	Options     []model.ClarifyOption
}

// TitleRule builds a board-task title from filled values. First match wins.
type TitleRule struct {
	IfKey    string   `json:"if_key,omitempty"`
	Prefix   string   `json:"prefix,omitempty"`
	FromKey  string   `json:"from_key,omitempty"`
	FromKeys []string `json:"from_keys,omitempty"`
	Format   string   `json:"format,omitempty"` // {key} substitution
	Literal  string   `json:"literal,omitempty"`
	Truncate int      `json:"truncate,omitempty"`
	Fallback string   `json:"fallback,omitempty"`
	SuffixIf string   `json:"suffix_if,omitempty"`
	Suffix   string   `json:"suffix,omitempty"`
}

// DescRule appends a description line when the key is non-empty.
type DescRule struct {
	Prefix   string `json:"prefix,omitempty"`
	Key      string `json:"key,omitempty"`
	Truncate int    `json:"truncate,omitempty"`
	Literal  string `json:"literal,omitempty"`
	Format   string `json:"format,omitempty"`
	SuffixIf string `json:"suffix_if,omitempty"`
	Suffix   string `json:"suffix,omitempty"`
}

// RequireGroup is an OR of keys: at least one must be filled.
type RequireGroup struct {
	Keys     []string `json:"keys"`
	Slot     string   `json:"slot,omitempty"`
	Question string   `json:"question,omitempty"`
}

// Schema is the ordered interview for one builtin (or the generic fallback).
type Schema struct {
	Builtin        string
	SpecialistTool string
	Hint           string
	Fields         []Field
	TitleRules     []TitleRule
	DescRules      []DescRule
	RequireAny     []RequireGroup
	Prefill        string // "mailbox"
}

var (
	regMu   sync.RWMutex
	schemas = map[string]Schema{}
	order   []string
)

// SetRegistry is called by agentmodule.Register. All specialists are wired this
// way: schema lives on the module. A new agent does not need significant
// platform changes — register it, do not add a switch here.
func SetRegistry(lookup map[string]Schema, builtins []string) {
	regMu.Lock()
	defer regMu.Unlock()
	schemas = lookup
	order = builtins
}

// Builtins is every registered specialist, in register order.
func Builtins() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, len(order))
	copy(out, order)
	return out
}

// Generic is the chat interview for a custom (non-builtin) agent.
func Generic() Schema {
	return Schema{
		Hint: "Describe the work. Title and description become the agent's prompt when you run.",
		Fields: []Field{
			{Key: "prompt", Label: "Prompt", Question: "What should the agent do?", Type: "textarea", Required: true, MinLength: 3},
		},
	}
}

// ForBuiltin returns the interview schema for a platform builtin marker.
// Unknown / empty builtin uses the generic prompt contract.
func ForBuiltin(builtin string) Schema {
	if builtin == "" {
		return Generic()
	}
	regMu.RLock()
	s, ok := schemas[builtin]
	regMu.RUnlock()
	if ok {
		return s
	}
	return Generic()
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
	return s.RequireAnyMissing(vals)
}

// RequireAnyMissing reports a group where every key is empty.
func (s Schema) RequireAnyMissing(vals map[string]string) (slot, question string, options []model.ClarifyOption) {
	for _, g := range s.RequireAny {
		anyFilled := false
		for _, k := range g.Keys {
			if strings.TrimSpace(vals[k]) != "" {
				anyFilled = true
				break
			}
		}
		if !anyFilled {
			slot = g.Slot
			if slot == "" && len(g.Keys) > 0 {
				slot = g.Keys[0]
			}
			q := g.Question
			if q == "" {
				q = "I need more detail to continue."
			}
			return slot, q, nil
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

// LooksLikeURL reports an http(s) string. Used by AbsorbPrompt on modules.
func LooksLikeURL(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
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
