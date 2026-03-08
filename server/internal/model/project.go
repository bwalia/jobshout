package model

import (
	"time"
	"github.com/google/uuid"
)

type Project struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	OwnerID     *uuid.UUID `json:"owner_id"`
	DueDate     *time.Time `json:"due_date"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type CreateProjectRequest struct {
	Name        string  `json:"name" validate:"required,min=2"`
	Description *string `json:"description"`
	Priority    string  `json:"priority" validate:"omitempty,oneof=low medium high critical"`
	DueDate     *string `json:"due_date"`
}

type UpdateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status" validate:"omitempty,oneof=active paused completed archived"`
	Priority    *string `json:"priority" validate:"omitempty,oneof=low medium high critical"`
	DueDate     *string `json:"due_date"`
}
