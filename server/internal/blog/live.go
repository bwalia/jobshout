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

// SetPostsLive makes each CMS draft public. It stops at the first failure and
// returns the posts that went live before it, so the caller can record them
// and a retry does not touch them again.
func (r *Runner) SetPostsLive(ctx context.Context, postUUIDs []string, agentName string, progress ProgressFunc) ([]string, error) {
	setter, ok := r.cms.(CMSStatusSetter)
	if !ok || !r.CanPublish() {
		return nil, fmt.Errorf("blog: the CMS is not configured for publishing live")
	}
	live := make([]string, 0, len(postUUIDs))
	for i, id := range postUUIDs {
		report(progress, model.BlogStepGoingLive,
			fmt.Sprintf("Publishing %d/%d in the CMS", i+1, len(postUUIDs)), agentName)
		if _, err := setter.SetPostStatus(ctx, id, opsapi.StatusPublished); err != nil {
			return live, fmt.Errorf("blog: publish CMS post %d/%d: %w", i+1, len(postUUIDs), err)
		}
		live = append(live, id)
	}
	return live, nil
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
