package model

import (
	"time"

	"github.com/google/uuid"
)

// Organization represents a company or team in the system.
type Organization struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	Slug      string     `json:"slug"`
	OwnerID   *uuid.UUID `json:"owner_id"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// UpdateOrgChartEntry is a single agent reporting line update.
type UpdateOrgChartEntry struct {
	AgentID   uuid.UUID  `json:"agent_id" validate:"required"`
	ManagerID *uuid.UUID `json:"manager_id"`
}

// UpdateOrgChartRequest represents a bulk update to the organization chart.
type UpdateOrgChartRequest struct {
	Agents []UpdateOrgChartEntry `json:"agents" validate:"required,dive"`
}
