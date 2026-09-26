package agentpack

import (
	"strings"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/model"
)

// Dest is the destination org as the preview engine sees it.
type Dest struct {
	NameTaken       bool
	ExistingBuiltin *model.Agent
	ModuleOK        bool
	ToolNames       map[string]bool
	SkillSlugs      map[string]bool
	ModelOK         bool
	Ready           []agentmodule.Issue
	DestFieldKeys   []string
	ExistingTools   []string
}

// Evaluate builds a preview report. It does not write to the database.
func Evaluate(pkg *Package, dest Dest) Report {
	SanitizePackage(pkg)
	rep := Report{
		Mode:    ModeCreate,
		Agent:   pkg.Agent,
		CanUndo: true,
		Bindings: Bindings{
			Name:          DefaultName(pkg, dest.NameTaken),
			ModelProvider: pkg.Agent.ModelProvider,
			ModelName:     pkg.Agent.ModelName,
		},
	}

	if err := ValidateKind(pkg); err != nil {
		rep.Issues = append(rep.Issues, Issue{Severity: "error", Code: "invalid_package", Message: err.Error()})
		return rep
	}
	if err := CheckSize(pkg); err != nil {
		rep.Issues = append(rep.Issues, Issue{Severity: "error", Code: "too_large", Message: err.Error()})
		return rep
	}
	if keys := remainingSecretKeys(pkg.Agent.EngineConfig); len(keys) > 0 {
		rep.Issues = append(rep.Issues, Issue{
			Severity: "error", Code: "secret_in_package",
			Message: "package still contains credential-shaped keys after sanitizing: " + strings.Join(keys, ", "),
		})
		return rep
	}

	builtin := strings.TrimSpace(pkg.Agent.Builtin)
	if builtin != "" {
		if !dest.ModuleOK {
			rep.Issues = append(rep.Issues, Issue{
				Severity: "error", Code: "specialist_missing",
				Message: "this JobShout does not have the " + builtin + " specialist. Upgrade the destination instead of importing as a generic agent.",
			})
			return rep
		}
		if dest.ExistingBuiltin == nil {
			rep.Issues = append(rep.Issues, Issue{
				Severity: "error", Code: "builtin_not_seeded",
				Message: "this organisation has no seeded " + builtin + " agent to update.",
			})
			return rep
		}
		rep.Mode = ModeOverlay
		id := dest.ExistingBuiltin.ID
		rep.TargetAgentID = &id
		rep.TargetName = dest.ExistingBuiltin.Name
		rep.CanUndo = false
		rep.Bindings.Name = dest.ExistingBuiltin.Name
		rep.Diff = overlayDiff(pkg, dest.ExistingBuiltin, dest.ExistingTools)
		rep.Issues = append(rep.Issues, Issue{
			Severity: "info", Code: "overlay",
			Message: "This organisation already has " + dest.ExistingBuiltin.Name + ". Import will update its prompt, model, tools, skills, and knowledge. This cannot be undone from the import dialog.",
		})
	} else if dest.NameTaken {
		rep.Issues = append(rep.Issues, Issue{
			Severity: "info", Code: "name_conflict",
			Message: "An agent named " + pkg.Agent.Name + " already exists. It will be imported as " + rep.Bindings.Name + ".",
		})
	}

	if !dest.ModelOK && (pkg.Agent.ModelProvider != "" || pkg.Agent.ModelName != "") {
		rep.Issues = append(rep.Issues, Issue{
			Severity: "warning", Code: "model_unavailable",
			Message: "The packaged model is not available here. Pick another model, or leave the destination default.",
		})
		rep.Bindings.ModelProvider = ""
		rep.Bindings.ModelName = ""
	}

	if len(pkg.Tools) == 0 {
		rep.Issues = append(rep.Issues, Issue{
			Severity: "warning", Code: "empty_tools",
			Message: "This agent has no tools allowed. It will run without tools unless you add some after import.",
		})
	}

	var skip []string
	for _, t := range pkg.Tools {
		if dest.ToolNames != nil && !dest.ToolNames[t] {
			rep.Issues = append(rep.Issues, Issue{
				Severity: "warning", Code: "unknown_tool",
				Message: "Tool " + t + " is not registered here and will be skipped.",
			})
			skip = append(skip, t)
		}
		if isGated(t) {
			rep.Issues = append(rep.Issues, Issue{
				Severity: "warning", Code: "gated_tool",
				Message: "Gated tool " + t + " will be skipped unless you opt in.",
			})
			skip = append(skip, t)
		}
	}
	rep.Bindings.SkipTools = unique(skip)

	for _, s := range pkg.Skills {
		if s.Origin == "org" {
			continue
		}
		if dest.SkillSlugs != nil && !dest.SkillSlugs[s.Slug] {
			rep.Issues = append(rep.Issues, Issue{
				Severity: "warning", Code: "missing_skill",
				Message: "Built-in skill " + s.Slug + " is not on this JobShout and will be skipped.",
			})
		}
	}

	if builtin != "" && len(pkg.Source.FieldKeys) > 0 && len(dest.DestFieldKeys) > 0 {
		if !sameKeys(pkg.Source.FieldKeys, dest.DestFieldKeys) {
			rep.Issues = append(rep.Issues, Issue{
				Severity: "warning", Code: "schema_drift",
				Message: "This " + builtin + " agent was exported from a JobShout whose form fields differ. Review the specialist tab after import.",
			})
		}
	}

	for _, iss := range dest.Ready {
		sev := iss.Severity
		if sev == "" {
			sev = "warning"
		}
		rep.Issues = append(rep.Issues, Issue{Severity: sev, Code: iss.Code, Message: iss.Message})
	}

	// Static Requirements are for export warnings and for specialists that
	// have no live Ready hook. When Ready exists, dest.Ready is the import view.
	if m, ok := agentmodule.Lookup(builtin); ok && m.Ready == nil {
		for _, req := range m.Requirements {
			sev := "warning"
			if req.Blocking {
				sev = "error"
			}
			rep.Issues = append(rep.Issues, Issue{Severity: sev, Code: req.Key, Message: req.Message})
		}
	}

	if len(pkg.Knowledge) > 0 {
		rep.Issues = append(rep.Issues, Issue{
			Severity: "info", Code: "knowledge_text",
			Message: "This package includes knowledge file text, which may contain personal or confidential content. Embeddings will be rebuilt on this JobShout.",
		})
	}

	return rep
}

