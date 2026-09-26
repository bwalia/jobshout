package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/model"
)

func (s *blogService) CanPublishLive() bool {
	return s.runner != nil && s.runner.CanPublishLive()
}

// liveSteps are appended when a run is published live.
func liveSteps() []model.BlogStep {
	return []model.BlogStep{
		{Key: model.BlogStepGoingLive, Label: "Publishing live", Status: model.StepStatusPending},
		{Key: model.BlogStepLive, Label: "Live on jobshout.com", Status: model.StepStatusPending},
	}
}

// PublishLive takes a Content Writer run live: its CMS drafts become public
// and its articles are published on jobshout.com.
//
// Each half is idempotent per article — a post already live is not touched
// again, an article already on jobshout.com is not submitted twice — so a
// publish that failed part-way is finished by pressing the button again. Drafts
// that were never filed (the CMS was down when the run finished) are filed
// first.
func (s *blogService) PublishLive(ctx context.Context, orgID uuid.UUID, runID uuid.UUID) (*model.BlogRun, error) {
	if !s.CanPublishLive() {
		return nil, fmt.Errorf("blog_svc: publishing live is not configured (the CMS needs OPSAPI_* and jobshout.com needs JOBSHOUT_COM_API_URL and JOBSHOUT_INTERNAL_TOKEN)")
	}
	run, err := s.repo.GetByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.OrgID != orgID {
		return nil, fmt.Errorf("blog_svc: run does not belong to this organization")
	}
	if run.Options.Writer != model.BuiltinJobShoutComWriter {
		return nil, fmt.Errorf("blog_svc: only JobShout.com Content Writer runs are published live; use Publish and Send to Insights")
	}
	if run.Status != model.BlogRunStatusCompleted {
		return nil, fmt.Errorf("blog_svc: only a completed run can be published (status is %q)", run.Status)
	}

	if run.PublishedAt == nil {
		if run, err = s.Publish(ctx, orgID, runID); err != nil {
			return nil, fmt.Errorf("blog_svc: file CMS drafts before publishing: %w", err)
		}
	}

	stored, err := s.repo.ListArticlesByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if len(stored) == 0 {
		return nil, fmt.Errorf("blog_svc: run has no articles to publish")
	}

	postArticle := make(map[string]uuid.UUID, len(stored))
	var pending []string
	articleIDs := make(map[string]uuid.UUID, len(stored))
	var toSubmit []blog.GeneratedArticle
	for _, a := range stored {
		if a.PostUUID != nil && *a.PostUUID != "" && (a.PostStatus == nil || *a.PostStatus != opsapi.StatusPublished) {
			postArticle[*a.PostUUID] = a.ID
			pending = append(pending, *a.PostUUID)
		}
		if a.InsightsItemID == nil || *a.InsightsItemID == "" {
			articleIDs[a.Slug] = a.ID
			toSubmit = append(toSubmit, blog.GeneratedArticle{
				Topic: a.Topic, Slug: a.Slug, Path: a.Path, Title: a.Title,
				Markdown: a.Markdown, HTML: a.HTML, WordCount: a.WordCount,
				CoverImageURL: a.CoverImageURL,
			})
		}
	}
	if len(pending) == 0 && len(toSubmit) == 0 {
		if run.InsightsPublishedAt == nil {
			return s.finalizeInsights(ctx, run, time.Now())
		}
		return run, nil
	}

	run.Steps = append(run.Steps, liveSteps()...)
	tracker := s.newTracker(run)
	writer := runWriterName(run)

	fail := func(cause error) (*model.BlogRun, error) {
		tracker.fail(cause)
		run.Steps = tracker.steps
		msg := cause.Error()
		run.ErrorMessage = &msg
		if uerr := s.repo.Update(ctx, run); uerr != nil {
			s.logger.Error("blog_svc: failed to record publish-live failure", zap.Error(uerr))
		}
		return nil, cause
	}

	// CMS first: if jobshout.com then fails, the article is at least public on
	// the site that owns it, and the retry only has jobshout.com left to do.
	if len(pending) > 0 {
		live, lerr := s.runner.SetPostsLive(ctx, pending, writer, tracker.advance)
		posts := make([]model.BlogArticlePost, 0, len(live))
		for _, id := range live {
			posts = append(posts, model.BlogArticlePost{ArticleID: postArticle[id], PostUUID: id, Status: opsapi.StatusPublished})
		}
		if err := s.repo.MarkArticlesPosted(ctx, posts); err != nil {
			s.logger.Error("blog_svc: failed to record live CMS posts", zap.Error(err))
		}
		if lerr != nil {
			return fail(lerr)
		}
	}

	at := time.Now()
	if len(toSubmit) > 0 {
		result, perr := s.runner.PublishLiveInsights(ctx, toSubmit, writer, tracker.advance)
		if result != nil {
			filed := make([]model.BlogArticleInsights, 0, len(result.Posts))
			for _, p := range result.Posts {
				if id, ok := articleIDs[p.Slug]; ok {
					filed = append(filed, model.BlogArticleInsights{ArticleID: id, ItemID: p.ItemID, Slug: p.ItemSlug, Status: p.Status})
				}
			}
			if err := s.repo.MarkArticlesInInsights(ctx, filed); err != nil {
				s.logger.Error("blog_svc: failed to record jobshout.com items — a retry would duplicate them", zap.Error(err))
			}
			at = result.PublishedAt
		}
		if perr != nil {
			return fail(perr)
		}
	}

	tracker.finish()
	run.Steps = tracker.steps
	return s.finalizeInsights(ctx, run, at)
}
