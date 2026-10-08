package model

import (
	"time"

	"github.com/google/uuid"
)

// Device is one signed-in app install. Refresh tokens issued to it carry its
// id, so revoking the device revokes that install's session.
type Device struct {
	ID              uuid.UUID `json:"id"`
	UserID          uuid.UUID `json:"-"`
	Platform        string    `json:"platform"`
	Name            string    `json:"name"`
	AppVersion      string    `json:"app_version"`
	PushToken       *string   `json:"-"`
	PushEnvironment *string   `json:"push_environment,omitempty"`
	HasPush         bool      `json:"has_push"`
	CreatedAt       time.Time `json:"created_at"`
	LastSeenAt      time.Time `json:"last_seen_at"`
}

// DeviceInfo is what a native client sends with a sign-in. ID is the id the
// server returned on an earlier sign-in, if the client kept one; the server
// ignores it unless it belongs to the same user.
type DeviceInfo struct {
	ID         string `json:"id,omitempty"`
	Platform   string `json:"platform" validate:"required,oneof=ios android macos watchos"`
	Name       string `json:"name" validate:"max=255"`
	AppVersion string `json:"app_version" validate:"max=64"`
}

// DevicePushRequest registers (or clears, with an empty token) a device's
// APNs token.
type DevicePushRequest struct {
	Token       string `json:"token" validate:"max=512"`
	Environment string `json:"environment" validate:"omitempty,oneof=sandbox production"`
}

// LogoutRequest revokes the session a refresh token belongs to.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// AppleSignInRequest exchanges a Sign in with Apple identity token for
// JobShout tokens. Nonce is the raw nonce; the app put its SHA-256 in the
// Apple request. FullName is only ever sent by Apple on the first sign-in, so
// the client forwards it when it has it.
type AppleSignInRequest struct {
	IdentityToken string      `json:"identity_token" validate:"required"`
	Nonce         string      `json:"nonce" validate:"required,min=16"`
	FullName      string      `json:"full_name" validate:"max=255"`
	OrgName       string      `json:"org_name" validate:"max=255"`
	Device        *DeviceInfo `json:"device,omitempty"`
}
