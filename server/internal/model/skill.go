package model

import (
	"time"

	"github.com/google/uuid"
)

// Skill is one entry in the OpenClaw-style capability registry. A skill is
// the metadata + config blob; it does NOT yet ship an execution surface —
// that's an explicit follow-up phase to keep this scope honest.
type Skill struct {
	ID          uuid.UUID      `json:"id"`
	OrgID       *uuid.UUID     `json:"org_id,omitempty"` // nil → built-in / community
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Description *string        `json:"description,omitempty"`
	Kind        string         `json:"kind"` // tool | prompt | bundle
	ConfigJSON  map[string]any `json:"config_json"`
	Version     string         `json:"version"`
	Status      string         `json:"status"` // draft | published | deprecated
	CreatedBy   *uuid.UUID     `json:"created_by,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type CreateSkillRequest struct {
	Slug        string         `json:"slug" validate:"required,min=2"`
	Name        string         `json:"name" validate:"required,min=2"`
	Description *string        `json:"description"`
	Kind        string         `json:"kind" validate:"omitempty,oneof=tool prompt bundle"`
	ConfigJSON  map[string]any `json:"config_json"`
	Version     string         `json:"version"`
}

// UpdateSkillRequest patches a skill's mutable fields. All fields are optional;
// only the ones provided are applied, which is what lets the UI drive status
// transitions (draft → published → deprecated) in isolation.
type UpdateSkillRequest struct {
	Name        *string        `json:"name" validate:"omitempty,min=2"`
	Description *string        `json:"description"`
	Kind        *string        `json:"kind" validate:"omitempty,oneof=tool prompt bundle"`
	ConfigJSON  map[string]any `json:"config_json"`
	Version     *string        `json:"version"`
	Status      *string        `json:"status" validate:"omitempty,oneof=draft published deprecated"`
}

type AgentSkill struct {
	AgentID        uuid.UUID      `json:"agent_id"`
	SkillID        uuid.UUID      `json:"skill_id"`
	ConfigOverride map[string]any `json:"config_override"`
	Enabled        bool           `json:"enabled"`
	EnabledAt      time.Time      `json:"enabled_at"`
}

type EnableSkillRequest struct {
	SkillID        uuid.UUID      `json:"skill_id" validate:"required"`
	ConfigOverride map[string]any `json:"config_override"`
}
