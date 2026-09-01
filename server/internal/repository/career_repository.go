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

// CareerRepository persists person-scoped career data. All queries are org-scoped.
type CareerRepository interface {
	UpsertProfile(ctx context.Context, p *model.CareerProfile) error
	GetProfileByUser(ctx context.Context, orgID, userID uuid.UUID) (*model.CareerProfile, error)
	GetProfileByID(ctx context.Context, orgID, id uuid.UUID) (*model.CareerProfile, error)
	InsertProfileVersion(ctx context.Context, orgID, profileID uuid.UUID, cv, note string) error

	ListBlacklist(ctx context.Context, orgID, profileID uuid.UUID) ([]model.CareerBlacklistEntry, error)
	InsertBlacklist(ctx context.Context, e *model.CareerBlacklistEntry) error

	ListPortals(ctx context.Context, orgID, profileID uuid.UUID) ([]model.CareerPortal, error)
	UpsertPortal(ctx context.Context, p *model.CareerPortal) error

	UpsertPipelineItem(ctx context.Context, it *model.CareerPipelineItem) error
	GetPipelineByURL(ctx context.Context, profileID uuid.UUID, listingURL string) (*model.CareerPipelineItem, error)
	ListPipeline(ctx context.Context, orgID, profileID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerPipelineItem], error)
	UpdatePipeline(ctx context.Context, it *model.CareerPipelineItem) error

	UpsertApplication(ctx context.Context, a *model.CareerApplication) error
	GetApplication(ctx context.Context, orgID, id uuid.UUID) (*model.CareerApplication, error)
	GetApplicationByURL(ctx context.Context, profileID uuid.UUID, listingURL string) (*model.CareerApplication, error)
	ListApplications(ctx context.Context, orgID, profileID uuid.UUID, status string, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerApplication], error)
	UpdateApplication(ctx context.Context, a *model.CareerApplication) error
	AppendStatusEvent(ctx context.Context, e *model.CareerStatusEvent) error
	ListStatusEvents(ctx context.Context, applicationID uuid.UUID) ([]model.CareerStatusEvent, error)

	InsertEvaluation(ctx context.Context, e *model.CareerEvaluation) error
	GetEvaluation(ctx context.Context, orgID, id uuid.UUID) (*model.CareerEvaluation, error)
	ListEvaluations(ctx context.Context, orgID, profileID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerEvaluation], error)

	InsertArtifact(ctx context.Context, a *model.CareerArtifact) error
	ListArtifacts(ctx context.Context, applicationID uuid.UUID) ([]model.CareerArtifact, error)

	ListStories(ctx context.Context, orgID, profileID uuid.UUID) ([]model.CareerStory, error)
	GetStoryByID(ctx context.Context, id uuid.UUID) (*model.CareerStory, error)
	UpsertStory(ctx context.Context, s *model.CareerStory) error

	InsertScanRun(ctx context.Context, r *model.CareerScanRun) error
	UpdateScanRun(ctx context.Context, r *model.CareerScanRun) error
	HasScanEvent(ctx context.Context, profileID uuid.UUID, listingURL string) (bool, error)
	InsertScanEvent(ctx context.Context, orgID, profileID uuid.UUID, listingURL, company, title string) error

	InsertFollowup(ctx context.Context, f *model.CareerFollowup) error
	ListFollowups(ctx context.Context, orgID, profileID uuid.UUID) ([]model.CareerFollowup, error)

	GetLatestEvaluationForApp(ctx context.Context, orgID, applicationID uuid.UUID) (*model.CareerEvaluation, error)

	InsertContact(ctx context.Context, c *model.CareerContact) error
	ListContacts(ctx context.Context, orgID, profileID uuid.UUID) ([]model.CareerContact, error)

	InsertOffer(ctx context.Context, o *model.CareerOffer) error
	InsertSalaryObservation(ctx context.Context, o *model.CareerSalaryObservation) error
}

type careerRepository struct {
	pool *pgxpool.Pool
}

func NewCareerRepository(pool *pgxpool.Pool) CareerRepository {
	return &careerRepository{pool: pool}
}

