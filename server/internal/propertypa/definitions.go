// Package propertypa defines the Property PA agents (Seller PA, Manager PA)
// as data, installs them into an org, and parses the output contract they
// emit. OpsAPI is the system of record and calls these agents through the
// generic run API (POST /tasks/{id}/run with role context and skill_slugs).
//
// These are generic agents, not builtin specialists: they have no Launch and
// no schema. Nothing here is special-cased by the platform; installing them is
// creating ordinary agents, tool permissions, approval rules and org skills.
package propertypa

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"strings"
)

//go:embed prompts/*.md
var promptFS embed.FS

// MetadataKey marks an installed PA agent: metadata.property_pa = Definition.Key.
// The installer finds an org's agent by it, so a re-install updates in place.
const MetadataKey = "property_pa"

// Agent keys.
const (
	KeySeller  = "seller"
	KeyManager = "manager"
)

// Prompt versions. Bump the version whenever the prompt text (common.md or the
// agent's own file) changes; TestPromptVersionsTrackPromptText fails until you do.
const (
	SellerPromptVersion  = "seller-pa@1.1.0"
	ManagerPromptVersion = "manager-pa@1.1.0"
)

// ExternalApprovalTools are tools that reach people outside the firm. Every
// PA agent gets an approval rule on each, so a run that calls one pauses for a
// human decision (authority level 4, execute with approval).
var ExternalApprovalTools = []string{"email_send", "slack_send_message", "teams_send_message"}

// Definition is one installable PA agent.
type Definition struct {
	Key           string
	Name          string
	Role          string // agents.role (free text shown in the UI)
	ContextRole   string // the context.role OpsAPI sends for this agent
	Description   string
	PromptVersion string
	SystemPrompt  string
	// Tools is the allow-list. Tools the server has not registered (e.g.
	// knowledge_search without an embedder) are dropped at install.
	Tools []string
	// ApprovalTools are gated: a call pauses the run for a human decision.
	ApprovalTools []string
	// Skills are the mode skills OpsAPI may pass as skill_slugs.
	Skills []string
}

// PromptSHA256 is the hex digest of the system prompt, stored beside the
// version so an audit can prove which text ran.
func (d Definition) PromptSHA256() string {
	sum := sha256.Sum256([]byte(d.SystemPrompt))
	return hex.EncodeToString(sum[:])
}

// Metadata is what the installer writes into agents.metadata.
func (d Definition) Metadata() map[string]any {
	return map[string]any{
		MetadataKey:      d.Key,
		"prompt_version": d.PromptVersion,
		"prompt_sha256":  d.PromptSHA256(),
		"context_role":   d.ContextRole,
		"pa_skills":      append([]string(nil), d.Skills...),
	}
}

// Skill is an org-private prompt skill that switches a PA run into one mode.
type Skill struct {
	Slug        string
	Name        string
	Description string
	Prompt      string
	Version     string
}

func mustPrompt(name string) string {
	b, err := promptFS.ReadFile("prompts/" + name)
	if err != nil {
		panic("propertypa: missing prompt " + name)
	}
	return strings.TrimSpace(string(b))
}

func compose(own string) string {
	return mustPrompt(own) + "\n\n" + mustPrompt("common.md") + "\n"
}

