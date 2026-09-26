package model

import (
	"time"

	"github.com/google/uuid"
)

// Chat source constants.
const (
	ChatSourceWeb      = "web"
	ChatSourceTelegram = "telegram"
	ChatSourceAPI      = "api"
)

// Chat role constants.
const (
	ChatRoleUser   = "user"
	ChatRoleAgent  = "agent"
	ChatRoleSystem = "system"
	ChatRoleTool   = "tool"
)

// ChatSession represents a conversation between a user and the system.
type ChatSession struct {
	ID        uuid.UUID      `json:"id"`
	OrgID     uuid.UUID      `json:"org_id"`
	UserID    uuid.UUID      `json:"user_id"`
	AgentID   *uuid.UUID     `json:"agent_id,omitempty"`
	Source    string         `json:"source"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ChatMessage is a single turn in a chat session, forming the audit trail.
type ChatMessage struct {
	ID        uuid.UUID      `json:"id"`
	SessionID uuid.UUID      `json:"session_id"`
	OrgID     uuid.UUID      `json:"org_id"`
	Role      string         `json:"role"`
	Source    string         `json:"source"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

// StartChatSessionRequest is the API payload for creating a new chat session.
type StartChatSessionRequest struct {
	AgentID *uuid.UUID `json:"agent_id,omitempty"`
	Source  string     `json:"source,omitempty"`
}

// SendChatMessageRequest is the API payload for sending a message in a session.
type SendChatMessageRequest struct {
	Content           string `json:"content" validate:"required,min=1"`
	Source            string `json:"source,omitempty"`
	ConfirmationToken string `json:"confirmation_token,omitempty"`
	// DisplayContent, when set, is what the transcript shows for this message
	// instead of Content. Used by clarify option picks, where Content carries
	// the machine value (e.g. a project UUID) and DisplayContent the label the
	// user actually clicked. The agent always acts on Content.
	DisplayContent string `json:"display_content,omitempty" validate:"omitempty,max=2000"`
}