func marshalJSON(v any) []byte {
	if v == nil {
		return []byte("{}")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func (r *careerRepository) UpsertProfile(ctx context.Context, p *model.CareerProfile) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO career_profiles (
			id, org_id, user_id, cv_markdown, identity, targets, location, work_auth,
			voice, house_rules, proof_points, narrative
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (org_id, user_id) DO UPDATE SET
			cv_markdown = EXCLUDED.cv_markdown,
			identity = EXCLUDED.identity,
			targets = EXCLUDED.targets,
			location = EXCLUDED.location,
			work_auth = EXCLUDED.work_auth,
			voice = EXCLUDED.voice,
			house_rules = EXCLUDED.house_rules,
			proof_points = EXCLUDED.proof_points,
			narrative = EXCLUDED.narrative,
			updated_at = NOW()
		RETURNING id, created_at, updated_at`,
		p.ID, p.OrgID, p.UserID, p.CVMarkdown,
		marshalJSON(p.Identity), marshalJSON(p.Targets), marshalJSON(p.Location), marshalJSON(p.WorkAuth),
		p.Voice, p.HouseRules, p.ProofPoints, p.Narrative,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("career_repo: upsert profile: %w", err)
	}
	return nil
}

func scanProfile(row pgx.Row) (*model.CareerProfile, error) {
	p := &model.CareerProfile{}
	var ident, targets, loc, auth []byte
	err := row.Scan(
		&p.ID, &p.OrgID, &p.UserID, &p.CVMarkdown,
		&ident, &targets, &loc, &auth,
		&p.Voice, &p.HouseRules, &p.ProofPoints, &p.Narrative,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(ident, &p.Identity)
	_ = json.Unmarshal(targets, &p.Targets)
	_ = json.Unmarshal(loc, &p.Location)
	_ = json.Unmarshal(auth, &p.WorkAuth)
	return p, nil
}

const profileCols = `id, org_id, user_id, cv_markdown, identity, targets, location, work_auth,
	voice, house_rules, proof_points, narrative, created_at, updated_at`

func (r *careerRepository) GetProfileByUser(ctx context.Context, orgID, userID uuid.UUID) (*model.CareerProfile, error) {
	p, err := scanProfile(r.pool.QueryRow(ctx,
		`SELECT `+profileCols+` FROM career_profiles WHERE org_id=$1 AND user_id=$2`, orgID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("career_repo: get profile: %w", err)
	}
	return p, nil
}

func (r *careerRepository) GetProfileByID(ctx context.Context, orgID, id uuid.UUID) (*model.CareerProfile, error) {
	p, err := scanProfile(r.pool.QueryRow(ctx,
		`SELECT `+profileCols+` FROM career_profiles WHERE org_id=$1 AND id=$2`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("career_repo: get profile by id: %w", err)
	}
	return p, nil
}

func (r *careerRepository) InsertProfileVersion(ctx context.Context, orgID, profileID uuid.UUID, cv, note string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO career_profile_versions (org_id, profile_id, cv_markdown, note)
		VALUES ($1,$2,$3,$4)`, orgID, profileID, cv, note)
	if err != nil {
		return fmt.Errorf("career_repo: profile version: %w", err)
	}
	return nil
}

func (r *careerRepository) ListBlacklist(ctx context.Context, orgID, profileID uuid.UUID) ([]model.CareerBlacklistEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, profile_id, company, domain, reason, created_at
		FROM career_blacklist WHERE org_id=$1 AND profile_id=$2 ORDER BY created_at DESC`, orgID, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CareerBlacklistEntry
	for rows.Next() {
		var e model.CareerBlacklistEntry
		if err := rows.Scan(&e.ID, &e.OrgID, &e.ProfileID, &e.Company, &e.Domain, &e.Reason, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if out == nil {
		out = []model.CareerBlacklistEntry{}
	}
	return out, rows.Err()
}

func (r *careerRepository) InsertBlacklist(ctx context.Context, e *model.CareerBlacklistEntry) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO career_blacklist (id, org_id, profile_id, company, domain, reason)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING created_at`,
		e.ID, e.OrgID, e.ProfileID, e.Company, e.Domain, e.Reason,
	).Scan(&e.CreatedAt)
}

