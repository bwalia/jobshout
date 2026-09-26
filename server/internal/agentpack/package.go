// Package agentpack is the versioned JSON contract for agent import/export.
//
// Platform-generic: no per-builtin switch. Specialist extras are declared on
// agentmodule.Module (Requirements / Ready).
package agentpack

import (
	"time"

	"github.com/google/uuid"
)

const (
	Kind             = "jobshout.agent"
	SchemaVersion    = 1
	MinSchemaVersion = 1
	ContentType      = "application/vnd.jobshout.agent+json"

	MaxJSONBytes      = 2 << 20
	MaxKnowledgeFiles = 50
	MaxKnowledgeBytes = 256 << 10
	MaxSystemPrompt   = 32 << 10
)

// GatedTools are skipped on import unless the operator opts in.
var GatedTools = []string{"shell_command"}

// Package is the on-disk / HTTP document.
type Package struct {
	Kind          string    `json:"kind"`
	SchemaVersion int       `json:"schema_version"`
	ExportedAt    time.Time `json:"exported_at"`
	Source        Source    `json:"source"`
	Agent         Body      `json:"agent"`
	Tools         []string  `json:"tools,omitempty"`
	Skills        []Skill   `json:"skills,omitempty"`
	Knowledge     []File    `json:"knowledge,omitempty"`
	Warnings      []string  `json:"warnings,omitempty"`
}

// Source is origin metadata. IDs are informational and must not be reused.
type Source struct {
	AgentID   string   `json:"agent_id,omitempty"`
	Builtin   string   `json:"builtin,omitempty"`
	FieldKeys []string `json:"field_keys,omitempty"`
}

// Body is the portable agent row. No ids, org, credentials, or runtime stats.
type Body struct {
	Name          string         `json:"name"`
	Role          string         `json:"role"`
	Description   string         `json:"description,omitempty"`
	SystemPrompt  string         `json:"system_prompt,omitempty"`
	ModelProvider string         `json:"model_provider,omitempty"`
	ModelName     string         `json:"model_name,omitempty"`
	EngineType    string         `json:"engine_type,omitempty"`
	EngineConfig  map[string]any `json:"engine_config,omitempty"`
	Builtin       string         `json:"builtin,omitempty"`
}

// Skill is an enabled skill referenced by slug. Org-private skills carry their def.
type Skill struct {
	Slug           string         `json:"slug"`
	Origin         string         `json:"origin"` // builtin | org
	Name           string         `json:"name,omitempty"`
	Description    string         `json:"description,omitempty"`
	Kind           string         `json:"kind,omitempty"`
	ConfigJSON     map[string]any `json:"config_json,omitempty"`
	Version        string         `json:"version,omitempty"`
	ConfigOverride map[string]any `json:"config_override,omitempty"`
}

// File is a knowledge file's text. Embeddings are rebuilt on import.
type File struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// Bindings are operator choices from the preview screen.
type Bindings struct {
	Name              string   `json:"name,omitempty"`
	ModelProvider     string   `json:"model_provider,omitempty"`
	ModelName         string   `json:"model_name,omitempty"`
	SkipTools         []string `json:"skip_tools,omitempty"`
	IncludeGatedTools bool     `json:"include_gated_tools,omitempty"`
}

// Mode is how import will land in the destination org.
type Mode string

const (
	ModeCreate  Mode = "create"
	ModeOverlay Mode = "overlay"
)

// Issue is a preview finding.
type Issue struct {
	Severity string `json:"severity"` // error | warning | info
	Code     string `json:"code"`
	Message  string `json:"message"`
}

func (i Issue) IsError() bool { return i.Severity == "error" }

// Report is the preview result.
type Report struct {
	Mode          Mode       `json:"mode"`
	TargetAgentID *uuid.UUID `json:"target_agent_id,omitempty"`
	TargetName    string     `json:"target_name,omitempty"`
	Agent         Body       `json:"agent"`
	Issues        []Issue    `json:"issues"`
	Bindings      Bindings   `json:"bindings"`
	Diff          *Diff      `json:"diff,omitempty"`
	CanUndo       bool       `json:"can_undo"`
}

// Diff summarises an overlay so the UI can show what will change.
type Diff struct {
	PromptChanged bool     `json:"prompt_changed"`
	ModelChanged  bool     `json:"model_changed"`
	ToolsAdded    []string `json:"tools_added,omitempty"`
	ToolsRemoved  []string `json:"tools_removed,omitempty"`
	KnowledgeN    int      `json:"knowledge_files"`
	SkillsN       int      `json:"skills"`
}

// HasError reports a blocking issue.
func (r Report) HasError() bool {
	for _, i := range r.Issues {
		if i.IsError() {
			return true
		}
	}
	return false
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
