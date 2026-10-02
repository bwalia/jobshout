package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/appleauth"
	"github.com/jobshout/server/internal/model"
)

// appleNonceIntent marks an auth_oauth_states row as a Sign in with Apple
// nonce rather than Google CSRF state. The table already gives us what a nonce
// needs (random key, expiry, delete-on-consume), so the two share it.
const appleNonceIntent = "apple"

const identityProviderApple = "apple"

// authResponseForDevice issues tokens bound to the install the client named.
// A device that can't be recorded degrades to an unbound session rather than
// failing a sign-in whose credentials were fine.
func (s *authService) authResponseForDevice(ctx context.Context, user *model.User, info *model.DeviceInfo) (*model.AuthResponse, error) {
	deviceID, err := s.resolveDevice(ctx, user.ID, info)
	if err != nil {
		s.logger.Warn("auth: failed to record device; issuing an unbound session",
			zap.String("user_id", user.ID.String()), zap.Error(err))
		deviceID = nil
	}
	return s.issueTokens(ctx, user, deviceID)
}

// resolveDevice returns the device row for this sign-in: the one the client
// named if it is this user's, else a new one. A device id belonging to
// someone else is ignored, not reused.
func (s *authService) resolveDevice(ctx context.Context, userID uuid.UUID, info *model.DeviceInfo) (*uuid.UUID, error) {
	if s.mobile.Devices == nil || info == nil {
		return nil, nil
	}
	d := &model.Device{
		UserID:     userID,
		Platform:   strings.ToLower(strings.TrimSpace(info.Platform)),
		Name:       clipString(strings.TrimSpace(info.Name), 255),
		AppVersion: clipString(strings.TrimSpace(info.AppVersion), 64),
	}
	if id, err := uuid.Parse(strings.TrimSpace(info.ID)); err == nil {
		existing, err := s.mobile.Devices.FindForUser(ctx, id, userID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			d.ID = existing.ID
			if err := s.mobile.Devices.Touch(ctx, d); err != nil {
				return nil, err
			}
			return &d.ID, nil
		}
	}
	d.ID = uuid.New()
	if err := s.mobile.Devices.Create(ctx, d); err != nil {
		return nil, err
	}
	return &d.ID, nil
}

func (s *authService) touchDevice(ctx context.Context, userID, deviceID uuid.UUID) {
	if s.mobile.Devices == nil {
		return
	}
	d, err := s.mobile.Devices.FindForUser(ctx, deviceID, userID)
	if err != nil || d == nil {
		return
	}
	if err := s.mobile.Devices.Touch(ctx, d); err != nil {
		s.logger.Debug("auth: touch device failed", zap.Error(err))
	}
}

// Logout revokes the session the refresh token belongs to. For a device-bound
// token that is the whole device (its push token goes too); for a web token,
// just that token. An unknown token is not an error: the client is signed out
// either way.
func (s *authService) Logout(ctx context.Context, refreshToken string) error {
	stored, err := s.tokenRepo.FindByHash(ctx, s.jwtSvc.HashToken(refreshToken))
	if err != nil {
		return fmt.Errorf("finding refresh token: %w", err)
	}
	if stored == nil {
		return nil
	}
	if stored.DeviceID != nil && s.mobile.Devices != nil {
		if _, err := s.mobile.Devices.Delete(ctx, *stored.DeviceID, stored.UserID); err != nil {
			return err
		}
	}
	return s.tokenRepo.Delete(ctx, stored.ID)
}

// LogoutAll revokes every refresh token and device the user has. Access
// tokens already issued stay valid until they expire (15 minutes by default).
func (s *authService) LogoutAll(ctx context.Context, userID uuid.UUID) error {
	if s.mobile.Devices != nil {
		if err := s.mobile.Devices.DeleteAllForUser(ctx, userID); err != nil {
			return err
		}
	}
	return s.tokenRepo.DeleteAllForUser(ctx, userID)
}

func (s *authService) ListDevices(ctx context.Context, userID uuid.UUID) ([]model.Device, error) {
	if s.mobile.Devices == nil {
		return []model.Device{}, nil
	}
	return s.mobile.Devices.ListByUser(ctx, userID)
}

