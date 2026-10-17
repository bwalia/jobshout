package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/integration"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type IntegrationService interface {
	Create(ctx context.Context, orgID, createdBy uuid.UUID, req model.CreateIntegrationRequest) (*model.Integration, error)
	Get(ctx context.Context, id uuid.UUID) (*model.Integration, error)
	List(ctx context.Context, orgID uuid.UUID) ([]model.Integration, error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateIntegrationRequest) (*model.Integration, error)
	Delete(ctx context.Context, id uuid.UUID) error

	LinkTask(ctx context.Context, integrationID, taskID uuid.UUID, direction string) (*model.IntegrationTaskLink, error)
	UnlinkTask(ctx context.Context, integrationID, taskID uuid.UUID) error
	ListLinks(ctx context.Context, integrationID uuid.UUID) ([]model.IntegrationTaskLink, error)
	SyncLink(ctx context.Context, linkID uuid.UUID, direction string) error
	ListSyncLogs(ctx context.Context, integrationID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.IntegrationSyncLog], error)
}

// ErrSecretsNotSealed refuses a new credential on a server that cannot
// encrypt it (no SECRETS_ENCRYPTION_KEY): it would be stored in plaintext.
var ErrSecretsNotSealed = errors.New("this server has no SECRETS_ENCRYPTION_KEY, so it cannot store keys encrypted; ask an operator to set it, then try again")

// ErrInvalidIntegration wraps a payload the provider cannot work with.
var ErrInvalidIntegration = errors.New("invalid integration")

// opsapiNamespace is an OpsAPI namespace slug.
var opsapiNamespace = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type integrationService struct {
	integRepo   repository.IntegrationRepository
	linkRepo    repository.TaskLinkRepository
	syncLogRepo repository.SyncLogRepository
	registry    *integration.Registry
	logger      *zap.Logger
	// sealsSecrets is whether the repository encrypts credentials at rest.
	// Without it, new credentials are refused rather than stored in plaintext.
	sealsSecrets bool
}

func NewIntegrationService(
	integRepo repository.IntegrationRepository,
	linkRepo repository.TaskLinkRepository,
	syncLogRepo repository.SyncLogRepository,
	registry *integration.Registry,
	logger *zap.Logger,
	sealsSecrets bool,
) IntegrationService {
	return &integrationService{
		integRepo:    integRepo,
		linkRepo:     linkRepo,
		syncLogRepo:  syncLogRepo,
		registry:     registry,
		logger:       logger,
		sealsSecrets: sealsSecrets,
	}
}

// newCredential reports whether v is a credential value the caller is setting,
// as opposed to blank or the masked value the API showed ("••••abcd").
func newCredential(v any) bool {
	str, ok := v.(string)
	return v != nil && (!ok || (strings.TrimSpace(str) != "" && !strings.HasPrefix(str, "••••")))
}

// checkSealing refuses creds that set a value this server would store in
// plaintext.
func (s *integrationService) checkSealing(creds map[string]any) error {
	if s.sealsSecrets {
		return nil
	}
	for _, v := range creds {
		if newCredential(v) {
			return ErrSecretsNotSealed
		}
	}
	return nil
}

// validateOpsAPI checks the parts of an opsapi integration an article is
// filed with. Only what a request sets is checked, so rows written before
// this check still save a rename or a status change.
func validateOpsAPI(baseURL *string, config map[string]any, creds map[string]any, creating bool) error {
	if baseURL != nil || creating {
		b := ""
		if baseURL != nil {
			b = strings.TrimSpace(*baseURL)
		}
		if u, err := url.Parse(b); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fmt.Errorf("%w: base_url must be a full http(s) OpsAPI address", ErrInvalidIntegration)
		}
	}
	if config != nil || creating {
		ns, _ := config["namespace_slug"].(string)
		if !opsapiNamespace.MatchString(strings.TrimSpace(ns)) {
			return fmt.Errorf("%w: config.namespace_slug must be an OpsAPI namespace slug (lower-case letters, digits and hyphens)", ErrInvalidIntegration)
		}
	}
	if creating {
		if key, _ := creds["api_key"].(string); strings.TrimSpace(key) == "" {
			return fmt.Errorf("%w: credentials.api_key is required", ErrInvalidIntegration)
		}
	}
	return nil
}

func (s *integrationService) Create(ctx context.Context, orgID, createdBy uuid.UUID, req model.CreateIntegrationRequest) (*model.Integration, error) {
	if req.Provider == model.ProviderOpsAPI {
		if err := validateOpsAPI(&req.BaseURL, req.Config, req.Credentials, true); err != nil {
			return nil, err
		}
	}
	if err := s.checkSealing(req.Credentials); err != nil {
		return nil, err
	}
	i := &model.Integration{
		OrgID:       orgID,
		Name:        req.Name,
		Provider:    req.Provider,
		BaseURL:     req.BaseURL,
		Credentials: req.Credentials,
		Config:      req.Config,
		Status:      "active",
		CreatedBy:   &createdBy,
	}
	if i.Config == nil {
		i.Config = map[string]any{}
	}
	if i.Credentials == nil {
		i.Credentials = map[string]any{}
	}

	if err := s.integRepo.Create(ctx, i); err != nil {
		return nil, fmt.Errorf("create integration: %w", err)
	}
	return i, nil
}

