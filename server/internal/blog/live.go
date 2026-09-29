package blog

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/integration/adapters/jobshoutcom"
	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/model"
)

// Publishing live is the JobShout.com Content Writer's second step. Writing
// ends with a CMS draft, as for the Article Writer; a person then publishes it,
// and that one action makes the CMS post public and publishes the article on
// jobshout.com. The person pressing the button is the editorial approval, so
// jobshout.com does not queue it for a second review — it trusts the agent the
// client is configured as (INSIGHTS_TRUSTED_AGENTS there).

// CMSStatusSetter is the CMS capability publishing live needs beyond drafting.
// Declared separately from CMSPublisher so existing fakes that only draft are
// untouched.
type CMSStatusSetter interface {
	SetPostStatus(ctx context.Context, postUUID, status string) (*opsapi.Post, error)
}

// LiveCMSPost is a draft to take public, with enough of the article to re-file
// as published when the API key cannot update existing posts (cms:create only).
type LiveCMSPost struct {
	UUID    string
	Article GeneratedArticle
}

// LiveCMSResult is one draft that is now public. LiveUUID equals OriginalUUID
// when the draft was updated in place; otherwise it is a new published post
// created because the key lacked cms:update.
type LiveCMSResult struct {
	OriginalUUID string
	LiveUUID     string
}

// WithLiveInsights enables publishing straight to readers on jobshout.com.
func (r *Runner) WithLiveInsights(p InsightsPublisher) *Runner {
	r.liveInsights = p
	return r
}

// CanPublishLive reports whether both halves of publishing live are
// configured: a CMS that can change a post's status, and a jobshout.com client
// that publishes directly. The nil checks are on dynamic values, as in
// CanPublish, because main.go passes possibly-nil pointers.
func (r *Runner) CanPublishLive() bool {
	if r == nil || !r.CanPublish() || r.liveInsights == nil {
		return false
	}
	if _, ok := r.cms.(CMSStatusSetter); !ok {
		return false
	}
	if c, ok := r.liveInsights.(*jobshoutcom.Client); ok {
		return c != nil
	}
	return true
}

// SetPostsLive makes each CMS draft public. Prefer updating the draft in place;
// when the API key only has cms:create (opsapi returns 403 on PUT), create a
// new post already in published status with the same body. Stops at the first
// failure and returns what went live before it.
func (r *Runner) SetPostsLive(ctx context.Context, posts []LiveCMSPost, agentName string, progress ProgressFunc) ([]LiveCMSResult, error) {
	setter, ok := r.cms.(CMSStatusSetter)
	if !ok || !r.CanPublish() {
		return nil, fmt.Errorf("blog: the CMS is not configured for publishing live")
	}
	live := make([]LiveCMSResult, 0, len(posts))
	for i, p := range posts {
		report(progress, model.BlogStepGoingLive,
			fmt.Sprintf("Publishing %d/%d in the CMS", i+1, len(posts)), agentName)
		res, err := r.takePostLive(ctx, setter, p)
		if err != nil {
			return live, fmt.Errorf("blog: publish CMS post %d/%d: %w", i+1, len(posts), err)
		}
		live = append(live, res)
	}
	return live, nil
}

func (r *Runner) takePostLive(ctx context.Context, setter CMSStatusSetter, p LiveCMSPost) (LiveCMSResult, error) {
	if _, err := setter.SetPostStatus(ctx, p.UUID, opsapi.StatusPublished); err == nil {
		return LiveCMSResult{OriginalUUID: p.UUID, LiveUUID: p.UUID}, nil
	} else if !opsapi.IsUpdateForbidden(err) {
		return LiveCMSResult{}, err
	}

	// Key can create but not update: file a published copy. The draft stays;
	// editors can delete it in the CMS. Prefer this over blocking Publish live
	// on a key minted with the original create-only script.
	a := p.Article
	if a.HTML == "" {
		if err := a.render(); err != nil {
			return LiveCMSResult{}, fmt.Errorf("render for create-as-published: %w", err)
		}
	} else {
		if a.Title == "" {
			a.Title = articleTitle(a.Markdown, a.Topic)
		}
		if a.Excerpt == "" {
			a.Excerpt = articleExcerpt(a.HTML)
		}
	}
	post, err := r.cms.CreatePost(ctx, opsapi.CreatePostRequest{
		Title:            a.Title,
		Slug:             a.Slug,
		Excerpt:          a.Excerpt,
		ContentHTML:      a.HTML,
		Status:           opsapi.StatusPublished,
		AuthorName:       r.cfg.AuthorName,
		FeaturedImageURL: publicImageURL(r.cfg.PublicBaseURL, a.CoverImageURL),
		Tags:             articleTags(a),
		SEOTitle:         a.Title,
		SEODescription:   a.Excerpt,
	})
	if err != nil {
		return LiveCMSResult{}, fmt.Errorf("create-as-published after cms:update denied: %w", err)
	}
	r.logger.Info("blog: created published CMS post because API key lacks cms:update",
		zap.String("draft_uuid", p.UUID),
		zap.String("live_uuid", post.UUID),
		zap.String("slug", post.Slug),
	)
	return LiveCMSResult{OriginalUUID: p.UUID, LiveUUID: post.UUID}, nil
}

// PublishLiveInsights publishes each article on jobshout.com. Same retry rule
// as PublishInsights: a failure part-way returns what was published so far.
func (r *Runner) PublishLiveInsights(ctx context.Context, articles []GeneratedArticle, agentName string, progress ProgressFunc) (*InsightsResult, error) {
	if !r.CanPublishLive() {
		return nil, fmt.Errorf("blog: publishing live is not configured")
	}
	if len(articles) == 0 {
		return nil, fmt.Errorf("blog: nothing to publish")
	}
	result := &InsightsResult{Posts: make([]InsightsPost, 0, len(articles))}
	for i, a := range articles {
		report(progress, model.BlogStepGoingLive,
			fmt.Sprintf("Publishing %d/%d on jobshout.com — %s", i+1, len(articles), a.Topic), agentName)
		item, err := r.liveInsights.SubmitInsight(ctx, insightFromArticle(a, r.cfg.PublicBaseURL))
		if err != nil {
			result.PublishedAt = r.clock()
			return result, fmt.Errorf("blog: jobshout.com %d/%d: %w", i+1, len(articles), err)
		}
		result.Posts = append(result.Posts, InsightsPost{
			Slug: a.Slug, ItemID: item.ID, ItemSlug: item.Slug, Status: item.Status,
		})
	}
	result.PublishedAt = r.clock()
	r.logger.Info("blog: published articles on jobshout.com", zap.Int("articles", len(result.Posts)))
	report(progress, model.BlogStepLive,
		fmt.Sprintf("Published %d article(s) on jobshout.com", len(result.Posts)), agentName)
	return result, nil
}
