package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// SSOService manages SSO/OIDC authentication flows.
type SSOService interface {
	CreateConfig(ctx context.Context, orgID uuid.UUID, req model.CreateSSOConfigRequest) (*model.SSOConfig, error)
	GetConfig(ctx context.Context, orgID uuid.UUID, provider string) (*model.SSOConfig, error)
	ListConfigs(ctx context.Context, orgID uuid.UUID) ([]model.SSOConfig, error)
	DeleteConfig(ctx context.Context, id uuid.UUID) error
	// GetAuthorizationURL returns the OIDC authorization URL for redirect.
	GetAuthorizationURL(ctx context.Context, orgID uuid.UUID, provider, state string) (string, error)
	// ExchangeCode exchanges an authorization code for user info and returns/creates a user.
	ExchangeCode(ctx context.Context, orgID uuid.UUID, provider, code string) (*model.User, error)
	RecordLogin(ctx context.Context, log *model.LoginAuditLog) error
	ListLoginAudit(ctx context.Context, orgID uuid.UUID, limit int) ([]model.LoginAuditLog, error)
}

type ssoService struct {
	ssoRepo   repository.SSORepository
	userRepo  repository.UserRepository
	rbacRepo  repository.RBACRepository
	auditRepo repository.AuditRepository
	logger    *zap.Logger
}

// NewSSOService creates an SSOService.
func NewSSOService(
	ssoRepo repository.SSORepository,
	userRepo repository.UserRepository,
	rbacRepo repository.RBACRepository,
	auditRepo repository.AuditRepository,
	logger *zap.Logger,
) SSOService {
	return &ssoService{
		ssoRepo:   ssoRepo,
		userRepo:  userRepo,
		rbacRepo:  rbacRepo,
		auditRepo: auditRepo,
		logger:    logger,
	}
}

func (s *ssoService) CreateConfig(ctx context.Context, orgID uuid.UUID, req model.CreateSSOConfigRequest) (*model.SSOConfig, error) {
	cfg := &model.SSOConfig{
		OrgID:         orgID,
		Provider:      req.Provider,
		ClientID:      req.ClientID,
		ClientSecret:  req.ClientSecret,
		IssuerURL:     req.IssuerURL,
		RedirectURL:   req.RedirectURL,
		Scopes:        req.Scopes,
		AutoProvision: true,
		DefaultRole:   "viewer",
		DomainFilter:  req.DomainFilter,
		Enabled:       true,
		Metadata:      map[string]any{},
	}
	if req.AutoProvision != nil {
		cfg.AutoProvision = *req.AutoProvision
	}
	if req.DefaultRole != "" {
		cfg.DefaultRole = req.DefaultRole
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "profile", "email"}
	}
	return s.ssoRepo.Create(ctx, cfg)
}

func (s *ssoService) GetConfig(ctx context.Context, orgID uuid.UUID, provider string) (*model.SSOConfig, error) {
	return s.ssoRepo.GetByOrgAndProvider(ctx, orgID, provider)
}

func (s *ssoService) ListConfigs(ctx context.Context, orgID uuid.UUID) ([]model.SSOConfig, error) {
	return s.ssoRepo.ListByOrg(ctx, orgID)
}

func (s *ssoService) DeleteConfig(ctx context.Context, id uuid.UUID) error {
	return s.ssoRepo.Delete(ctx, id)
}

func (s *ssoService) GetAuthorizationURL(ctx context.Context, orgID uuid.UUID, provider, state string) (string, error) {
	cfg, err := s.ssoRepo.GetByOrgAndProvider(ctx, orgID, provider)
	if err != nil {
		return "", fmt.Errorf("sso: get config: %w", err)
	}
	if cfg == nil {
		return "", fmt.Errorf("sso: no config found for provider %q", provider)
	}

	// Build OIDC authorization URL.
	authURL := strings.TrimSuffix(cfg.IssuerURL, "/") + "/authorize"

	// Azure AD uses /oauth2/v2.0/authorize
	if cfg.Provider == model.SSOProviderAzureAD {
		authURL = strings.TrimSuffix(cfg.IssuerURL, "/") + "/oauth2/v2.0/authorize"
	}

	params := url.Values{
		"client_id":     {cfg.ClientID},
		"redirect_uri":  {cfg.RedirectURL},
		"response_type": {"code"},
		"scope":         {strings.Join(cfg.Scopes, " ")},
		"state":         {state},
	}

	return authURL + "?" + params.Encode(), nil
}

