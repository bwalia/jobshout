package model

import (
	"time"

	"github.com/google/uuid"
)

// AgentMemoryShortTerm holds per-session conversation context for an agent.
type AgentMemoryShortTerm struct {
	ID        uuid.UUID `json:"id"`
	AgentID   uuid.UUID `json:"agent_id"`
	SessionID uuid.UUID `json:"session_id"`
	Messages  []byte    `json:"messages"` // JSONB: serialised []llm.Message
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AgentMemoryLongTerm stores persistent knowledge that an agent can recall.
type AgentMemoryLongTerm struct {
	ID        uuid.UUID      `json:"id"`
	AgentID   uuid.UUID      `json:"agent_id"`
	OrgID     uuid.UUID      `json:"org_id"`
	Content   string         `json:"content"`
	Summary   string         `json:"summary"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}
