package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// ErrAgentRunMissingInput is returned when a required interview slot is empty.
// It carries the slot and the question, so a caller can ask in its own idiom —
// a form field in the Task Manager, a clarifying question in chat.
type ErrAgentRunMissingInput struct {
	Missing  []string
	Question string
	Options  []model.ClarifyOption
}

func (e *ErrAgentRunMissingInput) Error() string {
	return fmt.Sprintf("agent_run: missing input %v: %s", e.Missing, e.Question)
}

// AgentRunner executes one builtin.
//
// Runners are registered by builtin marker rather than selected by a switch, so
// the schema and the executor cannot drift apart: Schema() is a method on the
// thing that runs. That is the failure this whole contract exists to prevent —
// the Mail Agent once had six input fields in TypeScript and none in Go, and
// nothing could notice.
type AgentRunner interface {
	// Builtin is the model.Builtin* marker this runner handles. Empty marks
	// the generic fallback.
	Builtin() string
	// Kind names the external row this runner produces, for AgentRun.ExternalKind.
	Kind() string
	// Start launches the work and returns the specialist run's id, if it has
	// one. It is called on a worker, never in the request.
	Start(ctx context.Context, run *model.AgentRun, agent *model.Agent, inputs map[string]string) (externalID string, err error)
}

// AgentRunPrechecker is an OPTIONAL capability a runner may implement to refuse
// a run synchronously, before it is recorded.
//
// It is kept separate from AgentRunner — the same shape as llm.ToolCapableClient
// — so existing runners satisfy the interface unchanged and the service
// type-asserts to discover the capability.
//
// It answers "can this agent run at all right now", not "are these inputs
// complete", which the schema already decides. Its purpose is to keep an
// immediately knowable problem immediate: telling someone their mailbox is not
// connected the moment they press Run is worth more than recording a run that
// fails a second later. Implementations must be fast and side-effect free.
type AgentRunPrechecker interface {
	Precheck(ctx context.Context, orgID uuid.UUID, inputs map[string]string) error
}

// AgentRunService is the one front door for "run agent X with inputs Y".
type AgentRunService interface {
	// Start validates inputs against the agent's schema, records a run and
	// dispatches it. Returns ErrAgentRunMissingInput when a required slot is
	// empty, before anything is written.
	Start(ctx context.Context, orgID uuid.UUID, req model.CreateAgentRunRequest, requestedBy *uuid.UUID, source string) (*model.AgentRun, *model.Agent, error)
	GetRun(ctx context.Context, orgID, runID uuid.UUID) (*model.AgentRun, error)
	ListRuns(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.AgentRun], error)
}

type agentRunService struct {
	runs      repository.AgentRunRepository
	agentRepo repository.AgentRepository
	runners   map[string]AgentRunner
	generic   AgentRunner
	logger    *zap.Logger
}

// NewAgentRunService wires the front door. Runners are indexed by their own
// Builtin(); the one reporting "" becomes the generic fallback.
func NewAgentRunService(
	runs repository.AgentRunRepository,
	agentRepo repository.AgentRepository,
	logger *zap.Logger,
	runners ...AgentRunner,
) AgentRunService {
	if logger == nil {
		logger = zap.NewNop()
	}
	s := &agentRunService{
		runs:      runs,
		agentRepo: agentRepo,
		runners:   map[string]AgentRunner{},
		logger:    logger,
	}
	for _, r := range runners {
		if r == nil {
			continue
		}
		if r.Builtin() == "" {
			s.generic = r
			continue
		}
		s.runners[r.Builtin()] = r
	}
	return s
}

// agentRunTimeout bounds the hand-off, not the work. Runners return once the
// specialist row exists; the pipeline behind it has its own budget.
const agentRunTimeout = 60 * time.Second

