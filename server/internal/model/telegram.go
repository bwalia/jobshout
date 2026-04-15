package model

import (
	"time"

	"github.com/google/uuid"
)

// TelegramUserMapping links a Telegram user to a JobShout account.
type TelegramUserMapping struct {
	ID               uuid.UUID `json:"id"`
	TelegramUserID   int64     `json:"telegram_user_id"`
	TelegramUsername string    `json:"telegram_username"`
	JobshoutUserID   uuid.UUID `json:"jobshout_user_id"`
	OrgID            uuid.UUID `json:"org_id"`
	Verified         bool      `json:"verified"`
	LinkedAt         time.Time `json:"linked_at"`
}

// TelegramLinkToken is a one-time token used for account linking via /start.
type TelegramLinkToken struct {
	Token     string    `json:"token"`
	UserID    uuid.UUID `json:"user_id"`
	OrgID     uuid.UUID `json:"org_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// GenerateLinkTokenResponse is the API response after generating a link token.
type GenerateLinkTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	BotURL    string    `json:"bot_url"`
}

// TelegramLinkStatusResponse shows the current Telegram linking status.
type TelegramLinkStatusResponse struct {
	Linked           bool    `json:"linked"`
	TelegramUsername *string `json:"telegram_username,omitempty"`
	TelegramUserID   *int64  `json:"telegram_user_id,omitempty"`
}
