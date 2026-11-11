package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// scriptLLM answers every call with the same reply.
type scriptLLM struct{ reply string }

func (s scriptLLM) Generate(context.Context, llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return &llm.GenerateResponse{Content: s.reply}, nil
}
func (scriptLLM) ProviderName() string { return "script" }

// redrawStore holds one article. Its writes fail on a context that is done,
// as a database call does.
type redrawStore struct {
	repository.BlogRepository
	article     model.BlogArticle
	body, cover bool
}

func (s *redrawStore) GetArticle(context.Context, uuid.UUID) (*model.BlogArticle, error) {
	a := s.article
	return &a, nil
}

func (s *redrawStore) UpdateArticleBody(ctx context.Context, _ uuid.UUID, _, _ string) error {
	s.body = ctx.Err() == nil
	return ctx.Err()
}

func (s *redrawStore) UpdateArticleCover(ctx context.Context, _ uuid.UUID, _, _ string, _ model.CoverImageMeta) error {
	s.cover = ctx.Err() == nil
	return ctx.Err()
}

// On int a redraw ran out of its budget at the cover, and the save then
// failed on the same expired context: the figures it had drawn were lost.
func TestRedrawImages_SavesWhatItDrewWhenTheBudgetRunsOut(t *testing.T) {
	defer func(b time.Duration) { redrawBudget = b }(redrawBudget)
	redrawBudget = time.Nanosecond

	org := uuid.New()
	store := &redrawStore{article: model.BlogArticle{
		ID: uuid.New(), OrgID: org, RunID: uuid.New(), Title: "CI in practice",
		Markdown: "# CI in practice\n\nArgo CD syncs the staging cluster from a Git repository.\n\n" +
			"## How the pipeline runs\n\nEvery commit is built, tested, then we deploy.\n\nMore detail.\n",
	}}
	svc := &blogService{repo: store, logger: zap.NewNop()}
	svc.runner = blog.NewRunner(blog.Config{}, scriptLLM{reply: `{"cover_scene":{"layout":"hub",` +
		`"labels":["Argo CD","Git repository","staging cluster"],"eyebrow":"GitOps"},` +
		`"figures":[{"kind":"process","section":"How the pipeline runs","content":"Commit → Build → Test → Deploy"}]}`},
		nil, nil, zap.NewNop()).WithCoverStore(newMemCovers())

	_, res, err := svc.RedrawImages(context.Background(), org, store.article.ID, false)
	if err != nil || res.Figures != 1 || !res.Cover || !store.body || !store.cover {
		t.Fatalf("err=%v res=%+v saved body=%v cover=%v", err, res, store.body, store.cover)
	}
}
