package handler

import (
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

	if req.AgentID != nil {
		id, _ := uuid.Parse(*req.AgentID)
		t.AgentID = &id
	}
	if req.WorkflowID != nil {
		id, _ := uuid.Parse(*req.WorkflowID)
		t.WorkflowID = &id
	}
	if req.ProviderConfigID != nil {
		id, _ := uuid.Parse(*req.ProviderConfigID)
		t.ProviderConfigID = &id
	}
	if req.PlannerID != nil {
		id, _ := uuid.Parse(*req.PlannerID)
		t.PlannerID = &id
	}
	if req.ExecutorID != nil {
		id, _ := uuid.Parse(*req.ExecutorID)
		t.ExecutorID = &id
	}
	if req.ReviewerID != nil {
		id, _ := uuid.Parse(*req.ReviewerID)
		t.ReviewerID = &id
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

func (h *SchedulerHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	t, err := h.repo.GetTask(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, "scheduled task not found")
		return
	}
	RespondJSON(w, http.StatusOK, t)
}

func (h *SchedulerHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	var req model.UpdateScheduledTaskRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	t, err := h.repo.UpdateTask(r.Context(), id, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to update: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, t)
}

func (h *SchedulerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	if err := h.repo.DeleteTask(r.Context(), id); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SchedulerHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

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