func (r *careerRepository) ListPortals(ctx context.Context, orgID, profileID uuid.UUID) ([]model.CareerPortal, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, profile_id, board, slug, company, title_include, title_exclude, enabled, created_at, updated_at
		FROM career_portals WHERE org_id=$1 AND profile_id=$2 ORDER BY company`, orgID, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CareerPortal
	for rows.Next() {
		var p model.CareerPortal
		var inc, exc []string
		if err := rows.Scan(&p.ID, &p.OrgID, &p.ProfileID, &p.Board, &p.Slug, &p.Company, &inc, &exc, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.TitleInclude = nzStrings(inc)
		p.TitleExclude = nzStrings(exc)
		out = append(out, p)
	}
	if out == nil {
		out = []model.CareerPortal{}
	}
	return out, rows.Err()
}

func (r *careerRepository) UpsertPortal(ctx context.Context, p *model.CareerPortal) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO career_portals (id, org_id, profile_id, board, slug, company, title_include, title_exclude, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (profile_id, board, slug) DO UPDATE SET
			company=EXCLUDED.company, title_include=EXCLUDED.title_include,
			title_exclude=EXCLUDED.title_exclude, enabled=EXCLUDED.enabled, updated_at=NOW()
		RETURNING id, created_at, updated_at`,
		p.ID, p.OrgID, p.ProfileID, p.Board, p.Slug, p.Company, nzStrings(p.TitleInclude), nzStrings(p.TitleExclude), p.Enabled,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *careerRepository) UpsertPipelineItem(ctx context.Context, it *model.CareerPipelineItem) error {
	if it.ID == uuid.Nil {
		it.ID = uuid.New()
	}
	if it.Status == "" {
		it.Status = model.CareerPipelineOpen
	}
	if it.Liveness == "" {
		it.Liveness = "unknown"
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO career_pipeline_items (
			id, org_id, profile_id, listing_url, company, title, source, status, liveness, liveness_checked_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (profile_id, listing_url) DO UPDATE SET
			company=EXCLUDED.company, title=EXCLUDED.title, source=EXCLUDED.source,
			status=EXCLUDED.status, liveness=EXCLUDED.liveness,
			liveness_checked_at=EXCLUDED.liveness_checked_at, updated_at=NOW()
		RETURNING id, created_at, updated_at`,
		it.ID, it.OrgID, it.ProfileID, it.ListingURL, it.Company, it.Title, it.Source, it.Status, it.Liveness, it.LivenessCheckedAt,
	).Scan(&it.ID, &it.CreatedAt, &it.UpdatedAt)
}

func scanPipeline(row pgx.Row) (*model.CareerPipelineItem, error) {
	it := &model.CareerPipelineItem{}
	err := row.Scan(&it.ID, &it.OrgID, &it.ProfileID, &it.ListingURL, &it.Company, &it.Title, &it.Source,
		&it.Status, &it.Liveness, &it.LivenessCheckedAt, &it.CreatedAt, &it.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return it, nil
}

const pipelineCols = `id, org_id, profile_id, listing_url, company, title, source, status, liveness, liveness_checked_at, created_at, updated_at`

func (r *careerRepository) GetPipelineByURL(ctx context.Context, profileID uuid.UUID, listingURL string) (*model.CareerPipelineItem, error) {
	it, err := scanPipeline(r.pool.QueryRow(ctx,
		`SELECT `+pipelineCols+` FROM career_pipeline_items WHERE profile_id=$1 AND listing_url=$2`, profileID, listingURL))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return it, nil
}

func (r *careerRepository) ListPipeline(ctx context.Context, orgID, profileID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerPipelineItem], error) {
	pagination.Normalize()
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM career_pipeline_items WHERE org_id=$1 AND profile_id=$2`, orgID, profileID).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+pipelineCols+` FROM career_pipeline_items
		WHERE org_id=$1 AND profile_id=$2
		ORDER BY created_at DESC LIMIT $3 OFFSET $4`, orgID, profileID, pagination.PerPage, pagination.Offset())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.CareerPipelineItem{}
	for rows.Next() {
		it, err := scanPipeline(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *it)
	}
	return paginate(items, total, pagination), rows.Err()
}

