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

type LinuxPatchRunRepository interface {
	Create(ctx context.Context, run *model.LinuxPatchRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.LinuxPatchRun, error)
	Update(ctx context.Context, run *model.LinuxPatchRun) error
	ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.LinuxPatchRun], error)
}

type linuxPatchRunRepository struct{ pool *pgxpool.Pool }

func NewLinuxPatchRunRepository(pool *pgxpool.Pool) LinuxPatchRunRepository {
	return &linuxPatchRunRepository{pool: pool}
}

const linuxPatchRunColumns = `
	id, agent_id, task_id, org_id, status, mode, workload, hosts, ssh_user,
	pre_script, post_script, services, reboot_policy, dry_run, use_llm,
	schedule_after, vault_mount, vault_path, vault_key_field, instruction,
	phases, host_results, plan, error_message,
	requested_by, started_at, completed_at, created_at, updated_at`

func scanLinuxPatchRun(row pgx.Row) (*model.LinuxPatchRun, error) {
	run := &model.LinuxPatchRun{}
	var phasesJSON, hostsJSON, planJSON []byte
	if err := row.Scan(
		&run.ID, &run.AgentID, &run.TaskID, &run.OrgID, &run.Status, &run.Mode,
		&run.Workload, &run.Hosts, &run.SSHUser, &run.PreScript, &run.PostScript,
		&run.Services, &run.RebootPolicy, &run.DryRun, &run.UseLLM, &run.ScheduleAfter,
		&run.VaultMount, &run.VaultPath, &run.VaultKeyField, &run.Instruction,
		&phasesJSON, &hostsJSON, &planJSON, &run.ErrorMessage,
		&run.RequestedBy, &run.StartedAt, &run.CompletedAt, &run.CreatedAt, &run.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(phasesJSON) > 0 {
		_ = json.Unmarshal(phasesJSON, &run.Phases)
	}
	if len(hostsJSON) > 0 {
		_ = json.Unmarshal(hostsJSON, &run.HostResults)
	}
	if len(planJSON) > 0 {
		var p model.LinuxPatchPlan
		if json.Unmarshal(planJSON, &p) == nil {
			run.Plan = &p
		}
	}
	return run, nil
}

func (r *linuxPatchRunRepository) Create(ctx context.Context, run *model.LinuxPatchRun) error {
	phases, hosts, plan, err := marshalLinuxPatch(run)
	if err != nil {
		return err
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO linux_patch_runs (
			id, agent_id, task_id, org_id, status, mode, workload, hosts, ssh_user,
			pre_script, post_script, services, reboot_policy, dry_run, use_llm,
			schedule_after, vault_mount, vault_path, vault_key_field, instruction,
			phases, host_results, plan, error_message,
			requested_by, started_at, completed_at, created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,NOW(),NOW()
		) RETURNING created_at, updated_at`,
		run.ID, run.AgentID, run.TaskID, run.OrgID, run.Status, run.Mode, run.Workload,
		run.Hosts, run.SSHUser, run.PreScript, run.PostScript, run.Services, run.RebootPolicy,
		run.DryRun, run.UseLLM, run.ScheduleAfter, run.VaultMount, run.VaultPath, run.VaultKeyField,
		run.Instruction, phases, hosts, plan, run.ErrorMessage, run.RequestedBy, run.StartedAt, run.CompletedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *linuxPatchRunRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.LinuxPatchRun, error) {
	run, err := scanLinuxPatchRun(r.pool.QueryRow(ctx, `SELECT `+linuxPatchRunColumns+` FROM linux_patch_runs WHERE id=$1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("linux patch run not found")
		}
		return nil, err
	}
	return run, nil
}

func (r *linuxPatchRunRepository) Update(ctx context.Context, run *model.LinuxPatchRun) error {
	phases, hosts, plan, err := marshalLinuxPatch(run)
	if err != nil {
		return err
	}
	return r.pool.QueryRow(ctx, `
		UPDATE linux_patch_runs SET
			status=$2, mode=$3, workload=$4, hosts=$5, ssh_user=$6, pre_script=$7, post_script=$8,
			services=$9, reboot_policy=$10, dry_run=$11, use_llm=$12, schedule_after=$13,
			vault_mount=$14, vault_path=$15, vault_key_field=$16, instruction=$17,
			phases=$18, host_results=$19, plan=$20, error_message=$21,
			started_at=$22, completed_at=$23, updated_at=NOW()
		WHERE id=$1 RETURNING created_at, updated_at`,
		run.ID, run.Status, run.Mode, run.Workload, run.Hosts, run.SSHUser, run.PreScript,
		run.PostScript, run.Services, run.RebootPolicy, run.DryRun, run.UseLLM,
		run.ScheduleAfter, run.VaultMount, run.VaultPath, run.VaultKeyField, run.Instruction,
		phases, hosts, plan, run.ErrorMessage, run.StartedAt, run.CompletedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *linuxPatchRunRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.LinuxPatchRun], error) {
	pagination.Normalize()
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM linux_patch_runs WHERE org_id=$1`, orgID).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+linuxPatchRunColumns+` FROM linux_patch_runs WHERE org_id=$1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, pagination.PerPage, pagination.Offset())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]model.LinuxPatchRun, 0, pagination.PerPage)
	for rows.Next() {
		run, err := scanLinuxPatchRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *run)
	}
	pages := 1
	if pagination.PerPage > 0 {
		pages = (total + pagination.PerPage - 1) / pagination.PerPage
	}
	return &model.PaginatedResponse[model.LinuxPatchRun]{
		Data: runs, Total: total, Page: pagination.Page, PerPage: pagination.PerPage, TotalPages: pages,
	}, nil
}

func marshalLinuxPatch(run *model.LinuxPatchRun) (phases, hosts, plan []byte, err error) {
	if run.Phases != nil {
		phases, err = json.Marshal(run.Phases)
		if err != nil {
			return
		}
	}
	if run.HostResults != nil {
		hosts, err = json.Marshal(run.HostResults)
		if err != nil {
			return
		}
	}
	if run.Plan != nil {
		plan, err = json.Marshal(run.Plan)
	}
	return
}
