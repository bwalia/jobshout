package model

import (
	"time"
	"github.com/google/uuid"
)

type Agent struct {
	ID               uuid.UUID  `json:"id"`
	OrgID            uuid.UUID  `json:"org_id"`
	Name             string     `json:"name"`
	Role             string     `json:"role"`
	Description      *string    `json:"description"`
	AvatarURL        *string    `json:"avatar_url"`
	Status           string     `json:"status"`
	ModelProvider    *string    `json:"model_provider"`
	ModelName        *string    `json:"model_name"`
	SystemPrompt     *string    `json:"system_prompt"`
	PerformanceScore float64    `json:"performance_score"`
	ManagerID        *uuid.UUID `json:"manager_id"`
	CreatedBy        *uuid.UUID `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type CreateAgentRequest struct {
	Name          string  `json:"name" validate:"required,min=2"`
	Role          string  `json:"role" validate:"required,min=2"`
	Description   *string `json:"description"`
	ModelProvider *string `json:"model_provider"`
	ModelName     *string `json:"model_name"`
	SystemPrompt  *string `json:"system_prompt"`
	ManagerID     *string `json:"manager_id"`
}

type UpdateAgentRequest struct {
	Name          *string `json:"name"`
	Role          *string `json:"role"`
	Description   *string `json:"description"`
	ModelProvider *string `json:"model_provider"`
	ModelName     *string `json:"model_name"`
	SystemPrompt  *string `json:"system_prompt"`
	ManagerID     *string `json:"manager_id"`
}

type UpdateAgentStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=idle active paused offline"`
}

type PaginationParams struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
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
	if p.PerPage > 100 {
		p.PerPage = 100
	}
}
