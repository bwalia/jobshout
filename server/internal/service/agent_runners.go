package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

// The runners below wrap the pipelines that already existed. None of them adds
// execution logic: this is re-plumbing, so that one contract reaches work that
// used to be reachable only by knowing which endpoint to post to.
//
// Each returns the specialist row's id, which the run records as
// ExternalRunID — that is the link from "the agent was asked to do this" to
// "here is the thing doing it".

// --- Article Writer ------------------------------------------------------

type articleRunner struct{ blog BlogService }

// NewArticleRunner runs the Article Writer through the blog pipeline.
func NewArticleRunner(blog BlogService) AgentRunner { return &articleRunner{blog: blog} }

func (r *articleRunner) Builtin() string { return model.BuiltinArticleWriter }
func (r *articleRunner) Kind() string    { return "blog_run" }

func (r *articleRunner) Start(ctx context.Context, run *model.AgentRun, _ *model.Agent, in map[string]string) (string, error) {
	if r.blog == nil {
		return "", fmt.Errorf("the article writer is not configured on this server")
	}
	out, err := r.blog.Generate(ctx, run.OrgID, run.RequestedBy, run.Source, model.GenerateBlogRequest{
		Briefs: []model.BlogBrief{{Topic: in["topic"], Context: in["context"]}},
		Model:  in["model"],
	})
	if err != nil {
		return "", err
	}
	return out.ID.String(), nil
}

// --- Researcher ----------------------------------------------------------

type researchRunner struct{ research ResearchService }

// NewResearchRunner runs the Research Agent, which persists its own run.
func NewResearchRunner(rs ResearchService) AgentRunner { return &researchRunner{research: rs} }

func (r *researchRunner) Builtin() string { return model.BuiltinResearcher }
func (r *researchRunner) Kind() string    { return "research_run" }

func (r *researchRunner) Start(ctx context.Context, run *model.AgentRun, _ *model.Agent, in map[string]string) (string, error) {
	if r.research == nil || !r.research.Available() {
		return "", fmt.Errorf("research is not configured on this server")
	}
	// StartAsync rather than Run: research already has its own worker and run
	// row, so this hands off rather than waiting inside another worker.
	out, err := r.research.StartAsync(ctx, run.OrgID, research.Request{
		Topic:   in["topic"],
		Context: in["context"],
	}, ResearchRunOptions{
		Source:      researchSourceFor(run.Source),
		TaskID:      run.TaskID,
		RequestedBy: run.RequestedBy,
	})
	if err != nil {
		return "", err
	}
	return out.ID.String(), nil
}

// researchSourceFor maps an agent-run source onto the research vocabulary, so
// a research run started through the Task Manager says so.
func researchSourceFor(source string) string {
	switch source {
	case model.AgentRunSourceChat:
		return model.ResearchSourceChat
	case model.AgentRunSourceTaskManager:
		return model.ResearchSourceTaskManager
	default:
		return model.ResearchSourceAPI
	}
}

// --- Pentester -----------------------------------------------------------

type pentestRunner struct{ pentest PentestService }

// NewPentestRunner runs the Penetration Testing Agent.
func NewPentestRunner(p PentestService) AgentRunner { return &pentestRunner{pentest: p} }

func (r *pentestRunner) Builtin() string { return model.BuiltinPentester }
func (r *pentestRunner) Kind() string    { return "pentest_run" }

func (r *pentestRunner) Start(ctx context.Context, run *model.AgentRun, agent *model.Agent, in map[string]string) (string, error) {
	if r.pentest == nil {
		return "", fmt.Errorf("penetration testing is not configured on this server")
	}
	req := model.CreatePentestRunRequest{
		AgentID:     agent.ID,
		TaskID:      run.TaskID,
		Target:      in["target"],
		ScanMode:    in["scan_mode"],
		Instruction: in["instruction"],
	}
	if b := strings.TrimSpace(in["max_budget"]); b != "" {
		n, err := strconv.Atoi(b)
		if err != nil {
			return "", fmt.Errorf("max budget must be a whole number of cents, got %q", b)
		}
		req.MaxBudget = &n
	}
	out, err := r.pentest.CreateRun(ctx, req, run.OrgID, run.RequestedBy)
	if err != nil {
		return "", err
	}
	return out.ID.String(), nil
}

// --- PR Reviewer ---------------------------------------------------------

type reviewRunner struct{ reviews ReviewService }

// NewReviewRunner runs the PR Reviewer.
func NewReviewRunner(rv ReviewService) AgentRunner { return &reviewRunner{reviews: rv} }

func (r *reviewRunner) Builtin() string { return model.BuiltinPRReviewer }
func (r *reviewRunner) Kind() string    { return "review_run" }

func (r *reviewRunner) Start(ctx context.Context, run *model.AgentRun, agent *model.Agent, in map[string]string) (string, error) {
	if r.reviews == nil {
		return "", fmt.Errorf("pull request review is not configured on this server")
	}
	pr, err := strconv.Atoi(strings.TrimSpace(in["pr_number"]))
	if err != nil || pr < 1 {
		return "", fmt.Errorf("pull request number must be a positive whole number, got %q", in["pr_number"])
	}
	// Whether this posts in public is decided by the schema default, which is
	// the one field the Go and TypeScript contracts still disagree on. See
	// agentschema.ForBuiltin and TestKnownDefaultDivergence_PRReviewerDryRun.
	dry := strings.EqualFold(strings.TrimSpace(in["dry_run"]), "true")
	agentID := agent.ID
	out, err := r.reviews.CreateRun(ctx, model.CreateReviewRunRequest{
		Repo:     in["repo"],
		PRNumber: pr,
		DryRun:   &dry,
		AgentID:  &agentID,
	}, run.OrgID, run.RequestedBy)
	if err != nil {
		return "", err
	}
	return out.ID.String(), nil
}

