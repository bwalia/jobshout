package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

// SEORunRepository persists SEO Analyst runs.
type SEORunRepository interface {
	Create(ctx context.Context, run *model.SEORun) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.SEORun, error)
	Update(ctx context.Context, run *model.SEORun) error
	ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.SEORun], error)
}

type seoRunRepository struct{ pool *pgxpool.Pool }

// NewSEORunRepository constructs a Postgres-backed store.
func NewSEORunRepository(pool *pgxpool.Pool) SEORunRepository {
	return &seoRunRepository{pool: pool}
}

const seoRunColumns = `
	id, agent_id, task_id, org_id, status, mode, COALESCE(source,'urls'), url, platform, detected_cms,
	git_repo, git_branch, COALESCE(github_connection,''), COALESCE(path_glob,''), COALESCE(max_pages,10),
	keywords, instruction, score, report, pages, progress, publish,
	error_message, requested_by, started_at, completed_at, created_at, updated_at`

func scanSEORun(row pgx.Row) (*model.SEORun, error) {
	run := &model.SEORun{}
	var scoreJSON, reportJSON, pagesJSON, progressJSON, publishJSON []byte
	if err := row.Scan(
		&run.ID, &run.AgentID, &run.TaskID, &run.OrgID, &run.Status, &run.Mode, &run.Source,
		&run.URL, &run.Platform, &run.DetectedCMS, &run.GitRepo, &run.GitBranch,
		&run.GitHubConnection, &run.PathGlob, &run.MaxPages,
		&run.Keywords, &run.Instruction, &scoreJSON, &reportJSON, &pagesJSON, &progressJSON, &publishJSON,
		&run.ErrorMessage, &run.RequestedBy, &run.StartedAt, &run.CompletedAt,
		&run.CreatedAt, &run.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(scoreJSON) > 0 {
		var s model.SEOScore
		if json.Unmarshal(scoreJSON, &s) == nil {
			run.Score = &s
		}
	}
	if len(reportJSON) > 0 {
		var r model.SEOReport
		if json.Unmarshal(reportJSON, &r) == nil {
			run.Report = &r
		}
	}
	if len(pagesJSON) > 0 {
		_ = json.Unmarshal(pagesJSON, &run.Pages)
	}
	if len(progressJSON) > 0 {
		var p model.SEOProgress
		if json.Unmarshal(progressJSON, &p) == nil {
			run.Progress = &p
		}
	}
	if len(publishJSON) > 0 {
		var p model.SEOPublishResult
		if json.Unmarshal(publishJSON, &p) == nil {
			run.Publish = &p
		}
	}
	return run, nil
}

func (r *seoRunRepository) Create(ctx context.Context, run *model.SEORun) error {
	score, report, pages, progress, publish, err := marshalSEOExtras(run)
	if err != nil {
		return err
	}
	if run.Source == "" {
		run.Source = "urls"
	}
	if run.MaxPages <= 0 {
		run.MaxPages = 10
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO seo_runs (
			id, agent_id, task_id, org_id, status, mode, source, url, platform, detected_cms,
			git_repo, git_branch, github_connection, path_glob, max_pages,
			keywords, instruction, score, report, pages, progress, publish,
			error_message, requested_by, started_at, completed_at, created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,NOW(),NOW()
		) RETURNING created_at, updated_at`,
		run.ID, run.AgentID, run.TaskID, run.OrgID, run.Status, run.Mode, run.Source, run.URL,
		run.Platform, run.DetectedCMS, run.GitRepo, run.GitBranch, run.GitHubConnection,
		run.PathGlob, run.MaxPages, run.Keywords, run.Instruction, score, report, pages, progress, publish,
		run.ErrorMessage, run.RequestedBy, run.StartedAt, run.CompletedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *seoRunRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.SEORun, error) {
	run, err := scanSEORun(r.pool.QueryRow(ctx, `SELECT `+seoRunColumns+` FROM seo_runs WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("seo run not found")
		}
		return nil, err
	}
	return run, nil
}

func (r *seoRunRepository) Update(ctx context.Context, run *model.SEORun) error {
	score, report, pages, progress, publish, err := marshalSEOExtras(run)
	if err != nil {
		return err
	}
	return r.pool.QueryRow(ctx, `
		UPDATE seo_runs SET
			status=$2, mode=$3, source=$4, url=$5, platform=$6, detected_cms=$7, git_repo=$8, git_branch=$9,
			github_connection=$10, path_glob=$11, max_pages=$12,
			keywords=$13, instruction=$14, score=$15, report=$16, pages=$17, progress=$18, publish=$19,
			error_message=$20, started_at=$21, completed_at=$22, updated_at=NOW()
		WHERE id=$1 RETURNING created_at, updated_at`,
		run.ID, run.Status, run.Mode, run.Source, run.URL, run.Platform, run.DetectedCMS, run.GitRepo,
		run.GitBranch, run.GitHubConnection, run.PathGlob, run.MaxPages,
		run.Keywords, run.Instruction, score, report, pages, progress, publish,
		run.ErrorMessage, run.StartedAt, run.CompletedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *seoRunRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.SEORun], error) {
	pagination.Normalize()
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM seo_runs WHERE org_id=$1`, orgID).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+seoRunColumns+` FROM seo_runs WHERE org_id=$1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, pagination.PerPage, pagination.Offset())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]model.SEORun, 0, pagination.PerPage)
	for rows.Next() {
		run, err := scanSEORun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *run)
	}
	pages := 1
	if pagination.PerPage > 0 {
		pages = (total + pagination.PerPage - 1) / pagination.PerPage
	}
	return &model.PaginatedResponse[model.SEORun]{
		Data: runs, Total: total, Page: pagination.Page, PerPage: pagination.PerPage, TotalPages: pages,
	}, nil
}

func marshalSEOExtras(run *model.SEORun) (score, report, pages, progress, publish []byte, err error) {
	if run.Score != nil {
		score, err = json.Marshal(run.Score)
		if err != nil {
			return
		}
	}
	if run.Report != nil {
		report, err = json.Marshal(run.Report)
		if err != nil {
			return
		}
	}
	if len(run.Pages) > 0 {
		pages, err = json.Marshal(run.Pages)
		if err != nil {
			return
		}
	}
	if run.Progress != nil {
		progress, err = json.Marshal(run.Progress)
		if err != nil {
			return
		}
	}
	if run.Publish != nil {
		publish, err = json.Marshal(run.Publish)
		if err != nil {
			return
		}
	}
	return
}
