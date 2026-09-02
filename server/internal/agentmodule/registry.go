// Package agentmodule is the specialist registry.
//
// All specialists are wired this way: own package, then one Register call.
// A new agent does not need significant platform changes — register it, do not
// add a switch. See .claude/rules/agent-modules.md.
package agentmodule

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Module is one builtin specialist. Schema, seed, launch, chat hint, and the
// Task Manager tab all come from here. Extra chat tools attach at Register time.
type Module struct {
	Builtin  string
	Label    string // Task Manager rail
	Icon     string // lucide icon name (briefcase, mail, …)
	TabSlug  string // URL ?agent= — empty means no rail tab
	Hint     string // form blurb
	ChatHint string

	Schema agentschema.Schema
	Seed   func(orgID uuid.UUID) *model.Agent
	Launch LaunchFunc

	// AbsorbPrompt maps a free-text prompt onto schema fields when the user
	// said "run X" plus substance. Nil means ignore the prompt.
	AbsorbPrompt func(prompt string, vals map[string]string)

	// StayOnTab: after launch, keep the Task Manager on this agent's tab
	// (used when the product UI, not the board card, is the result surface).
	StayOnTab bool

	// PrefillMailbox: Task Manager loads saved Gmail playbook into the form.
	PrefillMailbox bool

	// PromptRoute handles a chat shortcut before Launch (e.g. mail "drafts").
	PromptRoute *PromptRoute

	// InstallTools registers extra chat tools. Signature is untyped so this
	// package does not import platformtools; the chat registry passes (reg, deps).
	InstallTools func(reg, deps any)
}

var (
	toolMu         sync.RWMutex
	toolInstallers = map[string]func(reg, deps any){}
)

// SetToolInstaller attaches extra chat tools for a builtin. Called from the
// tools package init so Register does not import platformtools.
//
// All specialists are wired this way: extra tools live with the agent.
// NewRegistryWithTools iterates the registry — do not add a switch there.
func SetToolInstaller(builtin string, fn func(reg, deps any)) {
	if builtin == "" || fn == nil {
		return
	}
	toolMu.Lock()
	toolInstallers[builtin] = fn
	toolMu.Unlock()
}

// ToolInstaller returns extra chat tools for a builtin, if any were registered
// via SetToolInstaller. NewRegistryWithTools uses this so attach order does
// not depend on package init order.
func ToolInstaller(builtin string) func(reg, deps any) {
	toolMu.RLock()
	defer toolMu.RUnlock()
	return toolInstallers[builtin]
}

// PromptRoute is a data-driven agent_execute shortcut. No per-agent if in execute.go.
type PromptRoute struct {
	IfContains     string
	UnlessContains string
	Tool           string
	OnlyIfNoLaunch bool
}

// LaunchFunc runs after the board task exists. Closures capture the service.
type LaunchFunc func(ctx context.Context, in LaunchInput) (*LaunchOutput, error)

// LaunchInput is what every specialist Launch sees.
type LaunchInput struct {
	OrgID  uuid.UUID
	UserID uuid.UUID
	Agent  *model.Agent
	Task   *model.Task
	Values map[string]string
	Source string
}

// LaunchOutput is applied to the board task and the HTTP/chat result.
type LaunchOutput struct {
	RunID        *uuid.UUID
	SyncQueued   bool
	Brief        any
	ImageURL     string
	EvaluationID *uuid.UUID
	Message      string
	Description  string
	Status       string // done | in_progress | empty = leave in_progress
	ExtraMeta    map[string]any
}

var (
	mu      sync.RWMutex
	modules []Module
	byName  = map[string]int{}
)

// Register adds or replaces a specialist.
//
// All specialists are wired this way: own package, then register. A new agent
// does not need significant platform changes — register it, do not add a switch.
func Register(m Module) {
	if m.InstallTools == nil {
		m.InstallTools = ToolInstaller(m.Builtin)
	}
	if m.Builtin == "" {
		panic("agentmodule: Register missing builtin")
	}
	mu.Lock()
	defer mu.Unlock()
	if i, ok := byName[m.Builtin]; ok {
		modules[i] = m
		syncLookupLocked()
		return
	}
	byName[m.Builtin] = len(modules)
	modules = append(modules, m)
	syncLookupLocked()
}

// Reset clears the registry (tests).
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	modules = nil
	byName = map[string]int{}
	syncLookupLocked()
}

func syncLookupLocked() {
	lookup := make(map[string]agentschema.Schema, len(modules))
	order := make([]string, 0, len(modules))
	for _, m := range modules {
		lookup[m.Builtin] = m.Schema
		order = append(order, m.Builtin)
	}
	agentschema.SetRegistry(lookup, order)
}

// All returns registered modules in register order.
func All() []Module {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Module, len(modules))
	copy(out, modules)
	return out
}

// Lookup finds a module by metadata.builtin.
func Lookup(builtin string) (Module, bool) {
	mu.RLock()
	defer mu.RUnlock()
	i, ok := byName[builtin]
	if !ok {
		return Module{}, false
	}
	return modules[i], true
}

// ChatHints is the specialist section of the chat system prompt.
func ChatHints() string {
	mu.RLock()
	defer mu.RUnlock()
	var b string
	for _, m := range modules {
		if m.ChatHint == "" {
			continue
		}
		b += "- " + m.ChatHint + "\n"
	}
	return b
}
