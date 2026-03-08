package model

import (
	"time"
	"github.com/google/uuid"
)

type Task struct {
	ID              uuid.UUID  `json:"id"`
	ProjectID       uuid.UUID  `json:"project_id"`
	ParentID        *uuid.UUID `json:"parent_id"`
	Title           string     `json:"title"`
	Description     *string    `json:"description"`
	Status          string     `json:"status"`
	Priority        string     `json:"priority"`
	AssignedAgentID *uuid.UUID `json:"assigned_agent_id"`
	AssignedUserID  *uuid.UUID `json:"assigned_user_id"`
	StoryPoints     *int       `json:"story_points"`
	DueDate         *time.Time `json:"due_date"`
	Position        int        `json:"position"`
	CreatedBy       *uuid.UUID `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type CreateTaskRequest struct {
	ProjectID       string  `json:"project_id" validate:"required,uuid"`
	Title           string  `json:"title" validate:"required,min=2"`
	Description     *string `json:"description"`
	Priority        string  `json:"priority" validate:"omitempty,oneof=low medium high critical"`
	AssignedAgentID *string `json:"assigned_agent_id"`
	AssignedUserID  *string `json:"assigned_user_id"`
	StoryPoints     *int    `json:"story_points"`
	DueDate         *string `json:"due_date"`
	ParentID        *string `json:"parent_id"`
}

type UpdateTaskRequest struct {
	Title           *string `json:"title"`
	Description     *string `json:"description"`
	Priority        *string `json:"priority" validate:"omitempty,oneof=low medium high critical"`
	AssignedAgentID *string `json:"assigned_agent_id"`
	AssignedUserID  *string `json:"assigned_user_id"`
	StoryPoints     *int    `json:"story_points"`
	DueDate         *string `json:"due_date"`
}

type TransitionTaskRequest struct {
	Status string `json:"status" validate:"required,oneof=backlog todo in_progress review done"`
}

type ReorderTaskRequest struct {
	Status   string `json:"status" validate:"required,oneof=backlog todo in_progress review done"`
	Position int    `json:"position" validate:"min=0"`
}

type TaskComment struct {
	ID        uuid.UUID  `json:"id"`
	TaskID    uuid.UUID  `json:"task_id"`
	AuthorID  *uuid.UUID `json:"author_id"`
	AgentID   *uuid.UUID `json:"agent_id"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
}

type AddCommentRequest struct {
	Body string `json:"body" validate:"required,min=1"`
}
