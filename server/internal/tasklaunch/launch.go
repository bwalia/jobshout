// Package tasklaunch is the single place Task Manager and chat start an agent.
// It always creates or updates a board task, then dispatches the specialist.
package tasklaunch

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

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
	Task       *model.Task     `json:"task"`
	Kind       string          `json:"kind"`
	RunID      *uuid.UUID      `json:"run_id,omitempty"`
	SyncQueued bool            `json:"sync_queued,omitempty"`
	Brief      *research.Brief `json:"brief,omitempty"`
	ImageURL   string          `json:"image_url,omitempty"`
	Message    string          `json:"message,omitempty"`
}

// Service dispatches launches. Fields may be nil; the matching kind then errors.
type Service struct {
	Agents   service.AgentService
	Tasks    service.TaskService
	Projects service.ProjectService
	Research service.ResearchService
	Blog     service.BlogService
	Mail     service.MailService
	Pentest  service.PentestService
	Reviews  service.ReviewService
	Images   *service.ImageService
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

	title, desc := TitleFrom(kind, req.Values)
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
		if task.ProjectID != req.ProjectID && req.ProjectID != uuid.Nil {
			// Keep the existing project; the board card already lives there.
		}
		upd := model.UpdateTaskRequest{
			Title:           &title,
			AssignedAgentID: &agentID,
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

	_ = s.Tasks.Transition(ctx, task.ID, "in_progress")
	task.Status = "in_progress"

	out := &Result{Task: task, Kind: kind}
	switch kind {
	case model.BuiltinResearcher:
		if s.Research == nil || !s.Research.Available() {
			return nil, fmt.Errorf("research is not configured on this server")
		}
		brief, rerr := s.Research.Research(ctx, req.OrgID, research.Request{
			Topic:   strings.TrimSpace(req.Values["topic"]),
			Context: strings.TrimSpace(req.Values["context"]),
		}, nil)
		if rerr != nil {
			return nil, rerr
		}
		out.Brief = brief
		text := formatBrief(brief, desc)
		status := "done"
		task, _ = s.Tasks.Update(ctx, task.ID, model.UpdateTaskRequest{
			Description: &text,
			Metadata:    mergeTaskMeta(task.Metadata, meta),
		})
		_ = s.Tasks.Transition(ctx, task.ID, status)
		if task != nil {
			task.Status = status
			out.Task = task
		}
		out.Message = "Research complete"
	case model.BuiltinArticleWriter:
		if s.Blog == nil {
			return nil, fmt.Errorf("article writer is not configured")
		}
		uid := req.UserID
		tid := task.ID
		run, berr := s.Blog.Generate(ctx, req.OrgID, &uid, req.Source, model.GenerateBlogRequest{
			Briefs: []model.BlogBrief{{
				Topic:   strings.TrimSpace(req.Values["topic"]),
				Context: strings.TrimSpace(req.Values["context"]),
			}},
			Model:  strings.TrimSpace(req.Values["model"]),
			TaskID: &tid,
		})
		if berr != nil {
			return nil, berr
		}
		id := run.ID
		out.RunID = &id
		meta[model.TaskMetaRunID] = id.String()
		link := "Article run started. Open /articles/" + id.String() + " when it finishes."
		if desc != "" {
			link = desc + "\n\n" + link
		}
		task, _ = s.Tasks.Update(ctx, task.ID, model.UpdateTaskRequest{
			Description: &link,
			Metadata:    mergeTaskMeta(task.Metadata, meta),
		})
		if task != nil {
			out.Task = task
		}
		out.Message = "Article run started"
	case model.BuiltinMail:
		syncQueued, merr := s.launchMail(ctx, req)
		if merr != nil {
			return nil, merr
		}
		out.SyncQueued = syncQueued
		if syncQueued {
			out.Message = "Mailbox sync queued"
			note := "Mailbox sync queued. Drafts appear on Mail Agent. Nothing is sent until you Approve.\n\nOpen: /panel/task-manager?agent=mail"
			if desc != "" {
				note = desc + "\n\n" + note
			}
			status := "in_progress"
			task, _ = s.Tasks.Update(ctx, task.ID, model.UpdateTaskRequest{
				Description: &note,
				Metadata:    mergeTaskMeta(task.Metadata, meta),
			})
			_ = s.Tasks.Transition(ctx, task.ID, status)
			if task != nil {
				task.Status = status
				out.Task = task
			}
		} else {
			out.Message = "Playbook saved. Connect Gmail on Mail Agent to sync."
			note := "Playbook saved. Connect Gmail on Mail Agent to sync. Nothing is sent until you Approve."
			if desc != "" {
				note = desc + "\n\n" + note
			}
			done := "done"
			task, _ = s.Tasks.Update(ctx, task.ID, model.UpdateTaskRequest{
				Description: &note,
				Metadata:    mergeTaskMeta(task.Metadata, meta),
			})
			_ = s.Tasks.Transition(ctx, task.ID, done)
			if task != nil {
				task.Status = done
				out.Task = task
			}
		}
	case model.BuiltinPentester:
		if s.Pentest == nil {
			return nil, fmt.Errorf("security tester is not configured")
		}
		payload := model.CreatePentestRunRequest{
			AgentID:     agent.ID,
			TaskID:      &task.ID,
			Target:      strings.TrimSpace(req.Values["target"]),
			ScanMode:    strings.TrimSpace(req.Values["scan_mode"]),
			Instruction: strings.TrimSpace(req.Values["instruction"]),
		}
		if payload.ScanMode == "" {
			payload.ScanMode = "quick"
		}
		if raw := strings.TrimSpace(req.Values["max_budget"]); raw != "" {
			if n, perr := strconv.Atoi(raw); perr == nil {
				payload.MaxBudget = &n
			}
		}
		run, perr := s.Pentest.CreateRun(ctx, payload, req.OrgID, &req.UserID)
		if perr != nil {
			return nil, perr
		}
		id := run.ID
		out.RunID = &id
		meta[model.TaskMetaRunID] = id.String()
		task, _ = s.Tasks.Update(ctx, task.ID, model.UpdateTaskRequest{Metadata: mergeTaskMeta(task.Metadata, meta)})
		if task != nil {
			out.Task = task
		}
		out.Message = "Security scan queued"
	case model.BuiltinPRReviewer:
		if s.Reviews == nil {
			return nil, fmt.Errorf("PR review is not configured")
		}
		pr, _ := strconv.Atoi(strings.TrimSpace(req.Values["pr_number"]))
		dry := req.Values["dry_run"] != "false"
		run, rerr := s.Reviews.CreateRun(ctx, model.CreateReviewRunRequest{
			Repo:     strings.TrimSpace(req.Values["repo"]),
			PRNumber: pr,
			DryRun:   &dry,
			AgentID:  &agent.ID,
		}, req.OrgID, &req.UserID)
		if rerr != nil {
			return nil, rerr
		}
		id := run.ID
		out.RunID = &id
		meta[model.TaskMetaRunID] = id.String()
		task, _ = s.Tasks.Update(ctx, task.ID, model.UpdateTaskRequest{Metadata: mergeTaskMeta(task.Metadata, meta)})
		if task != nil {
			out.Task = task
		}
		out.Message = "PR review queued"
	case model.BuiltinImages:
		if s.Images == nil || !s.Images.Enabled() {
			return nil, fmt.Errorf("image generation is not configured")
		}
		res, ierr := s.Images.Generate(ctx, service.GenerateImageRequest{
			OrgID:  req.OrgID,
			UserID: &req.UserID,
			Prompt: strings.TrimSpace(req.Values["prompt"]),
			Source: "task_manager",
		})
		if ierr != nil {
			return nil, ierr
		}
		out.ImageURL = res.URL
		text := "Generated image"
		if res.URL != "" {
			text = "Generated image: " + res.URL
		}
		_ = s.Tasks.Transition(ctx, task.ID, "done")
		task, _ = s.Tasks.Update(ctx, task.ID, model.UpdateTaskRequest{Description: &text})
		if task != nil {
			task.Status = "done"
			out.Task = task
		}
		if res.RecordID != nil {
			out.RunID = res.RecordID
		}
		out.Message = "Image generated"
	default:
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
		meta[model.TaskMetaRunID] = id.String()
		task, _ = s.Tasks.Update(ctx, task.ID, model.UpdateTaskRequest{Metadata: mergeTaskMeta(task.Metadata, meta)})
		if task != nil {
			out.Task = task
		}
		out.Message = "Agent run started"
	}

	if out.Task != nil && out.Task.Metadata == nil {
		out.Task.Metadata = meta
	}
	return out, nil
}

func (s *Service) launchMail(ctx context.Context, req Request) (bool, error) {
	if s.Mail == nil {
		return false, fmt.Errorf("mail agent is not configured")
	}
	if !mailValuesBlank(req.Values) {
		patch := mailPatchFromValues(req.Values)
		if _, err := s.Mail.UpdateConnection(ctx, req.OrgID, patch); err != nil {
			return false, err
		}
	}
	if !s.Mail.Available(ctx, req.OrgID) {
		return false, nil
	}
	if err := s.Mail.EnqueueSync(ctx, req.OrgID); err != nil {
		return false, err
	}
	return true, nil
}

func mailValuesBlank(v map[string]string) bool {
	for _, k := range []string{"senders", "subject_prefixes", "labels", "knowledge_urls", "research_focus", "reply_instructions"} {
		if strings.TrimSpace(v[k]) != "" {
			return false
		}
	}
	return true
}

func mailPatchFromValues(v map[string]string) model.UpdateMailConnectionRequest {
	rules := model.MailWatchRules{
		Senders:         splitComma(v["senders"]),
		Labels:          splitComma(v["labels"]),
		SubjectPrefixes: splitComma(v["subject_prefixes"]),
	}
	urls := splitLines(v["knowledge_urls"])
	focus := strings.TrimSpace(v["research_focus"])
	style := strings.TrimSpace(v["reply_instructions"])
	return model.UpdateMailConnectionRequest{
		Rules:             &rules,
		KnowledgeURLs:     &urls,
		ResearchFocus:     &focus,
		ReplyInstructions: &style,
	}
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
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

func formatBrief(brief *research.Brief, prior string) string {
	var parts []string
	if strings.TrimSpace(prior) != "" {
		parts = append(parts, strings.TrimSpace(prior))
	}
	if brief == nil {
		return strings.Join(parts, "\n\n")
	}
	if strings.TrimSpace(brief.Summary) != "" {
		parts = append(parts, "## Summary\n\n"+strings.TrimSpace(brief.Summary))
	}
	if len(brief.Findings) > 0 {
		var lines []string
		for _, f := range brief.Findings {
			claim := strings.TrimSpace(f.Claim)
			if claim == "" {
				claim = "(finding)"
			}
			if f.SourceURL != "" {
				lines = append(lines, "- "+claim+" ([source]("+f.SourceURL+"))")
			} else {
				lines = append(lines, "- "+claim)
			}
		}
		parts = append(parts, "## Findings\n\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(parts, "\n\n")
}