func (s *ssoService) ExchangeCode(ctx context.Context, orgID uuid.UUID, provider, code string) (*model.User, error) {
	cfg, err := s.ssoRepo.GetByOrgAndProvider(ctx, orgID, provider)
	if err != nil || cfg == nil {
		return nil, fmt.Errorf("sso: config not found")
	}

	// Token endpoint
	tokenURL := strings.TrimSuffix(cfg.IssuerURL, "/") + "/token"
	if cfg.Provider == model.SSOProviderAzureAD {
		tokenURL = strings.TrimSuffix(cfg.IssuerURL, "/") + "/oauth2/v2.0/token"
	}

	// Exchange code for tokens.
	tokenData := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {cfg.RedirectURL},
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.PostForm(tokenURL, tokenData)
	if err != nil {
		return nil, fmt.Errorf("sso: token exchange: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sso: token exchange failed: %s", string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("sso: parse token response: %w", err)
	}

	// Fetch user info.
	userInfoURL := strings.TrimSuffix(cfg.IssuerURL, "/") + "/userinfo"
	if cfg.Provider == model.SSOProviderAzureAD {
		userInfoURL = "https://graph.microsoft.com/oidc/userinfo"
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	uiResp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sso: userinfo: %w", err)
	}
	defer uiResp.Body.Close()
	uiBody, _ := io.ReadAll(uiResp.Body)

	var userInfo struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		Sub   string `json:"sub"`
	}
	if err := json.Unmarshal(uiBody, &userInfo); err != nil {
		return nil, fmt.Errorf("sso: parse userinfo: %w", err)
	}

	if userInfo.Email == "" {
		return nil, fmt.Errorf("sso: no email in userinfo")
	}

	// Domain filter check.
	if cfg.DomainFilter != nil && *cfg.DomainFilter != "" {
		parts := strings.SplitN(userInfo.Email, "@", 2)
		if len(parts) != 2 || parts[1] != *cfg.DomainFilter {
			return nil, fmt.Errorf("sso: email domain %q not allowed", parts[1])
		}
	}

	// Find or create user.
	user, err := s.userRepo.FindByEmail(ctx, userInfo.Email)
	if err != nil {
		return nil, fmt.Errorf("sso: lookup user: %w", err)
	}

	if user == nil {
		if !cfg.AutoProvision {
			return nil, fmt.Errorf("sso: user not found and auto-provisioning disabled")
		}
		// Create user with SSO — no password (SSO-only login).
		user = &model.User{
			ID:       uuid.New(),
			Email:    userInfo.Email,
			FullName: userInfo.Name,
			OrgID:    &orgID,
			Role:     cfg.DefaultRole,
		}
		if err := s.userRepo.Create(ctx, user); err != nil {
			return nil, fmt.Errorf("sso: create user: %w", err)
		}
		// Assign default role.
		role, _ := s.rbacRepo.GetRoleByName(ctx, orgID, cfg.DefaultRole)
		if role != nil {
			_ = s.rbacRepo.AssignRole(ctx, &model.UserRole{
				UserID: user.ID, RoleID: role.ID, OrgID: orgID,
			})
		}
	}

	return user, nil
}

func (s *ssoService) RecordLogin(ctx context.Context, log *model.LoginAuditLog) error {
	return s.auditRepo.RecordLogin(ctx, log)
}

func (s *ssoService) ListLoginAudit(ctx context.Context, orgID uuid.UUID, limit int) ([]model.LoginAuditLog, error) {
	return s.auditRepo.ListLogins(ctx, orgID, limit)
}