func (s *agentRunService) Start(
	ctx context.Context,
	orgID uuid.UUID,
	req model.CreateAgentRunRequest,
	requestedBy *uuid.UUID,
	source string,
) (*model.AgentRun, *model.Agent, error) {
	agent, err := s.agentRepo.FindByID(ctx, req.AgentID)
	if err != nil || agent == nil {
		return nil, nil, fmt.Errorf("agent_run: agent not found")
	}
	if agent.OrgID != orgID {
		// Reported as absent rather than forbidden, so the API does not confirm
		// that an id in another organisation exists.
		return nil, nil, fmt.Errorf("agent_run: agent not found")
	}

	builtin := agentschema.BuiltinOf(agent)
	schema := agentschema.ForBuiltin(builtin)

	inputs := map[string]string{}
	for k, v := range req.Inputs {
		inputs[k] = v
	}
	// Validation happens here, once, for every surface. It reuses the same
	// NextMissing/ApplyDefaults the chat interview walks, so the two cannot
	// disagree about what "complete" means.
	if slot, question, opts := schema.NextMissing(inputs); slot != "" {
		return nil, nil, &ErrAgentRunMissingInput{
			Missing: []string{slot}, Question: question, Options: opts,
		}
	}
	inputs = schema.ApplyDefaults(inputs)

	runner := s.runnerFor(builtin)
	if runner == nil {
		return nil, nil, fmt.Errorf("agent_run: %s cannot be run on this server", agent.Name)
	}

	if pc, ok := runner.(AgentRunPrechecker); ok {
		if err := pc.Precheck(ctx, orgID, inputs); err != nil {
			return nil, nil, err
		}
	}

	blob, err := json.Marshal(inputs)
	if err != nil {
		return nil, nil, fmt.Errorf("agent_run: encode inputs: %w", err)
	}
	run := &model.AgentRun{
		OrgID:        orgID,
		AgentID:      agent.ID,
		TaskID:       req.TaskID,
		RequestedBy:  requestedBy,
		Builtin:      builtin,
		Source:       source,
		Inputs:       blob,
		Status:       model.AgentRunQueued,
		ExternalKind: runner.Kind(),
	}
	if err := s.runs.Create(ctx, run); err != nil {
		return nil, nil, err
	}

	// Dispatched inline, on purpose. Every runner is a hand-off: it writes a
	// queued row for a pipeline that already runs asynchronously — blog.Generate
	// spawns its own goroutine, pentest and review write rows a reconciler picks
	// up, research has its own worker — so none of them does the long work here.
	//
	// Doing it inline is what lets the response carry ExternalRunID, and the
	// Task Manager deep-links straight to the article, scan or review it just
	// started. On a worker that id would not exist yet, and the caller would
	// have to poll to find out where it had been sent.
	s.dispatch(ctx, run, agent, inputs, runner)

	return run, agent, nil
}

func (s *agentRunService) dispatch(
	ctx context.Context,
	run *model.AgentRun,
	agent *model.Agent,
	inputs map[string]string,
	runner AgentRunner,
) {
	bg, cancel := context.WithTimeout(ctx, agentRunTimeout)
	defer cancel()

	now := time.Now().UTC()
	run.Status = model.AgentRunRunning
	run.StartedAt = &now
	if err := s.runs.Update(bg, run); err != nil {
		s.logger.Warn("agent_run: could not mark the run running", zap.Error(err))
	}

	externalID, err := runner.Start(bg, run, agent, inputs)
	done := time.Now().UTC()
	run.CompletedAt = &done
	if externalID != "" {
		run.ExternalRunID = &externalID
	}
	if err != nil {
		msg := err.Error()
		run.Status = model.AgentRunFailed
		run.ErrorMessage = &msg
		s.logger.Warn("agent_run: dispatch failed",
			zap.String("run_id", run.ID.String()),
			zap.String("agent", agent.Name),
			zap.Error(err))
	} else {
		// Completed means "handed off successfully". The specialist row named
		// by ExternalRunID carries the work's own progress from here; claiming
		// the article is written when generation has only started would be a
		// lie the board would repeat.
		run.Status = model.AgentRunCompleted
	}
	if err := s.runs.Update(bg, run); err != nil {
		s.logger.Warn("agent_run: could not close out the run", zap.Error(err))
	}
}

func (s *agentRunService) runnerFor(builtin string) AgentRunner {
	if r, ok := s.runners[builtin]; ok {
		return r
	}
	return s.generic
}

func (s *agentRunService) GetRun(ctx context.Context, orgID, runID uuid.UUID) (*model.AgentRun, error) {
	run, err := s.runs.GetByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run == nil || run.OrgID != orgID {
		return nil, nil
	}
	return run, nil
}

func (s *agentRunService) ListRuns(ctx context.Context, orgID uuid.UUID, p model.PaginationParams) (*model.PaginatedResponse[model.AgentRun], error) {
	return s.runs.ListByOrg(ctx, orgID, p)
}

// AsMissingInput reports whether err is a missing-input error, for callers that
// need to turn it into a 400 or a clarifying question.
func AsMissingInput(err error) (*ErrAgentRunMissingInput, bool) {
	var e *ErrAgentRunMissingInput
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}
