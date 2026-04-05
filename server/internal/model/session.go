package model

import (
	"time"

	"github.com/google/uuid"
)

// Session preserves conversation context across LLM provider switches.
type Session struct {
	ID               uuid.UUID      `json:"id"`
	OrgID            uuid.UUID      `json:"org_id"`
	Name             string         `json:"name"`
	Description      *string        `json:"description"`
	ProviderConfigID *uuid.UUID     `json:"provider_config_id"`
	ModelName        *string        `json:"model_name"`
	Status           string         `json:"status"` // active | archived | deleted
	ContextMessages  []SessionMsg   `json:"context_messages"`
	TotalTokens      int            `json:"total_tokens"`
	MessageCount     int            `json:"message_count"`
	Tags             []string       `json:"tags"`
	CreatedBy        *uuid.UUID     `json:"created_by"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// SessionMsg is a single message in a session's context.
type SessionMsg struct {
	Role         string `json:"role"`
	Content      string `json:"content"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	TokenCount   int    `json:"token_count,omitempty"`
	Timestamp    string `json:"timestamp,omitempty"`
}

type CreateSessionRequest struct {
	Name             string   `json:"name" validate:"required,min=2"`
	Description      *string  `json:"description"`
	ProviderConfigID *string  `json:"provider_config_id"`
	ModelName        *string  `json:"model_name"`
	Tags             []string `json:"tags"`
}

type UpdateSessionRequest struct {
	Name             *string  `json:"name"`
	Description      *string  `json:"description"`
	ProviderConfigID *string  `json:"provider_config_id"`
	ModelName        *string  `json:"model_name"`
	Status           *string  `json:"status" validate:"omitempty,oneof=active archived deleted"`
	Tags             []string `json:"tags"`
}

// SendMessageRequest sends a user message into a session and gets an LLM reply.
type SendMessageRequest struct {
	Message string `json:"message" validate:"required,min=1"`
}

// CopyContextRequest copies context from one session to another.
type CopyContextRequest struct {
	SourceSessionID string  `json:"source_session_id" validate:"required,uuid"`
	IncludeSystem   bool    `json:"include_system"`
	MaxMessages     *int    `json:"max_messages"`
}

// SessionSnapshot is a saved checkpoint of session context.
type SessionSnapshot struct {
	ID              uuid.UUID    `json:"id"`
	SessionID       uuid.UUID    `json:"session_id"`
	Name            string       `json:"name"`
	Description     *string      `json:"description"`
	ContextMessages []SessionMsg `json:"context_messages"`
	ProviderType    *string      `json:"provider_type"`
	ModelName       *string      `json:"model_name"`
	TotalTokens     int          `json:"total_tokens"`
	MessageCount    int          `json:"message_count"`
	CreatedAt       time.Time    `json:"created_at"`
}

type CreateSnapshotRequest struct {
	Name        string  `json:"name" validate:"required,min=2"`
	Description *string `json:"description"`
}

// TaskDependency represents a dependency between two tasks.
type TaskDependency struct {
	TaskID         uuid.UUID `json:"task_id"`
	DependsOnID    uuid.UUID `json:"depends_on_id"`
	DependencyType string    `json:"dependency_type"` // blocks | related | subtask
}

type CreateTaskDependencyRequest struct {
	DependsOnID    string `json:"depends_on_id" validate:"required,uuid"`
	DependencyType string `json:"dependency_type" validate:"required,oneof=blocks related subtask"`
}
