package propertypa

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

// AgentStore is the agent persistence the installer needs.
type AgentStore interface {
	// FindByPAKey returns the org's agent with metadata.property_pa = key, or nil.
	FindByPAKey(ctx context.Context, orgID uuid.UUID, key string) (*model.Agent, error)
	// NameTaken reports whether any agent in the org has this name (case-insensitive).
	NameTaken(ctx context.Context, orgID uuid.UUID, name string) (bool, error)
	Create(ctx context.Context, agent *model.Agent) error
	// UpdateDefinition rewrites role, description, prompt and engine, and merges
	// metadata into the agent's metadata. The name and model are left alone so
	// an admin's rename or model pin survives a re-install.
	UpdateDefinition(ctx context.Context, orgID, agentID uuid.UUID, role, description, systemPrompt string, metadata map[string]any) error
}

// ToolSetter replaces an agent's tool allow-list (repository.AgentToolRepository).
type ToolSetter interface {
	Set(ctx context.Context, agentID uuid.UUID, toolNames []string) error
}

// ApprovalRules reads and replaces an agent's gated tools (repository.ApprovalRepository).
type ApprovalRules interface {
	ListRules(ctx context.Context, agentID uuid.UUID) ([]string, error)
	SetRules(ctx context.Context, agentID uuid.UUID, toolNames []string) error
}

// SkillStore upserts org skills (repository.SkillRepository).
type SkillStore interface {
	List(ctx context.Context, orgID uuid.UUID) ([]model.Skill, error)
	Create(ctx context.Context, s *model.Skill) error
	Update(ctx context.Context, s *model.Skill) error
}

// Installer installs and upgrades the PA agents in an org. Idempotent.
type Installer struct {
	Agents    AgentStore
	Tools     ToolSetter
	Approvals ApprovalRules
	Skills    SkillStore
	// HasTool filters the definitions' tools to registered ones. Nil keeps all.
	HasTool func(name string) bool
}

// Install statuses.
const (
	StatusCreated   = "created"
	StatusUpdated   = "updated"
	StatusUnchanged = "unchanged"
)

// InstalledAgent is one agent in an install result.
type InstalledAgent struct {
	Key           string   `json:"key"`
	AgentID       string   `json:"agent_id"`
	Name          string   `json:"name"`
	ContextRole   string   `json:"context_role"`
	PromptVersion string   `json:"prompt_version"`
	PromptSHA256  string   `json:"prompt_sha256"`
	Tools         []string `json:"tools"`
	ApprovalTools []string `json:"approval_tools"`
	Skills        []string `json:"skills"`
	Status        string   `json:"status"`
	SkippedTools  []string `json:"skipped_tools,omitempty"`
}

