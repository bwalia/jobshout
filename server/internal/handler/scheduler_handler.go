package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type SchedulerHandler struct {
	repo       repository.SchedulerRepository
	validate   *validator.Validate
	cronParser cron.Parser
}

func NewSchedulerHandler(repo repository.SchedulerRepository) *SchedulerHandler {
	return &SchedulerHandler{
		repo:       repo,
		validate:   validator.New(),
		cronParser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor),
	}
}

// schedulePresets maps the friendly preset names exposed in the API to the
// 5-field cron expressions the scheduler runner already understands. Times
// are interpreted in the server's local timezone (good enough for the v1
// "schedule it for every morning" UX; per-task timezones can come later).
var schedulePresets = map[string]string{
	"every_midnight":     "0 0 * * *",
	"every_morning_9am":  "0 9 * * *",
	"every_morning_10am": "0 10 * * *",
	"hourly":             "0 * * * *",
	"every_15m":          "*/15 * * * *",
	"every_5m":           "*/5 * * * *",
	"weekdays_9am":       "0 9 * * 1-5",
	"weekly_monday_9am":  "0 9 * * 1",
}

func (h *SchedulerHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.repo.ListTasks(r.Context(), orgID, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list scheduled tasks")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}

func (h *SchedulerHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	userID, _ := uuid.Parse(middleware.GetUserID(r.Context()))

	var req model.CreateScheduledTaskRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}

	// Translate preset → cron up front so downstream code (and the DB) only
	// has to reason about cron/interval/once.
	cronExpr := req.CronExpression
	if req.SchedulePreset != nil && *req.SchedulePreset != "" {
		mapped, ok := schedulePresets[*req.SchedulePreset]
		if !ok {
			RespondError(w, http.StatusBadRequest, "unknown schedule_preset: "+*req.SchedulePreset)
			return
		}
		cronExpr = &mapped
		// A preset always implies cron; honour that even if the caller picked
		// "interval" or "once" by accident.
		req.ScheduleType = "cron"
	}

	t := &model.ScheduledTask{
		ID:              uuid.New(),
		OrgID:           orgID,
		Name:            req.Name,
		Description:     req.Description,
		TaskType:        req.TaskType,
		InputPrompt:     req.InputPrompt,
		InputJSON:       req.InputJSON,
		ModelOverride:   req.ModelOverride,
		ScheduleType:    req.ScheduleType,
		CronExpression:  cronExpr,
		IntervalSeconds: req.IntervalSeconds,
		SchedulePreset:  req.SchedulePreset,
		Status:          "active",
		MaxRuns:         req.MaxRuns,
		RetryOnFailure:  req.RetryOnFailure,
		MaxRetries:      req.MaxRetries,
		Priority:        req.Priority,
		Tags:            req.Tags,
		MaxReview:       req.MaxReview,
		CreatedBy:       &userID,
	}

	if t.Priority == "" {
		t.Priority = "medium"
	}
	if t.Tags == nil {
		t.Tags = []string{}
	}
	if t.InputJSON == nil {
		t.InputJSON = map[string]any{}
	}

	// A malformed id is the caller's mistake, not a zero UUID to insert.
	for _, ref := range []struct {
		name string
		in   *string
		out  **uuid.UUID
	}{
		{"agent_id", req.AgentID, &t.AgentID},
		{"workflow_id", req.WorkflowID, &t.WorkflowID},
		{"provider_config_id", req.ProviderConfigID, &t.ProviderConfigID},
		{"planner_id", req.PlannerID, &t.PlannerID},
		{"executor_id", req.ExecutorID, &t.ExecutorID},
		{"reviewer_id", req.ReviewerID, &t.ReviewerID},
	} {
		if ref.in == nil || *ref.in == "" {
			continue
		}
		id, perr := uuid.Parse(*ref.in)
		if perr != nil {
			RespondError(w, http.StatusBadRequest, "invalid "+ref.name)
			return
		}
		*ref.out = &id
	}

	// Without this a one-time task ignored the time it was given and fired on
	// the next tick.
	if t.RunAt, err = parseRunAt(req.RunAt); err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if t.TaskType == "multi_agent" {
		if t.PlannerID == nil || t.ExecutorID == nil || t.ReviewerID == nil {
			RespondError(w, http.StatusBadRequest, "multi_agent tasks require planner_id, executor_id, reviewer_id")
			return
		}
	}

	// Compute initial next_run_at so the task is actually picked up by the
	// dispatcher. Without this, recurring schedules sit idle forever because
	// the runner only advances next_run_at *after* a run.
	if next, err := h.computeInitialNextRun(t); err != nil {
		RespondError(w, http.StatusBadRequest, "schedule: "+err.Error())
		return
	} else {
		t.NextRunAt = next
	}

	if err := h.repo.CreateTask(r.Context(), t); err != nil {
		if errors.Is(err, repository.ErrScheduleTargetNotFound) {
			RespondError(w, http.StatusBadRequest, "that agent or workflow is not in this organization")
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to create: "+err.Error())
		return
	}

	RespondJSON(w, http.StatusCreated, t)
}

func (h *SchedulerHandler) computeInitialNextRun(t *model.ScheduledTask) (*time.Time, error) {
	now := time.Now()
	switch t.ScheduleType {
	case "cron":
		if t.CronExpression == nil || *t.CronExpression == "" {
			return nil, &validationErr{"cron_expression required for schedule_type=cron"}
		}
		sched, err := h.cronParser.Parse(*t.CronExpression)
		if err != nil {
			return nil, &validationErr{"invalid cron: " + err.Error()}
		}
		next := sched.Next(now)
		return &next, nil
	case "interval":
		if t.IntervalSeconds == nil || *t.IntervalSeconds <= 0 {
			return nil, &validationErr{"interval_seconds required and must be > 0"}
		}
		next := now.Add(time.Duration(*t.IntervalSeconds) * time.Second)
		return &next, nil
	case "once":
		if t.RunAt == nil {
			// Default to "now" so a one-shot fires on the next tick.
			next := now
			return &next, nil
		}
		return t.RunAt, nil
	default:
		return nil, &validationErr{"unknown schedule_type"}
	}
}

type validationErr struct{ msg string }

func (e *validationErr) Error() string { return e.msg }

// ownedTask resolves {taskID} to a scheduled task of the caller's org. Another
// org's task is a 404, the same as one that does not exist.
func (h *SchedulerHandler) ownedTask(w http.ResponseWriter, r *http.Request) (*model.ScheduledTask, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return nil, false
	}
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return nil, false
	}
	t, err := h.repo.GetTask(r.Context(), id)
	if err != nil || t == nil || t.OrgID != orgID {
		RespondError(w, http.StatusNotFound, "scheduled task not found")
		return nil, false
	}
	return t, true
}

