// Package tasklaunch is the single place Task Manager and chat start an agent.
// It always creates or updates a board task, then dispatches via the specialist
// registry.
package tasklaunch

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
	"github.com/jobshout/server/internal/service"
)

// Request is what UI and chat pass after the agent (and project) are known.
type Request struct {
	OrgID     uuid.UUID
	UserID    uuid.UUID
	AgentID   uuid.UUID
	ProjectID uuid.UUID
	TaskID    *uuid.UUID
	Values    map[string]string
	Source    string // "task_manager" | "chat"
}

// Result is returned to the HTTP handler and chat tools.
type Result struct {
	Task         *model.Task     `json:"task"`
	Kind         string          `json:"kind"`
	RunID        *uuid.UUID      `json:"run_id,omitempty"`
	SyncQueued   bool            `json:"sync_queued,omitempty"`
	Brief        *research.Brief `json:"brief,omitempty"`
	ImageURL     string          `json:"image_url,omitempty"`
	EvaluationID *uuid.UUID      `json:"evaluation_id,omitempty"`
	Message      string          `json:"message,omitempty"`
}

// Service dispatches launches. Specialist work lives on the registered module
// (builtin → Launch), not on fields of this struct.
//
// All specialists are wired this way: register Launch on the module. A new
// agent does not need a case here — register it, do not add a switch.
type Service struct {
	Agents   service.AgentService
	Tasks    service.TaskService
	Projects service.ProjectService
	TaskRuns service.TaskRunService
}

// LaunchFromChat resolves the project (interview if the org has 2+) then Launch.
func (s *Service) LaunchFromChat(ctx context.Context, orgID, userID uuid.UUID, agent *model.Agent, values map[string]string, lastProjectID string) (*Result, *ProjectDecision, error) {
	if values == nil {
		values = map[string]string{}
	}
	hint := strings.TrimSpace(values["project"])
	dec, err := s.ResolveProject(ctx, orgID, hint, lastProjectID)
	if err != nil {
		return nil, nil, err
	}
	if dec.Missing != "" {
		return nil, &dec, nil
	}
	res, err := s.Launch(ctx, Request{
		OrgID:     orgID,
		UserID:    userID,
		AgentID:   agent.ID,
		ProjectID: dec.ProjectID,
		Values:    values,
		Source:    "chat",
	})
	return res, nil, err
}

