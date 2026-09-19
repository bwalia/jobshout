package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

// SecurityFindingEventRepository persists cross-run detect/fix history.
type SecurityFindingEventRepository interface {
	InsertMany(ctx context.Context, events []model.SecurityFindingEvent) error
	ListByRun(ctx context.Context, runID uuid.UUID) ([]model.SecurityFindingEvent, error)
	ListBySubject(ctx context.Context, orgID uuid.UUID, agentKind, subjectKey string, limit int) ([]model.SecurityFindingEvent, error)
}

type securityFindingEventRepository struct {
	pool *pgxpool.Pool
}

// NewSecurityFindingEventRepository constructs a Postgres-backed store.
func NewSecurityFindingEventRepository(pool *pgxpool.Pool) SecurityFindingEventRepository {
	return &securityFindingEventRepository{pool: pool}
}

func (r *securityFindingEventRepository) InsertMany(ctx context.Context, events []model.SecurityFindingEvent) error {
	if len(events) == 0 {
		return nil
	}
	const q = `
		INSERT INTO security_finding_events (
			id, org_id, agent_kind, subject_key, finding_key, event, run_id,
			report_seq, app_version, severity, title, detail, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NOW())`
	for i := range events {
		e := &events[i]
		if e.ID == uuid.Nil {
			e.ID = uuid.New()
		}
		if _, err := r.pool.Exec(ctx, q,
			e.ID, e.OrgID, e.AgentKind, e.SubjectKey, e.FindingKey, e.Event, e.RunID,
			e.ReportSeq, e.AppVersion, nullEmpty(e.Severity), e.Title, e.Detail,
		); err != nil {
			return fmt.Errorf("insert security finding event: %w", err)
		}
	}
	return nil
}

func (r *securityFindingEventRepository) ListByRun(ctx context.Context, runID uuid.UUID) ([]model.SecurityFindingEvent, error) {
	const q = `
		SELECT id, org_id, agent_kind, subject_key, finding_key, event, run_id,
		       report_seq, app_version, COALESCE(severity,''), title, COALESCE(detail,''), created_at
		FROM security_finding_events
		WHERE run_id = $1
		ORDER BY created_at ASC, title ASC`
	rows, err := r.pool.Query(ctx, q, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFindingEvents(rows)
}

func (r *securityFindingEventRepository) ListBySubject(ctx context.Context, orgID uuid.UUID, agentKind, subjectKey string, limit int) ([]model.SecurityFindingEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	const q = `
		SELECT id, org_id, agent_kind, subject_key, finding_key, event, run_id,
		       report_seq, app_version, COALESCE(severity,''), title, COALESCE(detail,''), created_at
		FROM security_finding_events
		WHERE org_id = $1 AND agent_kind = $2 AND lower(trim(subject_key)) = lower(trim($3))
		ORDER BY created_at DESC
		LIMIT $4`
	rows, err := r.pool.Query(ctx, q, orgID, agentKind, subjectKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFindingEvents(rows)
}

func scanFindingEvents(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]model.SecurityFindingEvent, error) {
	out := make([]model.SecurityFindingEvent, 0)
	for rows.Next() {
		var e model.SecurityFindingEvent
		if err := rows.Scan(
			&e.ID, &e.OrgID, &e.AgentKind, &e.SubjectKey, &e.FindingKey, &e.Event, &e.RunID,
			&e.ReportSeq, &e.AppVersion, &e.Severity, &e.Title, &e.Detail, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func nullEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}