func (s *authService) RevokeDevice(ctx context.Context, userID, deviceID uuid.UUID) error {
	if s.mobile.Devices == nil {
		return ErrDeviceNotFound
	}
	ok, err := s.mobile.Devices.Delete(ctx, deviceID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrDeviceNotFound
	}
	return nil
}

func (s *authService) SetDevicePush(ctx context.Context, userID, deviceID uuid.UUID, req model.DevicePushRequest) error {
	if s.mobile.Devices == nil {
		return ErrDeviceNotFound
	}
	token := strings.TrimSpace(req.Token)
	env := req.Environment
	if env == "" {
		env = "production"
	}
	ok, err := s.mobile.Devices.SetPush(ctx, deviceID, userID, token, env)
	if err != nil {
		return err
	}
	if !ok {
		return ErrDeviceNotFound
	}
	return nil
}

func (s *authService) AppleEnabled() bool {
	return s.mobile.Apple != nil && s.mobile.Identities != nil
}

// AppleNonce issues a one-time nonce, valid for ten minutes. The app sends
// SHA-256(nonce) to Apple and the raw nonce back to SignInWithApple.
func (s *authService) AppleNonce(ctx context.Context) (string, time.Time, error) {
	if !s.AppleEnabled() {
		return "", time.Time{}, ErrAppleAuthNotConfigured
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, fmt.Errorf("apple nonce: %w", err)
	}
	nonce := hex.EncodeToString(buf)
	expires := time.Now().Add(10 * time.Minute)
	if err := s.userRepo.PutGoogleOAuthState(ctx, &model.GoogleOAuthState{
		State:     nonce,
		Intent:    appleNonceIntent,
		ExpiresAt: expires,
	}); err != nil {
		return "", time.Time{}, err
	}
	return nonce, expires, nil
}

// SignInWithApple signs a user in (or up) from a native Apple identity token.
//
// Lookup order: the linked Apple subject; else an existing user with the same
// verified email, which gets the subject linked; else a new user and
// organization. Apple only shares the email on the first authorization, so a
// first sign-in without one is refused rather than creating an account we
// could never match again.
func (s *authService) SignInWithApple(ctx context.Context, req model.AppleSignInRequest) (*model.AuthResponse, error) {
	if !s.AppleEnabled() {
		return nil, ErrAppleAuthNotConfigured
	}

	// Burn the nonce before verifying, so a token that fails verification
	// cannot be retried against the same nonce.
	st, err := s.userRepo.ConsumeGoogleOAuthState(ctx, req.Nonce)
	if err != nil {
		return nil, err
	}
	if st == nil || st.Intent != appleNonceIntent {
		return nil, ErrInvalidAppleToken
	}

	id, err := s.mobile.Apple.Verify(ctx, req.IdentityToken, req.Nonce)
	if err != nil {
		if errors.Is(err, appleauth.ErrInvalidToken) {
			s.logger.Info("apple sign-in: token rejected", zap.Error(err))
			return nil, ErrInvalidAppleToken
		}
		return nil, fmt.Errorf("verifying apple token: %w", err)
	}

	user, err := s.userFromApple(ctx, id, req)
	if err != nil {
		return nil, err
	}
	return s.authResponseForDevice(ctx, user, req.Device)
}

func (s *authService) userFromApple(ctx context.Context, id appleauth.Identity, req model.AppleSignInRequest) (*model.User, error) {
	userID, err := s.mobile.Identities.FindUserID(ctx, identityProviderApple, id.Subject)
	if err != nil {
		return nil, err
	}
	if userID != uuid.Nil {
		user, err := s.userRepo.FindByID(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("finding user: %w", err)
		}
		if user == nil {
			return nil, ErrUserNotFound
		}
		return user, nil
	}

	if id.Email == "" || !id.EmailVerified {
		return nil, ErrAppleEmailRequired
	}

	user, err := s.userRepo.FindByEmailFold(ctx, id.Email)
	if err != nil {
		return nil, fmt.Errorf("finding user by email: %w", err)
	}
	if user == nil {
		user, err = s.registerExternalUser(ctx, id.Email, req.FullName, req.OrgName, nil, nil)
		if err != nil {
			return nil, err
		}
	}
	if err := s.mobile.Identities.Link(ctx, identityProviderApple, id.Subject, user.ID, id.Email); err != nil {
		return nil, err
	}
	return user, nil
}