func overlayDiff(pkg *Package, existing *model.Agent, existingTools []string) *Diff {
	d := &Diff{
		PromptChanged: derefStr(existing.SystemPrompt) != pkg.Agent.SystemPrompt,
		ModelChanged:  derefStr(existing.ModelProvider) != pkg.Agent.ModelProvider || derefStr(existing.ModelName) != pkg.Agent.ModelName,
		KnowledgeN:    len(pkg.Knowledge),
		SkillsN:       len(pkg.Skills),
	}
	have := map[string]bool{}
	for _, t := range existingTools {
		have[t] = true
	}
	want := map[string]bool{}
	for _, t := range pkg.Tools {
		want[t] = true
		if !have[t] {
			d.ToolsAdded = append(d.ToolsAdded, t)
		}
	}
	for _, t := range existingTools {
		if !want[t] {
			d.ToolsRemoved = append(d.ToolsRemoved, t)
		}
	}
	return d
}

func unique(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func sameKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	mb := map[string]bool{}
	for _, k := range b {
		mb[k] = true
	}
	for _, k := range a {
		if !mb[k] {
			return false
		}
	}
	return true
}

// EffectiveTools applies skip + gated defaults to the packaged tool list,
// then drops names the destination registry does not have.
func EffectiveTools(pkg *Package, bindings Bindings, known map[string]bool) []string {
	skip := skipSet(bindings.SkipTools, bindings.IncludeGatedTools, pkg.Tools)
	filtered := filterTools(pkg.Tools, skip)
	if known == nil {
		return filtered
	}
	out := make([]string, 0, len(filtered))
	for _, t := range filtered {
		if known[t] {
			out = append(out, t)
		}
	}
	return out
}

// DestFieldKeys is a helper for the service.
func DestFieldKeys(builtin string) []string {
	return schemaFieldKeys(builtin)
}