func (r *careerRepository) UpdatePipeline(ctx context.Context, it *model.CareerPipelineItem) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE career_pipeline_items SET company=$2, title=$3, source=$4, status=$5, liveness=$6,
			liveness_checked_at=$7, updated_at=NOW() WHERE id=$1`,
		it.ID, it.Company, it.Title, it.Source, it.Status, it.Liveness, it.LivenessCheckedAt)
	return err
}

func (r *careerRepository) UpsertApplication(ctx context.Context, a *model.CareerApplication) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.Status == "" {
		a.Status = model.CareerStatusEvaluated
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO career_applications (
			id, org_id, profile_id, company, role, listing_url, status, score, via, agency, employer
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (profile_id, listing_url) WHERE listing_url <> '' DO UPDATE SET
			company=EXCLUDED.company, role=EXCLUDED.role, status=EXCLUDED.status,
			score=EXCLUDED.score, via=EXCLUDED.via, agency=EXCLUDED.agency, employer=EXCLUDED.employer,
			updated_at=NOW()
		RETURNING id, created_at, updated_at`,
		a.ID, a.OrgID, a.ProfileID, a.Company, a.Role, a.ListingURL, a.Status, a.Score, a.Via, a.Agency, a.Employer,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		// Partial unique index ON CONFLICT requires the same predicate; empty URL inserts normally.
		if a.ListingURL == "" {
			return r.pool.QueryRow(ctx, `
				INSERT INTO career_applications (
					id, org_id, profile_id, company, role, listing_url, status, score, via, agency, employer
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
				RETURNING id, created_at, updated_at`,
				a.ID, a.OrgID, a.ProfileID, a.Company, a.Role, a.ListingURL, a.Status, a.Score, a.Via, a.Agency, a.Employer,
			).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
		}
		return fmt.Errorf("career_repo: upsert application: %w", err)
	}
	return nil
}

