package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// BlogService orchestrates blog.Runner invocations and persists each run.
type BlogService interface {
	Generate(ctx context.Context, orgID uuid.UUID, triggeredBy *uuid.UUID, source string, req model.GenerateBlogRequest) (*model.BlogRun, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.BlogRun, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.BlogRun], error)
}

type blogService struct {
	runner *blog.Runner
	repo   repository.BlogRepository
	logger *zap.Logger
}

// NewBlogService creates a BlogService. runner may be nil when GITHUB_TOKEN is
// unset — Generate will then return a clear error instead of crashing.
func NewBlogService(runner *blog.Runner, repo repository.BlogRepository, logger *zap.Logger) BlogService {
	return &blogService{runner: runner, repo: repo, logger: logger}
}

// Generate runs the pipeline synchronously. "Synchronous" is a deliberate
// choice — a single topic takes ~10-30s on a warm Ollama host, which is
// within HTTP timeout budget. Multi-topic batches that exceed that should
// use the scheduled variant which runs in the background.
func (s *blogService) Generate(
	ctx context.Context,
	orgID uuid.UUID,
	triggeredBy *uuid.UUID,
	source string,
	req model.GenerateBlogRequest,
) (*model.BlogRun, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("blog_svc: generator not configured (GITHUB_TOKEN missing?)")
	}

	startedAt := time.Now()
	run := &model.BlogRun{
		ID:          uuid.New(),
		OrgID:       orgID,
		TriggeredBy: triggeredBy,
		Source:      source,
		Status:      model.BlogRunStatusRunning,
		Topics:      req.Topics,
		StartedAt:   &startedAt,
	}
	if req.Model != "" {
		m := req.Model
		run.Model = &m
	}

	if err := s.repo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("blog_svc: persist run: %w", err)
	}

	result, runErr := s.runner.Run(ctx, blog.GenerateRequest{
		Topics:      req.Topics,
		Model:       req.Model,
		MaxArticles: req.MaxArticles,
	})

	completedAt := time.Now()
	run.CompletedAt = &completedAt

	if runErr != nil {
		msg := runErr.Error()
		run.Status = model.BlogRunStatusFailed
		run.ErrorMessage = &msg
		if updateErr := s.repo.Update(context.Background(), run); updateErr != nil {
			s.logger.Error("blog_svc: failed to record failure", zap.Error(updateErr))
		}
		return run, runErr
	}

	articles := make([]model.BlogRunArticle, 0, len(result.Articles))
	for _, a := range result.Articles {
		articles = append(articles, model.BlogRunArticle{Topic: a.Topic, Slug: a.Slug, Path: a.Path})
	}
	run.Status = model.BlogRunStatusCompleted
	run.Branch = &result.Branch
	pr := result.PRNumber
	run.PRNumber = &pr
	run.PRURL = &result.PRURL
	run.Articles = articles

	if err := s.repo.Update(ctx, run); err != nil {
		s.logger.Error("blog_svc: failed to persist success", zap.Error(err))
	}
	return run, nil
}

func (s *blogService) GetByID(ctx context.Context, id uuid.UUID) (*model.BlogRun, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *blogService) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.BlogRun], error) {
	return s.repo.ListByOrg(ctx, orgID, params)
}
