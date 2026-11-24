package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/model"
)

// Resend to OpsAPI: one stored article sent to its CMS again, as it is.
//
// Semantics:
//   - The article is not regenerated. The payload is the one Publish builds
//     (blog.Runner.buildPost), from the stored markdown, HTML and cover.
//   - It goes to the CMS its run files to: the writer's website when the run
//     names one, else the ring's OPSAPI_* namespace — the same as Publish.
//   - An article that already has a post (post_uuid) has that post updated in
//     place: PUT /api/v2/cms/posts/{uuid}. Status and slug are left alone, so
//     a draft stays a draft and a live post stays live at its URL.
//   - If opsapi answers 404 for that post (deleted in its dashboard), or the
//     article was never filed, a new draft is created: POST /api/v2/cms/posts,
//     exactly as Publish creates one, and the article records the new post.
//
// Duplicates: a resend first claims the article (blog_articles.resend_started_at,
// one conditional UPDATE), so a second resend of the same article — a double
// click, another tab, another API pod — is refused with ErrResendInProgress
// until the first finishes. A claim older than resendClaimStale is treated as
// abandoned (its pod died) and can be taken over.

var (
	// ErrResendInProgress: another resend of the article holds the claim.
	ErrResendInProgress = errors.New("blog_svc: this article is already being resent; wait for it to finish")
	// ErrArticleNotFound: no article has that id.
	ErrArticleNotFound = errors.New("blog_svc: article not found")
	// ErrArticleOtherOrg: the article belongs to another organization.
	ErrArticleOtherOrg = errors.New("blog_svc: article does not belong to this organization")
	// ErrResendNotConfigured: there is no CMS to send the article to.
	ErrResendNotConfigured = errors.New("blog_svc: publishing is not configured (OPSAPI_BASE_URL, " +
		"OPSAPI_API_KEY and OPSAPI_NAMESPACE must all be set, or pick a website for the writer)")
	// ErrResendNoContent: the article has no body to send.
	ErrResendNoContent = errors.New("blog_svc: the article has no content to send")
)

// resendBudget bounds one resend. It is usually a single CMS call, but an
// article with no cover has one drawn first, and copying images to a website
// uploads each one.
var resendBudget = 10 * time.Minute

// resendClaimStale is when a claim is considered abandoned. Longer than
// resendBudget, so a live resend is never taken over.
const resendClaimStale = 15 * time.Minute

func (s *blogService) ResendArticle(ctx context.Context, orgID, articleID uuid.UUID) (*model.BlogArticle, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("blog_svc: generator not configured")
	}
	a, err := s.repo.GetArticle(ctx, articleID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrArticleNotFound
	}
	if a.OrgID != orgID {
		return nil, ErrArticleOtherOrg
	}
	if a.Markdown == "" && a.HTML == "" {
		return nil, ErrResendNoContent
	}
	run, err := s.repo.GetByID(ctx, a.RunID)
	if err != nil {
		return nil, fmt.Errorf("blog_svc: looking up the article's run: %w", err)
	}
	// The same CMS Publish files the run's articles in.
	dest, err := s.destination(ctx, run)
	if err != nil {
		return nil, err
	}
	cms := dest
	if cms == nil {
		if !s.CanPublish() {
			return nil, ErrResendNotConfigured
		}
		cms = s.runner.RingCMS()
	}
	post, err := s.postSettings(ctx, run)
	if err != nil {
		return nil, err
	}

	// A browser that stops waiting must not abandon a resend half done: the
	// claim would then only clear when it goes stale.
	ctx = context.WithoutCancel(ctx)
	ok, err := s.repo.ClaimArticleResend(ctx, a.ID, resendClaimStale)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrResendInProgress
	}

	log := s.logger.With(
		zap.String("org_id", orgID.String()),
		zap.String("blog_run_id", run.ID.String()),
		zap.String("article_id", a.ID.String()),
		zap.String("namespace", cms.Namespace()),
		zap.String("previous_post_uuid", derefStr(a.PostUUID)),
	)
	started := time.Now()

	sendCtx, cancel := context.WithTimeout(ctx, resendBudget)
	defer cancel()
	// The fields Publish sends for a stored article, so the two cannot differ.
	gen := blog.GeneratedArticle{
		Topic: a.Topic, Slug: a.Slug, Path: a.Path,
		Markdown: a.Markdown, HTML: a.HTML, WordCount: a.WordCount,
		CoverImageURL: a.CoverImageURL,
		RunID:         &run.ID,
	}
	res, serr := s.runner.Resend(sendCtx, orgID, cms, gen, derefStr(a.PostUUID), post)

	rec := model.ArticleResend{At: time.Now().UTC(), Namespace: cms.Namespace()}
	var posted *model.BlogArticlePost
	if serr != nil {
		rec.Result = model.ResendResultFailed
		rec.PostUUID = derefStr(a.PostUUID)
		rec.Error = truncateRunError(serr.Error())
		log.Error("blog_svc: resend to OpsAPI failed",
			zap.Duration("duration", time.Since(started)), zap.Error(serr))
	} else {
		rec.Result = res.Action
		rec.PostUUID, rec.PostStatus = res.PostUUID, res.Status
		posted = &model.BlogArticlePost{ArticleID: a.ID, PostUUID: res.PostUUID, Status: res.Status,
			CoverImageURL: res.CoverImageURL}
		log.Info("blog_svc: resent article to OpsAPI",
			zap.String("result", res.Action),
			zap.String("post_uuid", res.PostUUID),
			zap.String("post_status", res.Status),
			zap.Duration("duration", time.Since(started)))
	}

	// Recorded, and the claim released, whatever happened. If this write
	// fails the claim stays until it goes stale; say so loudly, since a new
	// post's uuid would then be unknown here too.
	if ferr := s.repo.FinishArticleResend(ctx, a.ID, rec, posted); ferr != nil {
		log.Error("blog_svc: failed to record the resend outcome", zap.Error(ferr))
	}
	if serr != nil {
		return nil, fmt.Errorf("blog_svc: resend to OpsAPI: %w", serr)
	}

	if fresh, gerr := s.repo.GetArticle(ctx, a.ID); gerr == nil && fresh != nil {
		return fresh, nil
	}
	a.LastResend = &rec
	a.PostUUID, a.PostStatus = &rec.PostUUID, &rec.PostStatus
	return a, nil
}