func (s *integrationService) Get(ctx context.Context, id uuid.UUID) (*model.Integration, error) {
	i, err := s.integRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if i == nil {
		return nil, fmt.Errorf("integration not found")
	}
	return i, nil
}

func (s *integrationService) List(ctx context.Context, orgID uuid.UUID) ([]model.Integration, error) {
	return s.integRepo.ListByOrg(ctx, orgID)
}

func (s *integrationService) Update(ctx context.Context, id uuid.UUID, req model.UpdateIntegrationRequest) (*model.Integration, error) {
	i, err := s.integRepo.FindByID(ctx, id)
	if err != nil || i == nil {
		return nil, fmt.Errorf("integration not found")
	}
	if i.Provider == model.ProviderOpsAPI {
		if err := validateOpsAPI(req.BaseURL, req.Config, nil, false); err != nil {
			return nil, err
		}
	}
	if err := s.checkSealing(req.Credentials); err != nil {
		return nil, err
	}

	if req.Name != nil {
		i.Name = *req.Name
	}
	if req.BaseURL != nil {
		i.BaseURL = *req.BaseURL
	}
	// Merge, never replace: the API only ever shows masked secrets, so a key
	// sent blank (or left out) keeps its stored value.
	if i.Credentials == nil {
		i.Credentials = map[string]any{}
	}
	for k, v := range req.Credentials {
		// A masked value ("••••abcd") is what the API showed, echoed back.
		if !newCredential(v) {
			continue
		}
		i.Credentials[k] = v
	}
	if req.Config != nil {
		i.Config = req.Config
	}
	if req.Status != nil {
		i.Status = *req.Status
	}

	if err := s.integRepo.Update(ctx, i); err != nil {
		return nil, err
	}
	return i, nil
}

func (s *integrationService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.integRepo.Delete(ctx, id)
}

func (s *integrationService) LinkTask(ctx context.Context, integrationID, taskID uuid.UUID, direction string) (*model.IntegrationTaskLink, error) {
	if direction == "" {
		direction = "bidirectional"
	}

	integ, err := s.integRepo.FindByID(ctx, integrationID)
	if err != nil || integ == nil {
		return nil, fmt.Errorf("integration not found")
	}

	adapter, err := s.registry.GetTask(*integ)
	if err != nil {
		return nil, fmt.Errorf("get adapter: %w", err)
	}

	// Create the external issue
	issue := integration.ExternalIssue{
		Title:       fmt.Sprintf("Jobshout Task %s", taskID.String()[:8]),
		Description: "Linked from Jobshout",
	}

	start := time.Now()
	externalID, externalURL, err := adapter.CreateIssue(ctx, issue)
	duration := int(time.Since(start).Milliseconds())

	// Log the sync operation
	logStatus := "success"
	var errMsg *string
	if err != nil {
		logStatus = "failed"
		e := err.Error()
		errMsg = &e
	}
	_ = s.syncLogRepo.Append(ctx, &model.IntegrationSyncLog{
		IntegrationID: integrationID,
		Direction:     "push",
		Status:        logStatus,
		ErrorMessage:  errMsg,
		DurationMs:    &duration,
	})

	if err != nil {
		return nil, fmt.Errorf("create external issue: %w", err)
	}

	link := &model.IntegrationTaskLink{
		IntegrationID: integrationID,
		TaskID:        taskID,
		ExternalID:    externalID,
		ExternalURL:   &externalURL,
		SyncDirection: direction,
		SyncStatus:    "synced",
	}
	now := time.Now()
	link.LastSyncedAt = &now

	if err := s.linkRepo.Create(ctx, link); err != nil {
		return nil, fmt.Errorf("create link: %w", err)
	}
	return link, nil
}

func (s *integrationService) UnlinkTask(ctx context.Context, integrationID, taskID uuid.UUID) error {
	return s.linkRepo.DeleteByTaskAndIntegration(ctx, integrationID, taskID)
}

func (s *integrationService) ListLinks(ctx context.Context, integrationID uuid.UUID) ([]model.IntegrationTaskLink, error) {
	return s.linkRepo.ListByIntegration(ctx, integrationID)
}

func (s *integrationService) SyncLink(ctx context.Context, linkID uuid.UUID, direction string) error {
	// This would be expanded with the full sync engine
	s.logger.Info("sync link requested", zap.String("link_id", linkID.String()), zap.String("direction", direction))
	return nil
}

func (s *integrationService) ListSyncLogs(ctx context.Context, integrationID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.IntegrationSyncLog], error) {
	return s.syncLogRepo.ListByIntegration(ctx, integrationID, params)
}
