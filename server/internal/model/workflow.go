package model

import (
	"time"

	"github.com/google/uuid"
)

// Workflow is a user-defined multi-agent pipeline stored in the database.
type Workflow struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	Status      string     `json:"status"`
	Steps       []WorkflowStep `json:"steps,omitempty"`
	CreatedBy   *uuid.UUID `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// WorkflowStep is a single node in the workflow DAG.
type WorkflowStep struct {
	ID            uuid.UUID   `json:"id"`
	WorkflowID    uuid.UUID   `json:"workflow_id"`
	Name          string      `json:"name"`
	AgentID       uuid.UUID   `json:"agent_id"`
	// InputTemplate is a plain-text or Go-template string describing the task
	// to hand to the agent. Use {{.Outputs.<step_name>}} to reference the
	// output of a prior step.
	InputTemplate string      `json:"input_template"`
	Position      int         `json:"position"`
	// DependsOn lists the names of steps that must complete before this step.
	DependsOn     []string    `json:"depends_on,omitempty"`
	// EngineType overrides the agent's default engine for this step.
	// When empty, the agent's EngineType is used.
	EngineType    string      `json:"engine_type,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
}

// WorkflowRun is a single invocation of a Workflow.
type WorkflowRun struct {
	ID           uuid.UUID              `json:"id"`
	WorkflowID   uuid.UUID              `json:"workflow_id"`
	OrgID        uuid.UUID              `json:"org_id"`
	Status       string                 `json:"status"`
	Input        map[string]any         `json:"input"`
	Outputs      map[string]string      `json:"outputs"`
	ErrorMessage *string                `json:"error_message"`
	StartedAt    *time.Time             `json:"started_at"`
	CompletedAt  *time.Time             `json:"completed_at"`
	TriggeredBy  *uuid.UUID             `json:"triggered_by"`
	CreatedAt    time.Time              `json:"created_at"`
}

// --- request / response types ---

type CreateWorkflowRequest struct {
	Name        string                      `json:"name" validate:"required,min=2"`
	Description *string                     `json:"description"`
	Steps       []CreateWorkflowStepRequest `json:"steps" validate:"required,min=1,dive"`
}

type CreateWorkflowStepRequest struct {
	Name          string   `json:"name" validate:"required,min=1"`
	AgentID       string   `json:"agent_id" validate:"required,uuid"`
	InputTemplate string   `json:"input_template" validate:"required"`
	Position      int      `json:"position"`
	DependsOn     []string `json:"depends_on"`
	EngineType    string   `json:"engine_type,omitempty" validate:"omitempty,oneof=go_native langchain langgraph"`
}

type ExecuteWorkflowRequest struct {
	// Input is arbitrary key/value data made available to step templates.
	Input map[string]any `json:"input"`
}

type UpdateWorkflowRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status" validate:"omitempty,oneof=draft active archived"`
}
