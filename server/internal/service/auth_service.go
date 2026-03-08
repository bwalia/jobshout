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
}

type authService struct {
	userRepo  repository.UserRepository
	tokenRepo repository.TokenRepository
	orgRepo   repository.OrganizationRepository
	jwtSvc    JWTService
	logger    *zap.Logger
}

// NewAuthService creates a new AuthService.
func NewAuthService(
	userRepo repository.UserRepository,
	tokenRepo repository.TokenRepository,
	orgRepo repository.OrganizationRepository,
	jwtSvc JWTService,
	logger *zap.Logger,
) AuthService {
	return &authService{
		userRepo:  userRepo,
		tokenRepo: tokenRepo,
		orgRepo:   orgRepo,
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

	return s.generateAuthResponse(ctx, user)
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