// InstalledSkill is one skill in an install result.
type InstalledSkill struct {
	Slug    string `json:"slug"`
	ID      string `json:"id"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

// InstallResult is the response of an install.
type InstallResult struct {
	Agents []InstalledAgent `json:"agents"`
	Skills []InstalledSkill `json:"skills"`
}

// Install creates or updates every PA agent and mode skill in orgID.
func (in *Installer) Install(ctx context.Context, orgID, userID uuid.UUID) (*InstallResult, error) {
	if in.Agents == nil || in.Tools == nil || in.Approvals == nil || in.Skills == nil {
		return nil, fmt.Errorf("propertypa: installer is not fully wired")
	}
	res := &InstallResult{}
	skills, err := in.installSkills(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	res.Skills = skills
	for _, def := range Definitions() {
		a, err := in.installAgent(ctx, orgID, userID, def)
		if err != nil {
			return nil, err
		}
		res.Agents = append(res.Agents, *a)
	}
	return res, nil
}

func (in *Installer) installAgent(ctx context.Context, orgID, userID uuid.UUID, def Definition) (*InstalledAgent, error) {
	tools, skipped := in.filterTools(def.Tools)
	out := &InstalledAgent{
		Key: def.Key, ContextRole: def.ContextRole, PromptVersion: def.PromptVersion,
		PromptSHA256: def.PromptSHA256(), Tools: tools, Skills: def.Skills, SkippedTools: skipped,
	}

	existing, err := in.Agents.FindByPAKey(ctx, orgID, def.Key)
	if err != nil {
		return nil, fmt.Errorf("propertypa: find %s agent: %w", def.Key, err)
	}
	var agentID uuid.UUID
	if existing == nil {
		name, err := in.freeName(ctx, orgID, def.Name)
		if err != nil {
			return nil, err
		}
		desc, prompt := def.Description, def.SystemPrompt
		creator := userID
		a := &model.Agent{
			ID: uuid.New(), OrgID: orgID, Name: name, Role: def.Role, Description: &desc,
			Status: "idle", SystemPrompt: &prompt, CreatedBy: &creator,
			EngineType: model.EngineGoNative, EngineConfig: map[string]any{}, Metadata: def.Metadata(),
		}
		if err := in.Agents.Create(ctx, a); err != nil {
			return nil, fmt.Errorf("propertypa: create %s agent: %w", def.Key, err)
		}
		agentID, out.Name, out.Status = a.ID, a.Name, StatusCreated
	} else {
		agentID, out.Name, out.Status = existing.ID, existing.Name, StatusUnchanged
		if !upToDate(existing, def) {
			if err := in.Agents.UpdateDefinition(ctx, orgID, existing.ID, def.Role, def.Description,
				def.SystemPrompt, def.Metadata()); err != nil {
				return nil, fmt.Errorf("propertypa: update %s agent: %w", def.Key, err)
			}
			out.Status = StatusUpdated
		}
	}
	out.AgentID = agentID.String()

	// The tool allow-list is authoritative: it is the PA's authority boundary.
	if err := in.Tools.Set(ctx, agentID, tools); err != nil {
		return nil, fmt.Errorf("propertypa: set %s tools: %w", def.Key, err)
	}
	// Approval rules are additive: an admin may gate more, never fewer.
	current, err := in.Approvals.ListRules(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("propertypa: list %s approval rules: %w", def.Key, err)
	}
	rules := union(current, def.ApprovalTools)
	if err := in.Approvals.SetRules(ctx, agentID, rules); err != nil {
		return nil, fmt.Errorf("propertypa: set %s approval rules: %w", def.Key, err)
	}
	out.ApprovalTools = rules
	return out, nil
}

func upToDate(a *model.Agent, def Definition) bool {
	md := a.Metadata
	return md != nil &&
		md["prompt_version"] == def.PromptVersion &&
		md["prompt_sha256"] == def.PromptSHA256() &&
		a.Role == def.Role &&
		a.SystemPrompt != nil && *a.SystemPrompt == def.SystemPrompt &&
		a.EngineType == model.EngineGoNative
}

func (in *Installer) freeName(ctx context.Context, orgID uuid.UUID, base string) (string, error) {
	candidates := []string{base, base + " (Property PA)"}
	for i := 2; i <= 9; i++ {
		candidates = append(candidates, fmt.Sprintf("%s (Property PA %d)", base, i))
	}
	for _, name := range candidates {
		taken, err := in.Agents.NameTaken(ctx, orgID, name)
		if err != nil {
			return "", fmt.Errorf("propertypa: check name: %w", err)
		}
		if !taken {
			return name, nil
		}
	}
	return "", fmt.Errorf("propertypa: no free name for %q", base)
}

func (in *Installer) filterTools(want []string) (kept, skipped []string) {
	for _, t := range want {
		if in.HasTool == nil || in.HasTool(t) {
			kept = append(kept, t)
		} else {
			skipped = append(skipped, t)
		}
	}
	return kept, skipped
}

func (in *Installer) installSkills(ctx context.Context, orgID, userID uuid.UUID) ([]InstalledSkill, error) {
	list, err := in.Skills.List(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("propertypa: list skills: %w", err)
	}
	own := map[string]model.Skill{}
	for _, s := range list {
		if s.OrgID != nil && *s.OrgID == orgID {
			own[s.Slug] = s
		}
	}
	var out []InstalledSkill
	for _, sk := range Skills() {
		desc := sk.Description
		cfg := map[string]any{"prompt": sk.Prompt, MetadataKey: true}
		if cur, ok := own[sk.Slug]; ok {
			status := StatusUnchanged
			curPrompt, _ := cur.ConfigJSON["prompt"].(string)
			if cur.Version != sk.Version || curPrompt != sk.Prompt || cur.Kind != "prompt" || cur.Status == "deprecated" {
				cur.Name, cur.Description, cur.Kind, cur.ConfigJSON = sk.Name, &desc, "prompt", cfg
				cur.Version, cur.Status = sk.Version, "published"
				if err := in.Skills.Update(ctx, &cur); err != nil {
					return nil, fmt.Errorf("propertypa: update skill %s: %w", sk.Slug, err)
				}
				status = StatusUpdated
			}
			out = append(out, InstalledSkill{Slug: sk.Slug, ID: cur.ID.String(), Version: sk.Version, Status: status})
			continue
		}
		org, creator := orgID, userID
		s := &model.Skill{
			ID: uuid.New(), OrgID: &org, Slug: sk.Slug, Name: sk.Name, Description: &desc,
			Kind: "prompt", ConfigJSON: cfg, Version: sk.Version, Status: "published", CreatedBy: &creator,
		}
		if err := in.Skills.Create(ctx, s); err != nil {
			return nil, fmt.Errorf("propertypa: create skill %s: %w", sk.Slug, err)
		}
		out = append(out, InstalledSkill{Slug: sk.Slug, ID: s.ID.String(), Version: sk.Version, Status: StatusCreated})
	}
	return out, nil
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string(nil), a...), b...) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// Status describes what an org has installed against what this server ships.
type Status struct {
	Key              string `json:"key"`
	Name             string `json:"name"`
	ContextRole      string `json:"context_role"`
	AvailableVersion string `json:"available_version"`
	Installed        bool   `json:"installed"`
	AgentID          string `json:"agent_id,omitempty"`
	AgentName        string `json:"agent_name,omitempty"`
	InstalledVersion string `json:"installed_version,omitempty"`
	UpgradeAvailable bool   `json:"upgrade_available"`
}

// List reports the org's PA agents.
func (in *Installer) List(ctx context.Context, orgID uuid.UUID) ([]Status, error) {
	var out []Status
	for _, def := range Definitions() {
		st := Status{Key: def.Key, Name: def.Name, ContextRole: def.ContextRole, AvailableVersion: def.PromptVersion}
		a, err := in.Agents.FindByPAKey(ctx, orgID, def.Key)
		if err != nil {
			return nil, fmt.Errorf("propertypa: find %s agent: %w", def.Key, err)
		}
		if a != nil {
			st.Installed, st.AgentID, st.AgentName = true, a.ID.String(), a.Name
			st.InstalledVersion, _ = a.Metadata["prompt_version"].(string)
			st.UpgradeAvailable = !upToDate(a, def)
		}
		out = append(out, st)
	}
	return out, nil
}
