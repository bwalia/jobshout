package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/appleauth"
	"github.com/jobshout/server/internal/googleauth"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// AuthService handles user authentication business logic.
type AuthService interface {
	Register(ctx context.Context, req model.RegisterRequest) (*model.AuthResponse, error)
	Login(ctx context.Context, req model.LoginRequest) (*model.AuthResponse, error)
	RefreshToken(ctx context.Context, refreshToken string) (*model.AuthResponse, error)
	GetMe(ctx context.Context, userID uuid.UUID) (*model.User, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, req model.UpdateProfileRequest) (*model.User, error)
	GoogleEnabled() bool
	StartGoogle(ctx context.Context, intent, orgName string, native bool) (authURL string, err error)
	AbandonGoogle(ctx context.Context, state string) (intent string, native bool)
	CompleteGoogle(ctx context.Context, state, code string) (ticket, intent string, native bool, err error)
	ExchangeGoogleTicket(ctx context.Context, ticket string, device *model.DeviceInfo) (*model.AuthResponse, error)

	// Native clients (auth_service_mobile.go).
	Logout(ctx context.Context, refreshToken string) error
	LogoutAll(ctx context.Context, userID uuid.UUID) error
	ListDevices(ctx context.Context, userID uuid.UUID) ([]model.Device, error)
	RevokeDevice(ctx context.Context, userID, deviceID uuid.UUID) error
	SetDevicePush(ctx context.Context, userID, deviceID uuid.UUID, req model.DevicePushRequest) error
	AppleEnabled() bool
	AppleNonce(ctx context.Context) (nonce string, expiresAt time.Time, err error)
	SignInWithApple(ctx context.Context, req model.AppleSignInRequest) (*model.AuthResponse, error)
}

// MobileAuth is what native sign-in needs on top of the web flows. Any nil
// field disables the feature that needs it: without Devices, sign-ins are not
// device-bound; without Identities or Apple, Sign in with Apple is off.
type MobileAuth struct {
	Devices    repository.DeviceRepository
	Identities repository.IdentityRepository
	Apple      appleauth.Verifier
}

type authService struct {
	userRepo  repository.UserRepository
	tokenRepo repository.TokenRepository
	orgRepo   repository.OrganizationRepository
	agentRepo repository.AgentRepository
	rbacRepo  repository.RBACRepository
	jwtSvc    JWTService
	google    googleauth.Identity
	googleCfg googleauth.Config
	mobile    MobileAuth
	logger    *zap.Logger
}

// NewAuthService creates a new AuthService. agentRepo is used to give each new
// organization its built-in agents; rbacRepo to seed its system roles and make
// the creator an admin. Pass nil to skip either seeding. google may be nil when
// Google login is not configured.
func NewAuthService(
	userRepo repository.UserRepository,
	tokenRepo repository.TokenRepository,
	orgRepo repository.OrganizationRepository,
	agentRepo repository.AgentRepository,
	rbacRepo repository.RBACRepository,
	jwtSvc JWTService,
	google googleauth.Identity,
	googleCfg googleauth.Config,
	mobile MobileAuth,
	logger *zap.Logger,
) AuthService {
	return &authService{
		userRepo:  userRepo,
		tokenRepo: tokenRepo,
		orgRepo:   orgRepo,
		agentRepo: agentRepo,
		rbacRepo:  rbacRepo,
		jwtSvc:    jwtSvc,
		google:    google,
		googleCfg: googleCfg,
		mobile:    mobile,
		logger:    logger,
	}
}

func (s *authService) Register(ctx context.Context, req model.RegisterRequest) (*model.AuthResponse, error) {
	existing, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("checking existing user: %w", err)
	}
	if existing != nil {
		return nil, ErrEmailAlreadyExists
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	// Create the organization first
	org := &model.Organization{
		ID:   uuid.New(),
		Name: req.OrgName,
		Slug: slugify(req.OrgName),
	}
	if err := s.orgRepo.Create(ctx, org); err != nil {
		return nil, fmt.Errorf("creating organization: %w", err)
	}

	// Create the user
	user := &model.User{
		ID:       uuid.New(),
		Email:    req.Email,
		Password: string(hashedPassword),
		FullName: req.FullName,
		Role:     "admin",
		OrgID:    &org.ID,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}

	// Set the org owner to this user
	org.OwnerID = &user.ID

	s.seedOwnerRole(ctx, org.ID, user.ID)
	s.seedBuiltinAgents(ctx, org.ID, user.ID)

	return s.authResponseForDevice(ctx, user, req.Device)
}

// seedOwnerRole creates the organization's system roles and grants the
// creating user the admin one. Without it a fresh org has no RBAC rows at all,
// so every permission-guarded surface — most visibly the chat agent's tool
// guard — denies its own owner. Failures are logged and swallowed for the same
// reason seedBuiltinAgents' are: the org and user already exist.
func (s *authService) seedOwnerRole(ctx context.Context, orgID, userID uuid.UUID) {
	if s.rbacRepo == nil {
		return
	}
	if err := s.rbacRepo.EnsureSystemRoles(ctx, orgID); err != nil {
		s.logger.Warn("auth: failed to seed system roles",
			zap.String("org_id", orgID.String()), zap.Error(err))
		return
	}
	role, err := s.rbacRepo.GetRoleByName(ctx, orgID, model.RoleAdmin)
	if err != nil || role == nil {
		s.logger.Warn("auth: admin role missing after seeding",
			zap.String("org_id", orgID.String()), zap.Error(err))
		return
	}
	if err := s.rbacRepo.AssignRole(ctx, &model.UserRole{
		UserID: userID, RoleID: role.ID, OrgID: orgID, GrantedBy: &userID,
	}); err != nil {
		s.logger.Warn("auth: failed to grant admin role",
			zap.String("org_id", orgID.String()), zap.Error(err))
	}
}

