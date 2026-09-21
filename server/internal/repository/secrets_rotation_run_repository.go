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

// SecretsRotationRunRepository persists secrets rotation runs.
type SecretsRotationRunRepository interface {
	Create(ctx context.Context, run *model.SecretsRotationRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.SecretsRotationRun, error)
	Update(ctx context.Context, run *model.SecretsRotationRun) error
	ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.SecretsRotationRun], error)
}

type secretsRotationRunRepository struct{ pool *pgxpool.Pool }

// NewSecretsRotationRunRepository constructs a Postgres-backed store.
func NewSecretsRotationRunRepository(pool *pgxpool.Pool) SecretsRotationRunRepository {
	return &secretsRotationRunRepository{pool: pool}
}

const secretsRotRunColumns = `
	id, agent_id, task_id, org_id, status, mode, provider, detected_provider,
	vault_addr, mount, path, engine, keys, grace_seconds, retire_old, dry_run,
	instruction, phases, result, error_message, requested_by,
	started_at, completed_at, created_at, updated_at`

func scanSecretsRotRun(row pgx.Row) (*model.SecretsRotationRun, error) {
	run := &model.SecretsRotationRun{}
	var phasesJSON, resultJSON []byte
	if err := row.Scan(
		&run.ID, &run.AgentID, &run.TaskID, &run.OrgID, &run.Status, &run.Mode,
		&run.Provider, &run.DetectedProv, &run.VaultAddr, &run.Mount, &run.Path,
		&run.Engine, &run.Keys, &run.GraceSeconds, &run.RetireOld, &run.DryRun,
		&run.Instruction, &phasesJSON, &resultJSON, &run.ErrorMessage, &run.RequestedBy,
		&run.StartedAt, &run.CompletedAt, &run.CreatedAt, &run.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(phasesJSON) > 0 {
		_ = json.Unmarshal(phasesJSON, &run.Phases)
	}
	if len(resultJSON) > 0 {
		var r model.SecretsRotationResult
		if json.Unmarshal(resultJSON, &r) == nil {
			run.Result = &r
		}
	}
	return run, nil
}

func (r *secretsRotationRunRepository) Create(ctx context.Context, run *model.SecretsRotationRun) error {
	phases, result, err := marshalSecretsRotExtras(run)
	if err != nil {
		return err
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO secrets_rotation_runs (
			id, agent_id, task_id, org_id, status, mode, provider, detected_provider,
			vault_addr, mount, path, engine, keys, grace_seconds, retire_old, dry_run,
			instruction, phases, result, error_message, requested_by,
			started_at, completed_at, created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,NOW(),NOW()
		) RETURNING created_at, updated_at`,
		run.ID, run.AgentID, run.TaskID, run.OrgID, run.Status, run.Mode, run.Provider,
		run.DetectedProv, run.VaultAddr, run.Mount, run.Path, run.Engine, run.Keys,
		run.GraceSeconds, run.RetireOld, run.DryRun, run.Instruction, phases, result,
		run.ErrorMessage, run.RequestedBy, run.StartedAt, run.CompletedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *secretsRotationRunRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.SecretsRotationRun, error) {
	run, err := scanSecretsRotRun(r.pool.QueryRow(ctx, `SELECT `+secretsRotRunColumns+` FROM secrets_rotation_runs WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("secrets rotation run not found")
		}
		return nil, err
	}
	return run, nil
}

func (r *secretsRotationRunRepository) Update(ctx context.Context, run *model.SecretsRotationRun) error {
	phases, result, err := marshalSecretsRotExtras(run)
	if err != nil {
		return err
	}
	return r.pool.QueryRow(ctx, `
		UPDATE secrets_rotation_runs SET
			status=$2, mode=$3, provider=$4, detected_provider=$5, vault_addr=$6, mount=$7,
			path=$8, engine=$9, keys=$10, grace_seconds=$11, retire_old=$12, dry_run=$13,
			instruction=$14, phases=$15, result=$16, error_message=$17,
			started_at=$18, completed_at=$19, updated_at=NOW()
		WHERE id=$1 RETURNING created_at, updated_at`,
		run.ID, run.Status, run.Mode, run.Provider, run.DetectedProv, run.VaultAddr,
		run.Mount, run.Path, run.Engine, run.Keys, run.GraceSeconds, run.RetireOld,
		run.DryRun, run.Instruction, phases, result, run.ErrorMessage,
		run.StartedAt, run.CompletedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *secretsRotationRunRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.SecretsRotationRun], error) {
	pagination.Normalize()
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM secrets_rotation_runs WHERE org_id=$1`, orgID).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+secretsRotRunColumns+` FROM secrets_rotation_runs WHERE org_id=$1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, pagination.PerPage, pagination.Offset())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]model.SecretsRotationRun, 0, pagination.PerPage)
	for rows.Next() {
		run, err := scanSecretsRotRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *run)
	}
	pages := 1
	if pagination.PerPage > 0 {
		pages = (total + pagination.PerPage - 1) / pagination.PerPage
	}
	return &model.PaginatedResponse[model.SecretsRotationRun]{
		Data: runs, Total: total, Page: pagination.Page, PerPage: pagination.PerPage, TotalPages: pages,
	}, nil
}

func marshalSecretsRotExtras(run *model.SecretsRotationRun) (phases, result []byte, err error) {
	if run.Phases != nil {
		phases, err = json.Marshal(run.Phases)
		if err != nil {
			return nil, nil, err
		}
	}
	if run.Result != nil {
		result, err = json.Marshal(run.Result)
		if err != nil {
			return nil, nil, err
		}
	}
	return phases, result, nil
}
