package model

import (
	"time"

	"github.com/google/uuid"
)

// Plugin represents a user-uploaded LangGraph workflow plugin.
type Plugin struct {
	ID          uuid.UUID      `json:"id"`
	OrgID       uuid.UUID      `json:"org_id"`
	Name        string         `json:"name"`
	Version     string         `json:"version"`
	Description *string        `json:"description"`
	Status      string         `json:"status"`
	PluginType  string         `json:"plugin_type"`
	WorkflowDef map[string]any `json:"workflow_def"`
	Permissions []string       `json:"permissions"`
	Config      map[string]any `json:"config"`
	CreatedBy   *uuid.UUID     `json:"created_by"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// Plugin status values.
const (
	PluginStatusActive   = "active"
	PluginStatusInactive = "inactive"
	PluginStatusArchived = "archived"
)

// PluginExecution records a single invocation of a plugin.
type PluginExecution struct {
	ID           uuid.UUID  `json:"id"`
	PluginID     uuid.UUID  `json:"plugin_id"`
	ExecutionID  *uuid.UUID `json:"execution_id"`
	OrgID        uuid.UUID  `json:"org_id"`
	Input        map[string]any `json:"input"`
	Output       *string    `json:"output"`
	Status       string     `json:"status"`
	ErrorMessage *string    `json:"error_message"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// --- request / response types ---

type CreatePluginRequest struct {
	Name        string         `json:"name" validate:"required,min=2"`
	Version     string         `json:"version"`
	Description *string        `json:"description"`
	PluginType  string         `json:"plugin_type" validate:"omitempty,oneof=langgraph langchain"`
	WorkflowDef map[string]any `json:"workflow_def" validate:"required"`
	Permissions []string       `json:"permissions"`
	Config      map[string]any `json:"config"`
}

type UpdatePluginRequest struct {
	Name        *string        `json:"name"`
	Version     *string        `json:"version"`
	Description *string        `json:"description"`
	Status      *string        `json:"status" validate:"omitempty,oneof=active inactive archived"`
	WorkflowDef map[string]any `json:"workflow_def"`
	Permissions []string       `json:"permissions"`
	Config      map[string]any `json:"config"`
}

type ExecutePluginRequest struct {
	Input map[string]any `json:"input"`
}