// seedBuiltinAgents gives a brand-new organization the platform's built-in
// agents, so the dashboard is not empty on first login.
//
// All specialists are wired this way: Seed lives on the module. Iterate the
// registry. A new agent does not need a row here — register it, do not add a
// switch. Migration 000019 seeds the same agents for organizations that already
// existed; this covers everything created since the last boot. Failures are
// logged and swallowed — a missing built-in is a degraded experience, not a
// reason to fail a registration that has already created the org and user.
func (s *authService) seedBuiltinAgents(ctx context.Context, orgID, createdBy uuid.UUID) {
	if s.agentRepo == nil {
		return
	}

	seeded := 0
	for _, m := range agentmodule.All() {
		if m.Seed == nil {
			continue
		}
		agent := m.Seed(orgID)
		if agent == nil {
			continue
		}
		agent.CreatedBy = &createdBy
		if err := s.agentRepo.Create(ctx, agent); err != nil {
			s.logger.Warn("auth: failed to seed built-in agent",
				zap.String("agent", agent.Name),
				zap.String("org_id", orgID.String()), zap.Error(err))
			continue
		}
		seeded++
	}
	s.logger.Info("auth: seeded built-in agents for new organization",
		zap.String("org_id", orgID.String()), zap.Int("agents", seeded))
}

func (s *authService) Login(ctx context.Context, req model.LoginRequest) (*model.AuthResponse, error) {
	user, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	// Google-only accounts have no password hash. Do not bcrypt an empty string.
	if user.Password == "" {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.authResponseForDevice(ctx, user, req.Device)
}

func (s *authService) RefreshToken(ctx context.Context, refreshToken string) (*model.AuthResponse, error) {
	tokenHash := s.jwtSvc.HashToken(refreshToken)

	stored, err := s.tokenRepo.FindByHash(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("finding refresh token: %w", err)
	}
	if stored == nil {
		return nil, ErrInvalidRefreshToken
	}

	if time.Now().After(stored.ExpiresAt) {
		if deleteErr := s.tokenRepo.Delete(ctx, stored.ID); deleteErr != nil {
			s.logger.Error("failed to delete expired refresh token", zap.Error(deleteErr))
		}
		return nil, ErrRefreshTokenExpired
	}

	// Delete the old refresh token (rotation)
	if err := s.tokenRepo.Delete(ctx, stored.ID); err != nil {
		return nil, fmt.Errorf("deleting old refresh token: %w", err)
	}

	user, err := s.userRepo.FindByID(ctx, stored.UserID)
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}
	if user == nil {
		return nil, ErrInvalidRefreshToken
	}

	// The rotated token stays bound to the same install.
	if stored.DeviceID != nil {
		s.touchDevice(ctx, user.ID, *stored.DeviceID)
	}
	return s.issueTokens(ctx, user, stored.DeviceID)
}

func (s *authService) GetMe(ctx context.Context, userID uuid.UUID) (*model.User, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (s *authService) UpdateProfile(ctx context.Context, userID uuid.UUID, req model.UpdateProfileRequest) (*model.User, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	if req.FullName != nil {
		user.FullName = *req.FullName
	}
	if req.AvatarURL != nil {
		user.AvatarURL = req.AvatarURL
	}

	if err := s.userRepo.UpdateProfile(ctx, user); err != nil {
		return nil, fmt.Errorf("updating profile: %w", err)
	}
	return user, nil
}

func (s *authService) generateAuthResponse(ctx context.Context, user *model.User) (*model.AuthResponse, error) {
	return s.issueTokens(ctx, user, nil)
}

// issueTokens mints an access token and a refresh token bound to deviceID
// (nil for web sessions).
func (s *authService) issueTokens(ctx context.Context, user *model.User, deviceID *uuid.UUID) (*model.AuthResponse, error) {
	accessToken, err := s.jwtSvc.GenerateAccessToken(user.ID, user.Email, user.OrgID, user.Role)
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}

	plainRefresh, refreshHash, expiresAt := s.jwtSvc.GenerateRefreshToken()

	storedToken := &model.RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: refreshHash,
		ExpiresAt: expiresAt,
		DeviceID:  deviceID,
	}
	if err := s.tokenRepo.Save(ctx, storedToken); err != nil {
		return nil, fmt.Errorf("saving refresh token: %w", err)
	}

	return &model.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: plainRefresh,
		User:         *user,
		DeviceID:     deviceID,
	}, nil
}

var nonAlphaRegex = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	slug := strings.ToLower(name)
	slug = nonAlphaRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

// Sentinel errors for auth operations.
var (
	ErrEmailAlreadyExists      = authError("email already exists")
	ErrInvalidCredentials      = authError("invalid email or password")
	ErrInvalidRefreshToken     = authError("invalid refresh token")
	ErrRefreshTokenExpired     = authError("refresh token expired")
	ErrUserNotFound            = authError("user not found")
	ErrGoogleAuthNotConfigured = authError("google sign-in is not configured")
	ErrInvalidGoogleState      = authError("invalid or expired google sign-in state")
	ErrInvalidGoogleTicket     = authError("invalid or expired google sign-in ticket")
	ErrGoogleEmailNotVerified  = authError("google email is not verified")
	ErrAppleAuthNotConfigured  = authError("sign in with apple is not configured")
	ErrInvalidAppleToken       = authError("invalid sign in with apple token")
	ErrAppleEmailRequired      = authError("apple did not share a verified email")
	ErrDeviceNotFound          = authError("device not found")
)

type authError string

func (e authError) Error() string {
	return string(e)
}
