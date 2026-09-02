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
}

type authService struct {
	userRepo  repository.UserRepository
	tokenRepo repository.TokenRepository
	orgRepo   repository.OrganizationRepository
	agentRepo repository.AgentRepository
	rbacRepo  repository.RBACRepository
	jwtSvc    JWTService
	logger    *zap.Logger
}

// NewAuthService creates a new AuthService. agentRepo is used to give each new
// organization its built-in agents; rbacRepo to seed its system roles and make
// the creator an admin. Pass nil to skip either seeding.
func NewAuthService(
	userRepo repository.UserRepository,
	tokenRepo repository.TokenRepository,
	orgRepo repository.OrganizationRepository,
	agentRepo repository.AgentRepository,
	rbacRepo repository.RBACRepository,
	jwtSvc JWTService,
	logger *zap.Logger,
) AuthService {
	return &authService{
		userRepo:  userRepo,
		tokenRepo: tokenRepo,
		orgRepo:   orgRepo,
		agentRepo: agentRepo,
		rbacRepo:  rbacRepo,
		jwtSvc:    jwtSvc,
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

	return s.generateAuthResponse(ctx, user)
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
// Migration 000019 seeds the same agents for organizations that already
// existed; this covers everything created since the last boot. Failures are
// logged and swallowed — a missing built-in is a degraded experience, not a
// reason to fail a registration that has already created the org and user.
func (s *authService) seedBuiltinAgents(ctx context.Context, orgID, createdBy uuid.UUID) {
	if s.agentRepo == nil {
		return
	}

	// Each built-in is seeded independently so one failing does not deprive the
	// organization of the others.
	seeds := map[string]*model.Agent{
		"Article Writer":  articleWriterSeed(orgID),
		"Research Agent":  researcherSeed(orgID),
		"Security Tester": pentestSeed(orgID),
		"PR Reviewer":     prReviewerSeed(orgID),
		"Mail Agent":      mailAgentSeed(orgID),
		"Image Generator": imagesAgentSeed(orgID),
		"Career Agent":    careerOpsSeed(orgID),
	}
	seeded := 0
	for name, agent := range seeds {
		agent.CreatedBy = &createdBy
		if err := s.agentRepo.Create(ctx, agent); err != nil {
			s.logger.Warn("auth: failed to seed built-in agent",
				zap.String("agent", name),
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

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.generateAuthResponse(ctx, user)
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

	return s.generateAuthResponse(ctx, user)
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
	}
	if err := s.tokenRepo.Save(ctx, storedToken); err != nil {
		return nil, fmt.Errorf("saving refresh token: %w", err)
	}

	return &model.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: plainRefresh,
		User:         *user,
	}, nil
}

var nonAlphaRegex = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	slug := strings.ToLower(name)
	slug = nonAlphaRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
}

func pentestSeed(orgID uuid.UUID) *model.Agent {
	desc := "Autonomous security testing agent powered by Strix. Tests live APIs, applications, and codebases for vulnerabilities including OWASP Top 10 and beyond."
	prompt := "You are a security expert assisting with penetration testing. Use the Strix tool to run security scans against applications and APIs. Report findings clearly with severity levels and proof-of-concepts."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         "Security Tester",
		Role:         "Penetration Testing Agent",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinPentester},
	}
}

func prReviewerSeed(orgID uuid.UUID) *model.Agent {
	desc := "Reviews GitHub pull requests with a local coder model via OpenCode: explores the repo around the diff, then posts MERGE or FIX."
	prompt := "You are a senior engineer reviewing pull requests. Use the review_pull_request tool with a repo slug (owner/name) and PR number. It posts the review to the PR by default; pass dry_run=true only if the user explicitly asks for a preview that posts nothing. Summarise the verdict and blocking findings first."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         "PR Reviewer",
		Role:         "Pull Request Review Agent",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinPRReviewer},
	}
}

// Sentinel errors for auth operations.
var (
	ErrEmailAlreadyExists  = authError("email already exists")
	ErrInvalidCredentials  = authError("invalid email or password")
	ErrInvalidRefreshToken = authError("invalid refresh token")
	ErrRefreshTokenExpired = authError("refresh token expired")
	ErrUserNotFound        = authError("user not found")
)

type authError string

func (e authError) Error() string {
	return string(e)
}
