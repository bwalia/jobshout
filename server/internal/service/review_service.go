package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/reviewbot"
)

var ErrReviewRunNotFound = errors.New("review run not found")
var ErrReviewNotConfigured = errors.New("PR review is not enabled on this ring")
var ErrReviewRepoNotAllowed = errors.New("repo is not on the review allowlist")

type ReviewService interface {
	CreateRun(ctx context.Context, req model.CreateReviewRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.ReviewRun, error)
	GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.ReviewRun, error)
	ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.ReviewRun], error)
	AllowedRepos() []string
	Enabled() bool
}

type reviewService struct {
	runRepo   repository.ReviewRunRepository
	allowlist []string
	enabled   bool
	logger    *zap.Logger
}

func NewReviewService(
	runRepo repository.ReviewRunRepository,
	cfg reviewbot.Config,
	logger *zap.Logger,
) ReviewService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &reviewService{
		runRepo:   runRepo,
		allowlist: cfg.AllowedRepos,
		enabled:   cfg.Configured(),
		logger:    logger,
	}
}

func (s *reviewService) Enabled() bool          { return s.enabled }
func (s *reviewService) AllowedRepos() []string { return append([]string(nil), s.allowlist...) }

func (s *reviewService) CreateRun(ctx context.Context, req model.CreateReviewRunRequest, orgID uuid.UUID, requestedBy *uuid.UUID) (*model.ReviewRun, error) {
	if !s.enabled {
		return nil, ErrReviewNotConfigured
	}
	repo := strings.TrimSpace(req.Repo)
	if !reviewbot.RepoAllowed(repo, s.allowlist) {
		return nil, fmt.Errorf("%w: %s", ErrReviewRepoNotAllowed, repo)
	}
	dryRun := false
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}
	if !req.Force {
		existing, err := s.runRepo.FindActive(ctx, orgID, repo, req.PRNumber)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}

	now := time.Now()
	// next_poll_at is compared against the database's NOW() (UTC) by the
	// reconciler's claim query, so it is written in UTC — see
	// ReviewReconciler and pentestService.CreateRun for the same invariant.
	dueNow := now.UTC()
	run := &model.ReviewRun{
		ID:          uuid.New(),
		OrgID:       orgID,
		AgentID:     req.AgentID,
		RequestedBy: requestedBy,
		Repo:        repo,
		PRNumber:    req.PRNumber,
		DryRun:      dryRun,
		Force:       req.Force,
		Status:      "queued",
		NextPollAt:  &dueNow,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.runRepo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to create review run: %w", err)
	}
	s.logger.Info("review run queued",
		zap.String("runID", run.ID.String()),
		zap.String("repo", repo),
		zap.Int("pr", req.PRNumber),
		zap.Bool("dry_run", dryRun))
	return run, nil
}

func (s *reviewService) GetRun(ctx context.Context, runID, orgID uuid.UUID) (*model.ReviewRun, error) {
	run, err := s.runRepo.GetByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run == nil || run.OrgID != orgID {
		return nil, ErrReviewRunNotFound
	}
	return run, nil
}

func (s *reviewService) ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.ReviewRun], error) {
	pagination.Normalize()
	return s.runRepo.ListByOrg(ctx, orgID, pagination)
}