// parseRunAt reads a one-time task's run_at, which must be RFC 3339 so the
// time zone is never guessed.
func parseRunAt(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	at, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil, &validationErr{"run_at must be an RFC 3339 time, e.g. 2026-01-02T09:00:00Z"}
	}
	return &at, nil
}

func (h *SchedulerHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	t, ok := h.ownedTask(w, r)
	if !ok {
		return
	}
	RespondJSON(w, http.StatusOK, t)
}

func (h *SchedulerHandler) Update(w http.ResponseWriter, r *http.Request) {
	cur, ok := h.ownedTask(w, r)
	if !ok {
		return
	}
	id := cur.ID

	var req model.UpdateScheduledTaskRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	for name, v := range map[string]*string{"agent_id": req.AgentID, "workflow_id": req.WorkflowID} {
		if v != nil && *v != "" {
			if _, perr := uuid.Parse(*v); perr != nil {
				RespondError(w, http.StatusBadRequest, "invalid "+name)
				return
			}
		}
	}
	runAt, err := parseRunAt(req.RunAt)
	if err != nil {
		RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Checked before the write, so a typo is refused rather than stored and
	// then discovered by a dispatcher that can no longer schedule the task.
	var rescheduled *time.Time
	if req.CronExpression != nil && *req.CronExpression != "" {
		sched, perr := h.cronParser.Parse(*req.CronExpression)
		if perr != nil {
			RespondError(w, http.StatusBadRequest, "invalid cron expression: "+perr.Error())
			return
		}
		next := sched.Next(time.Now())
		rescheduled = &next
	} else if req.IntervalSeconds != nil {
		if *req.IntervalSeconds <= 0 {
			RespondError(w, http.StatusBadRequest, "interval_seconds must be positive")
			return
		}
		next := time.Now().Add(time.Duration(*req.IntervalSeconds) * time.Second)
		rescheduled = &next
	} else if runAt != nil && cur.ScheduleType == "once" {
		rescheduled = runAt
	}

	t, err := h.repo.UpdateTask(r.Context(), id, req)
	if errors.Is(err, repository.ErrScheduleTargetNotFound) {
		RespondError(w, http.StatusBadRequest, "that agent or workflow is not in this organization")
		return
	}
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to update: "+err.Error())
		return
	}

	// Changing when a task runs has to move when it next runs, or the edit
	// looks ignored: next_run_at was computed from the old schedule and is only
	// recomputed after a fire, so the task would run once more at the very time
	// the user just changed away from.
	if rescheduled != nil {
		if serr := h.repo.SetNextRunAt(r.Context(), t.ID, *rescheduled); serr != nil {
			RespondError(w, http.StatusInternalServerError,
				"schedule saved but the next run time could not be updated: "+serr.Error())
			return
		}
		t.NextRunAt = rescheduled
	}

	RespondJSON(w, http.StatusOK, t)
}

func (h *SchedulerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	t, ok := h.ownedTask(w, r)
	if !ok {
		return
	}

	if err := h.repo.DeleteTask(r.Context(), t.ID); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SchedulerHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	t, ok := h.ownedTask(w, r)
	if !ok {
		return
	}
	id := t.ID

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	params := model.PaginationParams{Page: page, PerPage: perPage}

	result, err := h.repo.ListRuns(r.Context(), id, params)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	RespondJSON(w, http.StatusOK, result)
}