func scanApp(row pgx.Row) (*model.CareerApplication, error) {
	a := &model.CareerApplication{}
	err := row.Scan(&a.ID, &a.OrgID, &a.ProfileID, &a.Company, &a.Role, &a.ListingURL, &a.Status, &a.Score, &a.Via, &a.Agency, &a.Employer, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

const appCols = `id, org_id, profile_id, company, role, listing_url, status, score, via, agency, employer, created_at, updated_at`

func (r *careerRepository) GetApplication(ctx context.Context, orgID, id uuid.UUID) (*model.CareerApplication, error) {
	a, err := scanApp(r.pool.QueryRow(ctx, `SELECT `+appCols+` FROM career_applications WHERE org_id=$1 AND id=$2`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (r *careerRepository) GetApplicationByURL(ctx context.Context, profileID uuid.UUID, listingURL string) (*model.CareerApplication, error) {
	if listingURL == "" {
		return nil, nil
	}
	a, err := scanApp(r.pool.QueryRow(ctx, `SELECT `+appCols+` FROM career_applications WHERE profile_id=$1 AND listing_url=$2`, profileID, listingURL))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (r *careerRepository) ListApplications(ctx context.Context, orgID, profileID uuid.UUID, status string, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerApplication], error) {
	pagination.Normalize()
	args := []any{orgID, profileID}
	where := `org_id=$1 AND profile_id=$2`
	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(` AND status=$%d`, len(args))
	}
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM career_applications WHERE `+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	args = append(args, pagination.PerPage, pagination.Offset())
	rows, err := r.pool.Query(ctx, `
		SELECT `+appCols+` FROM career_applications WHERE `+where+`
		ORDER BY updated_at DESC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.CareerApplication{}
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *a)
	}
	return paginate(items, total, pagination), rows.Err()
}

func (r *careerRepository) UpdateApplication(ctx context.Context, a *model.CareerApplication) error {
	return r.pool.QueryRow(ctx, `
		UPDATE career_applications SET company=$2, role=$3, listing_url=$4, status=$5, score=$6,
			via=$7, agency=$8, employer=$9, updated_at=NOW() WHERE id=$1
		RETURNING updated_at`,
		a.ID, a.Company, a.Role, a.ListingURL, a.Status, a.Score, a.Via, a.Agency, a.Employer,
	).Scan(&a.UpdatedAt)
}

func (r *careerRepository) AppendStatusEvent(ctx context.Context, e *model.CareerStatusEvent) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO career_status_events (id, org_id, profile_id, application_id, from_status, to_status, note)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING created_at`,
		e.ID, e.OrgID, e.ProfileID, e.ApplicationID, e.FromStatus, e.ToStatus, e.Note,
	).Scan(&e.CreatedAt)
}

func (r *careerRepository) ListStatusEvents(ctx context.Context, applicationID uuid.UUID) ([]model.CareerStatusEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, profile_id, application_id, from_status, to_status, note, created_at
		FROM career_status_events WHERE application_id=$1 ORDER BY created_at`, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CareerStatusEvent
	for rows.Next() {
		var e model.CareerStatusEvent
		if err := rows.Scan(&e.ID, &e.OrgID, &e.ProfileID, &e.ApplicationID, &e.FromStatus, &e.ToStatus, &e.Note, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if out == nil {
		out = []model.CareerStatusEvent{}
	}
	return out, rows.Err()
}

func (r *careerRepository) InsertEvaluation(ctx context.Context, e *model.CareerEvaluation) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO career_evaluations (
			id, org_id, profile_id, application_id, pipeline_item_id, listing_url, jd_text,
			company, role, blocks, score, report_markdown, legitimacy_tier, hard_stop, hard_stop_reason, mode
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING created_at`,
		e.ID, e.OrgID, e.ProfileID, e.ApplicationID, e.PipelineItemID, e.ListingURL, e.JDText,
		e.Company, e.Role, marshalJSON(e.Blocks), marshalJSON(e.Score), e.ReportMarkdown,
		e.LegitimacyTier, e.HardStop, e.HardStopReason, e.Mode,
	).Scan(&e.CreatedAt)
	if err != nil {
		return fmt.Errorf("career_repo: insert evaluation: %w", err)
	}
	return nil
}

func scanEval(row pgx.Row) (*model.CareerEvaluation, error) {
	e := &model.CareerEvaluation{}
	var blocks, score []byte
	err := row.Scan(
		&e.ID, &e.OrgID, &e.ProfileID, &e.ApplicationID, &e.PipelineItemID, &e.ListingURL, &e.JDText,
		&e.Company, &e.Role, &blocks, &score, &e.ReportMarkdown, &e.LegitimacyTier, &e.HardStop, &e.HardStopReason, &e.Mode, &e.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(blocks, &e.Blocks)
	_ = json.Unmarshal(score, &e.Score)
	return e, nil
}

const evalCols = `id, org_id, profile_id, application_id, pipeline_item_id, listing_url, jd_text,
	company, role, blocks, score, report_markdown, legitimacy_tier, hard_stop, hard_stop_reason, mode, created_at`

func (r *careerRepository) GetEvaluation(ctx context.Context, orgID, id uuid.UUID) (*model.CareerEvaluation, error) {
	e, err := scanEval(r.pool.QueryRow(ctx, `SELECT `+evalCols+` FROM career_evaluations WHERE org_id=$1 AND id=$2`, orgID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

func (r *careerRepository) ListEvaluations(ctx context.Context, orgID, profileID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerEvaluation], error) {
	pagination.Normalize()
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM career_evaluations WHERE org_id=$1 AND profile_id=$2`, orgID, profileID).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+evalCols+` FROM career_evaluations WHERE org_id=$1 AND profile_id=$2
		ORDER BY created_at DESC LIMIT $3 OFFSET $4`, orgID, profileID, pagination.PerPage, pagination.Offset())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.CareerEvaluation{}
	for rows.Next() {
		e, err := scanEval(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *e)
	}
	return paginate(items, total, pagination), rows.Err()
}

func (r *careerRepository) InsertArtifact(ctx context.Context, a *model.CareerArtifact) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO career_artifacts (id, org_id, profile_id, application_id, evaluation_id, kind, title, body_markdown, file_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING created_at`,
		a.ID, a.OrgID, a.ProfileID, a.ApplicationID, a.EvaluationID, a.Kind, a.Title, a.BodyMarkdown, a.FileID,
	).Scan(&a.CreatedAt)
}

func (r *careerRepository) ListArtifacts(ctx context.Context, applicationID uuid.UUID) ([]model.CareerArtifact, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, profile_id, application_id, evaluation_id, kind, title, body_markdown, file_id,
			(file_bytes IS NOT NULL AND octet_length(file_bytes) > 0), created_at
		FROM career_artifacts WHERE application_id=$1 ORDER BY created_at DESC`, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CareerArtifact
	for rows.Next() {
		var a model.CareerArtifact
		if err := rows.Scan(&a.ID, &a.OrgID, &a.ProfileID, &a.ApplicationID, &a.EvaluationID, &a.Kind, &a.Title, &a.BodyMarkdown, &a.FileID, &a.HasPDF, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if out == nil {
		out = []model.CareerArtifact{}
	}
	return out, rows.Err()
}

func (r *careerRepository) ListStories(ctx context.Context, orgID, profileID uuid.UUID) ([]model.CareerStory, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, profile_id, title, situation, task, action, result, reflection, provenance, tags, created_at, updated_at
		FROM career_stories WHERE org_id=$1 AND profile_id=$2 ORDER BY updated_at DESC`, orgID, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CareerStory
	for rows.Next() {
		var s model.CareerStory
		var tags []string
		if err := rows.Scan(&s.ID, &s.OrgID, &s.ProfileID, &s.Title, &s.Situation, &s.Task, &s.Action, &s.Result, &s.Reflection, &s.Provenance, &tags, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.Tags = nzStrings(tags)
		out = append(out, s)
	}
	if out == nil {
		out = []model.CareerStory{}
	}
	return out, rows.Err()
}

func (r *careerRepository) GetStoryByID(ctx context.Context, id uuid.UUID) (*model.CareerStory, error) {
	s := &model.CareerStory{}
	var tags []string
	err := r.pool.QueryRow(ctx, `
		SELECT id, org_id, profile_id, title, situation, task, action, result, reflection, provenance, tags, created_at, updated_at
		FROM career_stories WHERE id=$1`, id).Scan(
		&s.ID, &s.OrgID, &s.ProfileID, &s.Title, &s.Situation, &s.Task, &s.Action, &s.Result, &s.Reflection, &s.Provenance, &tags, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("career_repo: get story: %w", err)
	}
	s.Tags = nzStrings(tags)
	return s, nil
}

func (r *careerRepository) UpsertStory(ctx context.Context, s *model.CareerStory) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.Provenance == "" {
		s.Provenance = model.CareerStoryUser
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO career_stories (id, org_id, profile_id, title, situation, task, action, result, reflection, provenance, tags)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO UPDATE SET
			title=EXCLUDED.title, situation=EXCLUDED.situation, task=EXCLUDED.task, action=EXCLUDED.action,
			result=EXCLUDED.result, reflection=EXCLUDED.reflection, provenance=EXCLUDED.provenance, tags=EXCLUDED.tags,
			updated_at=NOW()
		WHERE career_stories.org_id = EXCLUDED.org_id
		  AND career_stories.profile_id = EXCLUDED.profile_id
		RETURNING created_at, updated_at`,
		s.ID, s.OrgID, s.ProfileID, s.Title, s.Situation, s.Task, s.Action, s.Result, s.Reflection, s.Provenance, nzStrings(s.Tags),
	).Scan(&s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("career_repo: upsert story: %w", errors.New("not found"))
	}
	return err
}

func (r *careerRepository) InsertScanRun(ctx context.Context, run *model.CareerScanRun) error {
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	if run.Status == "" {
		run.Status = "running"
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO career_scan_runs (id, org_id, profile_id, status, added, skipped, error_message)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING created_at`,
		run.ID, run.OrgID, run.ProfileID, run.Status, run.Added, run.Skipped, run.ErrorMessage,
	).Scan(&run.CreatedAt)
}

func (r *careerRepository) UpdateScanRun(ctx context.Context, run *model.CareerScanRun) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE career_scan_runs SET status=$2, added=$3, skipped=$4, error_message=$5, completed_at=$6 WHERE id=$1`,
		run.ID, run.Status, run.Added, run.Skipped, run.ErrorMessage, run.CompletedAt)
	return err
}

func (r *careerRepository) HasScanEvent(ctx context.Context, profileID uuid.UUID, listingURL string) (bool, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM career_scan_events WHERE profile_id=$1 AND listing_url=$2`, profileID, listingURL).Scan(&n)
	return n > 0, err
}

func (r *careerRepository) InsertScanEvent(ctx context.Context, orgID, profileID uuid.UUID, listingURL, company, title string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO career_scan_events (org_id, profile_id, listing_url, company, title)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (profile_id, listing_url) DO UPDATE SET seen_at=NOW()`,
		orgID, profileID, listingURL, company, title)
	return err
}

func (r *careerRepository) InsertFollowup(ctx context.Context, f *model.CareerFollowup) error {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO career_followups (id, org_id, profile_id, application_id, due_at, kind, draft, sent)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING created_at`,
		f.ID, f.OrgID, f.ProfileID, f.ApplicationID, f.DueAt, f.Kind, f.Draft, f.Sent,
	).Scan(&f.CreatedAt)
}

func (r *careerRepository) ListFollowups(ctx context.Context, orgID, profileID uuid.UUID) ([]model.CareerFollowup, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, profile_id, application_id, due_at, kind, draft, sent, created_at
		FROM career_followups WHERE org_id=$1 AND profile_id=$2 ORDER BY due_at`, orgID, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CareerFollowup
	for rows.Next() {
		var f model.CareerFollowup
		if err := rows.Scan(&f.ID, &f.OrgID, &f.ProfileID, &f.ApplicationID, &f.DueAt, &f.Kind, &f.Draft, &f.Sent, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if out == nil {
		out = []model.CareerFollowup{}
	}
	return out, rows.Err()
}

func (r *careerRepository) GetLatestEvaluationForApp(ctx context.Context, orgID, applicationID uuid.UUID) (*model.CareerEvaluation, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id FROM career_evaluations
		WHERE org_id=$1 AND application_id=$2
		ORDER BY created_at DESC LIMIT 1`, orgID, applicationID)
	var id uuid.UUID
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return r.GetEvaluation(ctx, orgID, id)
}

func (r *careerRepository) InsertContact(ctx context.Context, c *model.CareerContact) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO career_contacts (id, org_id, profile_id, application_id, name, role, company, email, linkedin_url, note, linkedin_draft)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING created_at`,
		c.ID, c.OrgID, c.ProfileID, c.ApplicationID, c.Name, c.Role, c.Company, c.Email, c.LinkedInURL, c.Note, c.LinkedInDraft,
	).Scan(&c.CreatedAt)
}

func (r *careerRepository) ListContacts(ctx context.Context, orgID, profileID uuid.UUID) ([]model.CareerContact, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, org_id, profile_id, application_id, name, role, company, email, linkedin_url, note, linkedin_draft, created_at
		FROM career_contacts WHERE org_id=$1 AND profile_id=$2 ORDER BY created_at DESC`, orgID, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CareerContact
	for rows.Next() {
		var c model.CareerContact
		if err := rows.Scan(&c.ID, &c.OrgID, &c.ProfileID, &c.ApplicationID, &c.Name, &c.Role, &c.Company, &c.Email, &c.LinkedInURL, &c.Note, &c.LinkedInDraft, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if out == nil {
		out = []model.CareerContact{}
	}
	return out, rows.Err()
}

func (r *careerRepository) InsertOffer(ctx context.Context, o *model.CareerOffer) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	if o.Clauses == nil {
		o.Clauses = map[string]any{}
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO career_offers (id, org_id, profile_id, application_id, clauses, notes)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING created_at`,
		o.ID, o.OrgID, o.ProfileID, o.ApplicationID, marshalJSON(o.Clauses), o.Notes,
	).Scan(&o.CreatedAt)
}

func (r *careerRepository) InsertSalaryObservation(ctx context.Context, o *model.CareerSalaryObservation) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO career_salary_observations (id, org_id, profile_id, application_id, desired, advertised, actual)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING created_at`,
		o.ID, o.OrgID, o.ProfileID, o.ApplicationID, o.Desired, o.Advertised, o.Actual,
	).Scan(&o.CreatedAt)
}

func paginate[T any](data []T, total int, p model.PaginationParams) *model.PaginatedResponse[T] {
	pages := 0
	if p.PerPage > 0 {
		pages = (total + p.PerPage - 1) / p.PerPage
	}
	return &model.PaginatedResponse[T]{Data: data, Total: total, Page: p.Page, PerPage: p.PerPage, TotalPages: pages}
}
