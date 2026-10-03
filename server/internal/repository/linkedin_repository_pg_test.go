package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// linkedInPG runs migration 060 in a throwaway schema of a real Postgres. The
// claim and guard statements are what keep two replicas from drafting or
// posting twice, so they are exercised against the real thing:
//
//	LINKEDIN_TEST_DATABASE_URL=postgres://postgres:pw@localhost:5432/postgres go test ./internal/repository -run LinkedInPG
func linkedInPG(t *testing.T) (*pgxpool.Pool, uuid.UUID) {
	t.Helper()
	dsn := os.Getenv("LINKEDIN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LINKEDIN_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "linkedin_test_" + uuid.NewString()[:8]
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		pool.Close()
	})
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("%v\n%s", err, sql[:min(200, len(sql))])
		}
	}
	// Just enough of the tables migration 060 refers to.
	exec(`CREATE SCHEMA ` + schema)
	exec(`CREATE TABLE organizations (id UUID PRIMARY KEY DEFAULT gen_random_uuid());
		CREATE TABLE users (id UUID PRIMARY KEY DEFAULT gen_random_uuid());
		CREATE TABLE agents (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), org_id UUID, name TEXT, role TEXT,
			description TEXT, status TEXT, engine_type TEXT, system_prompt TEXT, metadata JSONB);
		CREATE TABLE blog_articles (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), org_id UUID NOT NULL,
			topic TEXT NOT NULL DEFAULT '', title TEXT, markdown TEXT, audience VARCHAR(64),
			post_status VARCHAR(50), posted_at TIMESTAMPTZ,
			insights_slug TEXT, insights_status VARCHAR(50), insights_posted_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
		INSERT INTO organizations DEFAULT VALUES`)
	up, err := os.ReadFile("../../migrations/000060_linkedin_agent.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(string(up))
	exec(string(up)) // migrations replay on every boot

	var org uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM organizations LIMIT 1`).Scan(&org); err != nil {
		t.Fatal(err)
	}
	var seeded int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM agents WHERE metadata->>'builtin' = 'linkedin_poster'`).Scan(&seeded); err != nil || seeded != 1 {
		t.Fatalf("seeded %d LinkedIn agents after replay (err %v), want 1", seeded, err)
	}
	return pool, org
}

