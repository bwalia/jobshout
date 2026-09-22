package service

import (
	"testing"
	"time"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/model"
)

func TestRuntimeBudgetScalesPerArticle(t *testing.T) {
	s := &blogService{maxRuntime: 45 * time.Minute}
	briefs := func(n int) []model.BlogBrief { return make([]model.BlogBrief, n) }

	cases := []struct {
		name string
		req  model.GenerateBlogRequest
		want int
	}{
		{"trending default", model.GenerateBlogRequest{Trending: true}, 1},
		{"trending three", model.GenerateBlogRequest{Trending: true, TrendingCount: 3}, 3},
		{"trending capped by max_articles", model.GenerateBlogRequest{Trending: true, TrendingCount: 5, MaxArticles: 2}, 2},
		{"explicit briefs", model.GenerateBlogRequest{Briefs: briefs(4)}, 4},
		{"briefs capped by max_articles", model.GenerateBlogRequest{Briefs: briefs(4), MaxArticles: 1}, 1},
		{"hard ceiling", model.GenerateBlogRequest{Briefs: briefs(blog.HardMaxArticles + 5)}, blog.HardMaxArticles},
		{"empty never zero", model.GenerateBlogRequest{}, 1},
	}
	for _, tc := range cases {
		if got, want := s.runtimeBudget(tc.req), time.Duration(tc.want)*45*time.Minute; got != want {
			t.Errorf("%s: budget = %v, want %v", tc.name, got, want)
		}
	}
}