// Launch creates or updates the board task and starts the agent.
//
// All specialists are wired this way: lookup Launch on the module.
// A new agent does not need a case here — register it, do not add a switch.
func (s *Service) Launch(ctx context.Context, req Request) (*Result, error) {
	if req.Values == nil {
		req.Values = map[string]string{}
	}
	if req.Source == "" {
		req.Source = "task_manager"
	}

	agent, err := s.Agents.GetByID(ctx, req.AgentID)
	if err != nil || agent == nil {
		return nil, fmt.Errorf("agent not found")
	}
	if agent.OrgID != req.OrgID {
		return nil, fmt.Errorf("agent not found")
	}

	builtin := agentschema.BuiltinOf(agent)
	kind := builtin
	if kind == "" {
		kind = "task_run"
	}

	schema := agentschema.ForBuiltin(builtin)
	req.Values = schema.ApplyDefaults(req.Values)
	title, desc := agentschema.TitleFrom(schema, req.Values)
	agentID := agent.ID.String()
	meta := map[string]any{
		model.TaskMetaLaunchValues: valuesAsAny(req.Values),
		model.TaskMetaLaunchKind:   kind,
	}

	var task *model.Task
	if req.TaskID != nil {
		task, err = s.Tasks.GetByID(ctx, *req.TaskID)
		if err != nil {
			return nil, err
		}
		upd := model.UpdateTaskRequest{
			Title:           &title,
			AssignedAgentID: model.OptionalString{Set: true, Value: &agentID},
			Metadata:        mergeTaskMeta(task.Metadata, meta),
		}
		if desc != "" {
			upd.Description = &desc
		}
		task, err = s.Tasks.Update(ctx, task.ID, upd)
		if err != nil {
			return nil, err
		}
	} else {
		if req.ProjectID == uuid.Nil {
			return nil, fmt.Errorf("project is required")
		}
		var descPtr *string
		if desc != "" {
			descPtr = &desc
		}
		task, err = s.Tasks.Create(ctx, req.UserID, model.CreateTaskRequest{
			ProjectID:       req.ProjectID.String(),
			Title:           title,
			Description:     descPtr,
			AssignedAgentID: &agentID,
			Metadata:        meta,
		})
		if err != nil {
			return nil, err
		}
	}

	_ = s.Tasks.Transition(ctx, task.ID, "in_progress", nil)
	task.Status = "in_progress"

	out := &Result{Task: task, Kind: kind}

	mod, ok := agentmodule.Lookup(kind)
	if ok && mod.Launch != nil {
		lo, lerr := mod.Launch(ctx, agentmodule.LaunchInput{
			OrgID:  req.OrgID,
			UserID: req.UserID,
			Agent:  agent,
			Task:   task,
			Values: req.Values,
			Source: req.Source,
		})
		if lerr != nil {
			return nil, lerr
		}
		applyLaunch(s, ctx, task, meta, out, lo)
		return out, nil
	}

	if s.TaskRuns == nil {
		return nil, fmt.Errorf("task runs are not configured")
	}
	prompt := strings.TrimSpace(req.Values["prompt"])
	if prompt == "" {
		prompt = strings.TrimSpace(req.Values["title"])
		if d := strings.TrimSpace(req.Values["description"]); d != "" {
			if prompt != "" {
				prompt += "\n\n" + d
			} else {
				prompt = d
			}
		}
	}
	run, rerr := s.TaskRuns.CreateRun(ctx, task.ID, model.CreateTaskRunRequest{
		AgentID: &agent.ID,
		Prompt:  &prompt,
	}, req.OrgID, &req.UserID)
	if rerr != nil {
		return nil, rerr
	}
	id := run.ID
	out.RunID = &id
	out.Kind = "task_run"
	out.Message = "Agent run started"
	meta[model.TaskMetaRunID] = id.String()
	task, _ = s.Tasks.Update(ctx, task.ID, model.UpdateTaskRequest{Metadata: mergeTaskMeta(task.Metadata, meta)})
	if task != nil {
		out.Task = task
	}
	if out.Task != nil && out.Task.Metadata == nil {
		out.Task.Metadata = meta
	}
	return out, nil
}

func applyLaunch(s *Service, ctx context.Context, task *model.Task, meta map[string]any, out *Result, lo *agentmodule.LaunchOutput) {
	if lo == nil {
		return
	}
	out.Message = lo.Message
	out.SyncQueued = lo.SyncQueued
	out.ImageURL = lo.ImageURL
	out.RunID = lo.RunID
	out.EvaluationID = lo.EvaluationID
	if b, ok := lo.Brief.(*research.Brief); ok {
		out.Brief = b
	}
	for k, v := range lo.ExtraMeta {
		meta[k] = v
	}
	if lo.RunID != nil {
		meta[model.TaskMetaRunID] = lo.RunID.String()
	}

	upd := model.UpdateTaskRequest{Metadata: mergeTaskMeta(task.Metadata, meta)}
	if strings.TrimSpace(lo.Description) != "" {
		d := lo.Description
		upd.Description = &d
	}
	updated, _ := s.Tasks.Update(ctx, task.ID, upd)
	status := lo.Status
	if status == "" {
		status = "in_progress"
	}
	_ = s.Tasks.Transition(ctx, task.ID, status, nil)
	if updated != nil {
		updated.Status = status
		out.Task = updated
	} else if task != nil {
		task.Status = status
		out.Task = task
	}
}

func valuesAsAny(v map[string]string) map[string]any {
	out := make(map[string]any, len(v))
	for k, val := range v {
		out[k] = val
	}
	return out
}

func mergeTaskMeta(existing map[string]any, extra map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