// Definitions returns the installable PA agents.
func Definitions() []Definition {
	reads := []string{"opsapi_get", "knowledge_search"}
	writes := []string{"opsapi_post"}
	external := append([]string(nil), ExternalApprovalTools[:1]...) // email_send: drafts a human sends
	tools := append(append(append([]string(nil), reads...), writes...), external...)
	return []Definition{
		{
			Key:           KeySeller,
			Name:          "Seller PA",
			Role:          "Seller agent's PA",
			ContextRole:   "seller_agent",
			Description:   "Property PA for a seller agent: today's priorities, prepare me, capture, coach and handoff. Grounded in OpsAPI records.",
			PromptVersion: SellerPromptVersion,
			SystemPrompt:  compose("seller_pa.md"),
			Tools:         tools,
			ApprovalTools: append([]string(nil), ExternalApprovalTools...),
			Skills:        []string{SkillToday, SkillPrepare, SkillCapture, SkillCoach, SkillHandoff},
		},
		{
			Key:           KeyManager,
			Name:          "Manager PA",
			Role:          "Sales manager's PA",
			ContextRole:   "manager",
			Description:   "Property PA for a manager: exceptions first, handoff decisions, teach the PA rules. Grounded in OpsAPI records.",
			PromptVersion: ManagerPromptVersion,
			SystemPrompt:  compose("manager_pa.md"),
			Tools:         tools,
			ApprovalTools: append([]string(nil), ExternalApprovalTools...),
			Skills:        []string{SkillManagerToday, SkillHandoff, SkillTeach},
		},
	}
}

// DefinitionByKey returns one definition.
func DefinitionByKey(key string) (Definition, bool) {
	for _, d := range Definitions() {
		if d.Key == key {
			return d, true
		}
	}
	return Definition{}, false
}

// Mode skill slugs. OpsAPI passes one per run as skill_slugs.
const (
	SkillToday        = "pa-today"
	SkillPrepare      = "pa-prepare-me"
	SkillCapture      = "pa-capture"
	SkillCoach        = "pa-coach"
	SkillHandoff      = "pa-handoff"
	SkillManagerToday = "pa-manager-today"
	SkillTeach        = "pa-teach"
)

// SkillsVersion is the version written on every mode skill.
const SkillsVersion = "1.1.0"

// Skills returns the mode skills.
func Skills() []Skill {
	s := func(slug, name, prompt string) Skill {
		return Skill{Slug: slug, Name: name, Description: "Property PA mode: " + name, Prompt: prompt, Version: SkillsVersion}
	}
	return []Skill{
		s(SkillToday, "Today", `Mode "today". Read /api/v2/property-pa/briefing first. Give a two-line brief, then the ranked priority cards: who, property, recommended action, why now, objective. Keep OpsAPI's order. If the user gives a time budget, fit the list to it and say what waits.`),
		s(SkillPrepare, "Prepare me", `Mode "prepare". Resolve the person or property (/api/v2/property-pa/people?q=), then read /api/v2/property-pa/context/{opportunity_id} and the timeline. Brief in 30 to 60 seconds of reading: snapshot, motivation, last meaningful interaction, price and offer position, decision makers, promises, likely objections, questions to establish, the playbook rule, the objective. Every fact cited; unknowns listed.`),
		s(SkillCapture, "Capture", `Mode "capture". The input is a note or transcript of something that happened. Resolve the person and property with /api/v2/property-pa/people?q= (ask if ambiguous), then read /api/v2/property-pa/context/{opportunity_id}. Send one capture to /api/v2/property-pa/captures: the interaction plus each field change as a separate proposal with confidence, is_inferred and an exact source_quote, and the next action (owner, due time, reason). List what you proposed so the user can confirm or correct it.`),
		s(SkillCoach, "Coach", `Mode "coach". Separate what is known (cited) from what is assumed. Ask the questions that would change the decision before discussing any figure. Point to the relevant approved rule. Recommend one next step. Never default to a higher offer or a price cut.`),
		s(SkillHandoff, "Handoff", `Mode "handoff". Package the context (three lines, cited), the exact question and your recommendation into one handoff via /api/v2/property-pa/handoffs. If you are the manager's PA and the manager has given a reply in this conversation, record it; otherwise present the decision.`),
		s(SkillManagerToday, "Manager today", `Mode "manager_today". Exceptions only: handoffs waiting, unusual negotiations, relationship risks, deal risks, top opportunities. End with "Everything else is being handled." Do not list routine work.`),
		s(SkillTeach, "Teach", `Mode "teach". Turn the manager's statement into one rule proposal (rule, rationale, category, scope, triggers) via POST /api/v2/property-pa/knowledge (proposed_by_ai true). It takes effect only after approval in OpsAPI.`),
	}
}