// --- Mail ----------------------------------------------------------------

type mailRunner struct{ mail MailService }

// NewMailRunner saves the Mail Agent playbook and queues a sync.
func NewMailRunner(m MailService) AgentRunner { return &mailRunner{mail: m} }

func (r *mailRunner) Builtin() string { return model.BuiltinMail }
func (r *mailRunner) Kind() string    { return "mail_sync" }

// Precheck refuses only the case that is both knowable now and not worth
// recording: nothing to save and nowhere to sync. When there is a playbook to
// write, the run proceeds so the operator's configuration is kept even though
// the sync will not succeed.
func (r *mailRunner) Precheck(ctx context.Context, orgID uuid.UUID, in map[string]string) error {
	if r.mail == nil {
		return fmt.Errorf("the mail agent is not configured on this server")
	}
	if _, hasPlaybook := MailPlaybookPatch(in); hasPlaybook {
		return nil
	}
	if !r.mail.Available(ctx, orgID) {
		return fmt.Errorf("Gmail is not connected — connect the shared mailbox on Mail Agent first")
	}
	return nil
}

func (r *mailRunner) Start(ctx context.Context, run *model.AgentRun, _ *model.Agent, in map[string]string) (string, error) {
	if r.mail == nil {
		return "", fmt.Errorf("the mail agent is not configured on this server")
	}
	// Save the playbook before syncing, so the sync uses it. Only fields the
	// caller supplied are written: an omitted field must never wipe a playbook
	// the operator saved, which is the rule the web form enforces too.
	if patch, ok := MailPlaybookPatch(in); ok {
		if _, err := r.mail.UpdateConnection(ctx, run.OrgID, patch); err != nil {
			return "", err
		}
	}
	if !r.mail.Available(ctx, run.OrgID) {
		// The playbook is saved; the mailbox simply is not connected. That is
		// worth saying plainly rather than failing as though nothing happened.
		return "", fmt.Errorf("playbook saved, but Gmail is not connected — connect the shared mailbox on Mail Agent")
	}
	if err := r.mail.EnqueueSync(ctx, run.OrgID); err != nil {
		return "", err
	}
	return "", nil
}

// MailPlaybookPatch builds a connection patch from the playbook fields present,
// reporting false when none were supplied.
//
// Watch rules travel together: MailWatchRules is one value on the connection,
// so supplying any of senders/prefixes/labels rewrites all three.
func MailPlaybookPatch(in map[string]string) (model.UpdateMailConnectionRequest, bool) {
	var patch model.UpdateMailConnectionRequest
	changed := false

	senders, hasSenders := in["senders"]
	prefixes, hasPrefixes := in["subject_prefixes"]
	labels, hasLabels := in["labels"]
	if hasSenders || hasPrefixes || hasLabels {
		patch.Rules = &model.MailWatchRules{
			Senders:         splitPlaybookList(senders),
			SubjectPrefixes: splitPlaybookList(prefixes),
			Labels:          splitPlaybookList(labels),
		}
		changed = true
	}
	if v, ok := in["knowledge_urls"]; ok {
		urls := splitPlaybookList(v)
		patch.KnowledgeURLs = &urls
		changed = true
	}
	if v, ok := in["research_focus"]; ok {
		s := strings.TrimSpace(v)
		patch.ResearchFocus = &s
		changed = true
	}
	if v, ok := in["reply_instructions"]; ok {
		s := strings.TrimSpace(v)
		patch.ReplyInstructions = &s
		changed = true
	}
	return patch, changed
}

// splitPlaybookList accepts a comma- or newline-separated list, because the web
// form uses commas for watch rules and newlines for knowledge URLs, and a chat
// user will not know the difference.
func splitPlaybookList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' })
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if s := strings.TrimSpace(f); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// --- Generic fallback ----------------------------------------------------

type genericRunner struct {
	runs TaskRunService
	exec ExecutionService
}

// NewGenericRunner runs a user-created agent — one with no builtin marker —
// through the prompt path that already existed.
//
// Which path depends on whether the run belongs to a board task. The Task
// Manager always creates the task first, so its runs land in task_runs and show
// on the board; chat has no task, so those go through ExecutionService exactly
// as they did before. Inventing a task for the chat case would need a project
// to put it in, and picking one on the user's behalf is not this runner's
// decision to make.
func NewGenericRunner(runs TaskRunService, exec ExecutionService) AgentRunner {
	return &genericRunner{runs: runs, exec: exec}
}

func (r *genericRunner) Builtin() string { return "" }
func (r *genericRunner) Kind() string    { return "task_run" }

func (r *genericRunner) Start(ctx context.Context, run *model.AgentRun, agent *model.Agent, in map[string]string) (string, error) {
	if run.TaskID != nil {
		if r.runs == nil {
			return "", fmt.Errorf("task execution is not configured on this server")
		}
		out, err := r.runs.CreateRun(ctx, *run.TaskID, model.CreateTaskRunRequest{
			AgentID: &agent.ID,
		}, run.OrgID, run.RequestedBy)
		if err != nil {
			return "", err
		}
		return out.ID.String(), nil
	}

	if r.exec == nil {
		return "", fmt.Errorf("agent execution is not configured on this server")
	}
	prompt := strings.TrimSpace(in["prompt"])
	if prompt == "" {
		return "", fmt.Errorf("%s needs a prompt to run", agent.Name)
	}
	out, err := r.exec.Start(ctx, run.OrgID, agent.ID, model.ExecuteAgentRequest{Prompt: prompt})
	if err != nil {
		return "", err
	}
	return out.ID.String(), nil
}