func insertArticle(t *testing.T, pool *pgxpool.Pool, org uuid.UUID, title, cols string, args ...any) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	q := `INSERT INTO blog_articles (org_id, title, markdown` + cols + `) VALUES ($1, $2, 'body'`
	for i := range args {
		q += ", $" + string(rune('3'+i))
	}
	q += `) RETURNING id`
	if err := pool.QueryRow(context.Background(), q, append([]any{org, title}, args...)...).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestLinkedInPG_ArticlesAndClaims(t *testing.T) {
	pool, org := linkedInPG(t)
	ctx := context.Background()
	r := NewLinkedInRepository(pool)
	now := time.Now()

	insertArticle(t, pool, org, "Unpublished draft", "")
	cms := insertArticle(t, pool, org, "CMS article", ", post_status, posted_at", "published", now.Add(-2*time.Hour))
	ins := insertArticle(t, pool, org, "Insights article on routing", ", insights_status, insights_slug, insights_posted_at", "Published", "routing", now.Add(-time.Hour))
	insertArticle(t, pool, org, "In review", ", insights_status, insights_slug, insights_posted_at", "InReview", "review", now)

	a, err := r.LatestUndrafted(ctx, org)
	if err != nil || a.ID != ins || a.InsightsSlug != "routing" || a.PublishedAt == nil {
		t.Fatalf("LatestUndrafted = %+v, %v", a, err)
	}
	if a, err := r.FindArticleByTitle(ctx, org, "cms"); err != nil || a.ID != cms || a.InsightsSlug != "" {
		t.Errorf("FindArticleByTitle = %+v, %v", a, err)
	}
	if _, err := r.FindArticleByTitle(ctx, org, "unpublished"); !errors.Is(err, ErrLinkedInNotFound) {
		t.Errorf("unpublished article found: %v", err)
	}
	since, err := r.PublishedSince(ctx, org, now.Add(-3*time.Hour), 10)
	if err != nil || len(since) != 2 || since[0].ID != cms || since[1].ID != ins {
		t.Fatalf("PublishedSince = %+v, %v", since, err)
	}

	base := model.LinkedInPost{OrgID: org, ArticleID: ins, ArticleTitle: "Insights article on routing", LinkURL: "https://x/insights/routing"}
	got, err := r.ClaimDrafts(ctx, base, model.LinkedInVariants, false)
	if err != nil || len(got) != 2 || got[0].Status != model.LinkedInPostDrafting {
		t.Fatalf("first claim = %+v, %v", got, err)
	}
	if again, _ := r.ClaimDrafts(ctx, base, model.LinkedInVariants, false); len(again) != 0 {
		t.Errorf("second claim took %d rows", len(again))
	}
	if again, _ := r.ClaimDrafts(ctx, base, model.LinkedInVariants, true); len(again) != 0 {
		t.Errorf("redraft took rows still drafting: %d", len(again))
	}
	if a, _ := r.LatestUndrafted(ctx, org); a.ID != cms {
		t.Errorf("drafted article still undrafted")
	}

	tech, biz := got[0], got[1]
	if err := r.FinishDraft(ctx, tech.ID, "Tech post.", ""); err != nil {
		t.Fatal(err)
	}
	if err := r.FinishDraft(ctx, biz.ID, "", "model down"); err != nil {
		t.Fatal(err)
	}
	if err := r.FinishDraft(ctx, tech.ID, "late writer", ""); err != nil {
		t.Fatal(err)
	}
	if p, _ := r.GetPost(ctx, org, tech.ID); p.Commentary != "Tech post." || p.Status != model.LinkedInPostDraft {
		t.Errorf("a finished draft was overwritten: %+v", p)
	}

	// Redraft resets the failed row only.
	re, err := r.ClaimDrafts(ctx, model.LinkedInPost{OrgID: org, ArticleID: ins, Notes: "new angle"}, []string{model.LinkedInVariantBusiness}, true)
	if err != nil || len(re) != 1 || re[0].ID != biz.ID || re[0].Notes != "new angle" || re[0].ErrorMessage != "" {
		t.Fatalf("redraft = %+v, %v", re, err)
	}

	// Posting: one winner, guarded edits, release and mark.
	if _, err := r.ClaimPosting(ctx, org, biz.ID); !errors.Is(err, ErrLinkedInConflict) {
		t.Errorf("claimed a drafting row for posting: %v", err)
	}
	if _, err := r.ClaimPosting(ctx, uuid.New(), tech.ID); !errors.Is(err, ErrLinkedInNotFound) {
		t.Errorf("other org claimed the post: %v", err)
	}
	p, err := r.ClaimPosting(ctx, org, tech.ID)
	if err != nil || p.Status != model.LinkedInPostPosting {
		t.Fatalf("ClaimPosting = %+v, %v", p, err)
	}
	if _, err := r.ClaimPosting(ctx, org, tech.ID); !errors.Is(err, ErrLinkedInConflict) {
		t.Errorf("second ClaimPosting: %v", err)
	}
	if _, err := r.UpdateCommentary(ctx, org, tech.ID, "edit in flight"); !errors.Is(err, ErrLinkedInConflict) {
		t.Errorf("edited while posting: %v", err)
	}
	if err := r.ReleasePosting(ctx, tech.ID, "rate limited"); err != nil {
		t.Fatal(err)
	}
	if p, _ := r.GetPost(ctx, org, tech.ID); p.Status != model.LinkedInPostDraft || p.ErrorMessage != "rate limited" {
		t.Errorf("after release: %+v", p)
	}
	if _, err := r.ClaimPosting(ctx, org, tech.ID); err != nil {
		t.Fatal(err)
	}
	user := uuid.New()
	posted, err := r.MarkPosted(ctx, tech.ID, "urn:li:share:9", "https://www.linkedin.com/feed/update/urn:li:share:9/", &user)
	if err != nil || posted.Status != model.LinkedInPostPosted || posted.PostedAt == nil || *posted.PostedBy != user {
		t.Fatalf("MarkPosted = %+v, %v", posted, err)
	}
	if again, _ := r.ClaimDrafts(ctx, base, []string{model.LinkedInVariantTechnical}, true); len(again) != 0 {
		t.Error("redraft reset a posted row")
	}

	// A drafting row whose drafter died is failed by the sweep.
	if _, err := pool.Exec(ctx, `UPDATE linkedin_posts SET updated_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, biz.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := r.FailStaleDrafting(ctx, now.Add(-30*time.Minute)); err != nil || n != 1 {
		t.Errorf("FailStaleDrafting = %d, %v", n, err)
	}
	list, err := r.ListPosts(ctx, org, 10)
	if err != nil || len(list) != 2 {
		t.Errorf("ListPosts = %d, %v", len(list), err)
	}
}

func TestLinkedInPG_ConnectionAndOAuthState(t *testing.T) {
	pool, org := linkedInPG(t)
	ctx := context.Background()
	r := NewLinkedInRepository(pool)

	if _, _, err := r.GetConnection(ctx, org); !errors.Is(err, ErrLinkedInNotFound) {
		t.Fatalf("GetConnection before connect: %v", err)
	}
	c := model.LinkedInConnection{OrgID: org, MemberURN: "urn:li:person:a", Name: "A", ExpiresAt: time.Now().Add(time.Hour)}
	if err := r.UpsertConnection(ctx, c, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	c.Name = "A renamed"
	if err := r.UpsertConnection(ctx, c, []byte{4}); err != nil {
		t.Fatal(err)
	}
	got, enc, err := r.GetConnection(ctx, org)
	if err != nil || got.Name != "A renamed" || len(enc) != 1 {
		t.Errorf("GetConnection = %+v %v %v", got, enc, err)
	}
	if orgs, _ := r.ConnectedOrgs(ctx); len(orgs) != 1 || orgs[0] != org {
		t.Errorf("ConnectedOrgs = %v", orgs)
	}
	c.ExpiresAt = time.Now().Add(-time.Minute)
	_ = r.UpsertConnection(ctx, c, []byte{4})
	if orgs, _ := r.ConnectedOrgs(ctx); len(orgs) != 0 {
		t.Errorf("expired connection listed: %v", orgs)
	}

	user := uuid.New()
	if err := r.PutOAuthState(ctx, "s1", org, user, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := r.PutOAuthState(ctx, "old", org, user, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if o, u, err := r.ConsumeOAuthState(ctx, "s1"); err != nil || o != org || u != user {
		t.Errorf("ConsumeOAuthState = %v %v %v", o, u, err)
	}
	if _, _, err := r.ConsumeOAuthState(ctx, "s1"); !errors.Is(err, ErrLinkedInNotFound) {
		t.Errorf("state reused: %v", err)
	}
	if _, _, err := r.ConsumeOAuthState(ctx, "old"); !errors.Is(err, ErrLinkedInNotFound) {
		t.Errorf("expired state accepted: %v", err)
	}
	if err := r.DeleteConnection(ctx, org); err != nil {
		t.Fatal(err)
	}
}
