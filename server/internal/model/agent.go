package model

import (
	"github.com/google/uuid"
	"time"
)

// EngineType constants identify which runtime executes a given agent or step.
const (
	EngineGoNative  = "go_native"
	EngineLangChain = "langchain"
	EngineLangGraph = "langgraph"
)

type Agent struct {
	ID               uuid.UUID      `json:"id"`
	OrgID            uuid.UUID      `json:"org_id"`
	Name             string         `json:"name"`
	Role             string         `json:"role"`
	Description      *string        `json:"description"`
	AvatarURL        *string        `json:"avatar_url"`
	Status           string         `json:"status"`
	ModelProvider    *string        `json:"model_provider"`
	ModelName        *string        `json:"model_name"`
	SystemPrompt     *string        `json:"system_prompt"`
	PerformanceScore float64        `json:"performance_score"`
	ManagerID        *uuid.UUID     `json:"manager_id"`
	CreatedBy        *uuid.UUID     `json:"created_by"`
	EngineType       string         `json:"engine_type"`
	EngineConfig     map[string]any `json:"engine_config"`
	// Metadata carries platform-owned annotations that are not user-editable.
	// Built-in agents seeded by the platform are marked here — see
	// BuiltinArticleWriter — which is how the UI tells them apart from agents a
	// user created.
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// MetadataKeyBuiltin is the Metadata key identifying a platform-seeded agent,
// and the Builtin* values name each one. Kept as constants because migration
// 000019 and auth_service.Register both write these exact markers and the
// board/UI read them.
const (
	MetadataKeyBuiltin   = "builtin"
	BuiltinArticleWriter = "article_writer"
	// BuiltinResearcher is the Research Agent: it searches the internet, reads
	// sources and returns verified findings. The Article Writer is its first
	// consumer, but it is deliberately a separate agent — "find out about X and
	// come back with sources you have actually read" is a capability worth
	// having on its own, and it appears on the board in its own right.
	BuiltinResearcher = "researcher"
	// BuiltinPentester is the Penetration Testing Agent: autonomous security testing
	// powered by Strix. Tests live APIs, applications, and codebases for vulnerabilities.
	BuiltinPentester = "pentester"
	// BuiltinPRReviewer reviews GitHub pull requests via the in-cluster review-bot sidecar.
	BuiltinPRReviewer = "pr_reviewer"
	// BuiltinMail is the Mail Agent: one shared org Gmail, draft-only replies,
	// Research Agent handoff, human approve-before-send.
	BuiltinMail = "mail"
	// BuiltinImages is the Image Generator: one prompt in, one stored image
	// on the Task Manager board.
	BuiltinImages = "images"
)

// EngineConfigStructuredModel is the EngineConfig key holding the Article
// Writer's model for calls that must return JSON — choosing the title and
// outline, and reviewing the draft.
//
// The agent's own ModelName is the writing model, because that is the field the
// Agents UI has always shown and the one a user setting "the model for this
// agent" means. The second model lives in EngineConfig instead of getting its
// own column: it is configuration for how this particular agent runs, which is
// exactly what EngineConfig is for, and it needs no migration.
const EngineConfigStructuredModel = "structured_model"

// IsBuiltin reports whether the agent was seeded by the platform under the
// given builtin name.
func (a *Agent) IsBuiltin(name string) bool {
	if a.Metadata == nil {
		return false
	}
	v, _ := a.Metadata[MetadataKeyBuiltin].(string)
	return v == name
}

type CreateAgentRequest struct {
	Name          string         `json:"name" validate:"required,min=2"`
	Role          string         `json:"role" validate:"required,min=2"`
	Description   *string        `json:"description"`
	ModelProvider *string        `json:"model_provider"`
	ModelName     *string        `json:"model_name"`
	SystemPrompt  *string        `json:"system_prompt"`
	ManagerID     *string        `json:"manager_id"`
	EngineType    *string        `json:"engine_type" validate:"omitempty,oneof=go_native langchain langgraph"`
	EngineConfig  map[string]any `json:"engine_config"`
}

type UpdateAgentRequest struct {
	Name          *string        `json:"name"`
	Role          *string        `json:"role"`
	Description   *string        `json:"description"`
	ModelProvider *string        `json:"model_provider"`
	ModelName     *string        `json:"model_name"`
	SystemPrompt  *string        `json:"system_prompt"`
	ManagerID     *string        `json:"manager_id"`
	EngineType    *string        `json:"engine_type" validate:"omitempty,oneof=go_native langchain langgraph"`
	EngineConfig  map[string]any `json:"engine_config"`
}

type UpdateAgentStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=idle active paused offline"`
}

type PaginationParams struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	// Status optionally filters list endpoints that support it (org task list).
	Status string `json:"-"`
	// AssignedAgentID optionally filters the org task list to one agent.
	AssignedAgentID string `json:"-"`
}

type PaginatedResponse[T any] struct {
	Data       []T `json:"data"`
	Total      int `json:"total"`
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
}

func (p *PaginationParams) Offset() int {
	return (p.Page - 1) * p.PerPage
}

func (p *PaginationParams) Normalize() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PerPage < 1 {
		p.PerPage = 20
	}
	if p.PerPage > 200 {
		p.PerPage = 200
	}
}
