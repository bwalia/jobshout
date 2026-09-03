package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/googleauth"
	"github.com/jobshout/server/internal/model"
)

func (s *authService) GoogleEnabled() bool {
	return s.google != nil && s.googleCfg.Configured()
}

func (s *authService) StartGoogle(ctx context.Context, intent, orgName string) (string, error) {
	if !s.GoogleEnabled() {
		return "", ErrGoogleAuthNotConfigured
	}
	intent = normalizeGoogleIntent(intent)
	orgName = clipString(strings.TrimSpace(orgName), 255)

	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("google oauth state: %w", err)
	}
	state := hex.EncodeToString(buf)
	st := &model.GoogleOAuthState{
		State:     state,
		Intent:    intent,
		OrgName:   orgName,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	if err := s.userRepo.PutGoogleOAuthState(ctx, st); err != nil {
		return "", err
	}
	return s.google.AuthURL(state), nil
}

// AbandonGoogle consumes CSRF state after Google returns an error so a later
// retry cannot reuse it, and so the UI can send the user back to signup vs login.
func (s *authService) AbandonGoogle(ctx context.Context, state string) string {
	if strings.TrimSpace(state) == "" {
		return "login"
	}
	st, err := s.userRepo.ConsumeGoogleOAuthState(ctx, state)
	if err != nil || st == nil {
		return "login"
	}
	return normalizeGoogleIntent(st.Intent)
}

func (s *authService) CompleteGoogle(ctx context.Context, state, code string) (ticket, intent string, err error) {
	if !s.GoogleEnabled() {
		return "", "login", ErrGoogleAuthNotConfigured
	}
	st, err := s.userRepo.ConsumeGoogleOAuthState(ctx, state)
	if err != nil {
		return "", "login", err
	}
	if st == nil {
		return "", "login", ErrInvalidGoogleState
	}
	intent = normalizeGoogleIntent(st.Intent)

	profile, err := s.google.ProfileFromCode(ctx, code)
	if err != nil {
		if errors.Is(err, googleauth.ErrEmailNotVerified) {
			return "", intent, ErrGoogleEmailNotVerified
		}
		s.logger.Warn("google oauth: profile exchange failed", zap.Error(err))
		return "", intent, fmt.Errorf("google sign-in failed")
	}

	user, err := s.userFromGoogleProfile(ctx, profile, st.OrgName)
	if err != nil {
		return "", intent, err
	}

	tbuf := make([]byte, 32)
	if _, err := rand.Read(tbuf); err != nil {
		return "", intent, fmt.Errorf("google oauth ticket: %w", err)
	}
	ticket = hex.EncodeToString(tbuf)
	if err := s.userRepo.PutGoogleOAuthTicket(ctx, &model.GoogleOAuthTicket{
		Ticket:    ticket,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(2 * time.Minute),
	}); err != nil {
		return "", intent, err
	}
	return ticket, intent, nil
}

func (s *authService) ExchangeGoogleTicket(ctx context.Context, ticket string) (*model.AuthResponse, error) {
	if strings.TrimSpace(ticket) == "" {
		return nil, ErrInvalidGoogleTicket
	}
	stored, err := s.userRepo.ConsumeGoogleOAuthTicket(ctx, ticket)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, ErrInvalidGoogleTicket
	}
	user, err := s.userRepo.FindByID(ctx, stored.UserID)
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return s.generateAuthResponse(ctx, user)
}

func (s *authService) userFromGoogleProfile(ctx context.Context, p googleauth.Profile, orgName string) (*model.User, error) {
	avatar := optionalClipped(p.Picture, 500)

	if existing, err := s.userRepo.FindByGoogleSub(ctx, p.Sub); err != nil {
		return nil, fmt.Errorf("finding user by google_sub: %w", err)
	} else if existing != nil {
		s.maybeRefreshGoogleProfile(ctx, existing, p, avatar)
		return existing, nil
	}

	if existing, err := s.userRepo.FindByEmailFold(ctx, p.Email); err != nil {
		return nil, fmt.Errorf("finding user by email: %w", err)
	} else if existing != nil {
		if err := s.userRepo.LinkGoogleSub(ctx, existing.ID, p.Sub, avatar); err != nil {
			return nil, err
		}
		existing.GoogleSub = &p.Sub
		if existing.AvatarURL == nil || *existing.AvatarURL == "" {
			existing.AvatarURL = avatar
		}
		return existing, nil
	}

	return s.registerGoogleUser(ctx, p, orgName, avatar)
}

func (s *authService) maybeRefreshGoogleProfile(ctx context.Context, user *model.User, p googleauth.Profile, avatar *string) {
	changed := false
	if p.Name != "" && user.FullName != p.Name {
		user.FullName = clipString(p.Name, 255)
		changed = true
	}
	if avatar != nil && (user.AvatarURL == nil || *user.AvatarURL == "") {
		user.AvatarURL = avatar
		changed = true
	}
	if !changed {
		return
	}
	if err := s.userRepo.UpdateProfile(ctx, user); err != nil {
		s.logger.Warn("google oauth: failed to refresh profile",
			zap.String("user_id", user.ID.String()), zap.Error(err))
	}
}

func (s *authService) registerGoogleUser(ctx context.Context, p googleauth.Profile, orgName string, avatar *string) (*model.User, error) {
	name := clipString(p.Name, 255)
	if name == "" {
		name = strings.Split(p.Email, "@")[0]
	}
	if strings.TrimSpace(orgName) == "" {
		orgName = name + "'s workspace"
	}
	orgName = clipString(orgName, 255)

	org := &model.Organization{
		ID:   uuid.New(),
		Name: orgName,
		Slug: s.uniqueOrgSlug(ctx, orgName),
	}
	if err := s.orgRepo.Create(ctx, org); err != nil {
		return nil, fmt.Errorf("creating organization: %w", err)
	}

	sub := p.Sub
	user := &model.User{
		ID:        uuid.New(),
		Email:     p.Email,
		Password:  "",
		FullName:  name,
		AvatarURL: avatar,
		Role:      "admin",
		OrgID:     &org.ID,
		GoogleSub: &sub,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}

	org.OwnerID = &user.ID
	s.seedOwnerRole(ctx, org.ID, user.ID)
	s.seedBuiltinAgents(ctx, org.ID, user.ID)
	return user, nil
}

func (s *authService) uniqueOrgSlug(ctx context.Context, name string) string {
	base := slugify(name)
	if base == "" {
		base = "workspace"
	}
	slug := base
	for i := 0; i < 8; i++ {
		existing, err := s.orgRepo.FindBySlug(ctx, slug)
		if err != nil {
			s.logger.Warn("google oauth: slug lookup failed", zap.Error(err))
			return base + "-" + uuid.New().String()[:8]
		}
		if existing == nil {
			return slug
		}
		slug = base + "-" + uuid.New().String()[:8]
	}
	return slug
}

func normalizeGoogleIntent(intent string) string {
	if strings.EqualFold(strings.TrimSpace(intent), "signup") {
		return "signup"
	}
	return "login"
}

func clipString(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}

func optionalClipped(s string, max int) *string {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > max {
		return nil
	}
	return &s
}
