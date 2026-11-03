package metalead

// The eval is how the model is chosen: people label DMs, comments and mentions
// in the tab, and each candidate model qualifies every labelled one several
// times with the production prompt. The report scores accuracy against the
// labels, consistency across runs, and whether "is a lead" answers quote the
// message truthfully. Switching to the winner stays configuration
// (AGENT_MODEL__META_LEAD_ANALYST or META_LEAD_MODEL).

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
)

const (
	maxEvalCases  = 300
	maxEvalModels = 4
	maxEvalRuns   = 5
	evalWorkers   = 4 // concurrent cases per model
	evalTimeout   = 90 * time.Minute
	evalCallLimit = 2 * time.Minute
)

// ModelResolver builds a completer for provider and model, with no fallback
// to another provider: a comparison must measure the model it names.
type ModelResolver func(provider, model string) (Completer, error)

// BindModels enables model comparisons.
func (s *Service) BindModels(r ModelResolver) { s.models = r }

// SetLabel records a person's verdict on a lead (nil clears it).
func (s *Service) SetLabel(ctx context.Context, orgID, leadID uuid.UUID, isLead *bool) (*model.MetaLead, error) {
	if err := s.store.SetLeadLabel(ctx, orgID, leadID, isLead); err != nil {
		return nil, err
	}
	return s.store.GetLead(ctx, orgID, leadID)
}

// ListEvals returns recent comparisons and how many labelled cases exist.
func (s *Service) ListEvals(ctx context.Context, orgID uuid.UUID) ([]model.MetaLeadEval, int, error) {
	evals, err := s.store.ListEvals(ctx, orgID, 20)
	if err != nil {
		return nil, 0, err
	}
	// A comparison dies with the pod that ran it.
	// ponytail: shown as interrupted, not resumed; start it again.
	for i := range evals {
		if evals[i].Status == model.MetaLeadRunStatusRunning && time.Since(evals[i].UpdatedAt) > evalTimeout+5*time.Minute {
			evals[i].Status, evals[i].ErrorMessage = model.MetaLeadRunStatusFailed, "interrupted (the server restarted); start it again"
		}
	}
	cases, err := s.store.ListLabelledLeads(ctx, orgID, maxEvalCases)
	return evals, len(cases), err
}

// StartEval runs every labelled case through each model, runs times, in the
// background.
func (s *Service) StartEval(ctx context.Context, orgID, userID uuid.UUID, models []string, runs int) (*model.MetaLeadEval, error) {
	if s.models == nil {
		return nil, fmt.Errorf("model comparison is not configured on this server")
	}
	var specs []string
	for _, m := range models {
		if m = strings.TrimSpace(m); m != "" && !slices.Contains(specs, m) {
			specs = append(specs, m)
		}
	}
	if len(specs) == 0 || len(specs) > maxEvalModels {
		return nil, fmt.Errorf("choose 1 to %d models", maxEvalModels)
	}
	for _, m := range specs {
		if p, n, ok := strings.Cut(m, ":"); !ok || p == "" || n == "" {
			return nil, fmt.Errorf("model %q must be provider:model", m)
		}
	}
	if runs <= 0 {
		runs = 3
	}
	runs = min(runs, maxEvalRuns)
	cases, err := s.store.ListLabelledLeads(ctx, orgID, maxEvalCases)
	if err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("label some DMs, comments or mentions as lead or not a lead first")
	}
	e := &model.MetaLeadEval{
		OrgID: orgID, Status: model.MetaLeadRunStatusRunning, Models: specs, Runs: runs,
		CaseCount: len(cases), CreatedBy: &userID,
	}
	if err := s.store.CreateEval(ctx, e); err != nil {
		return nil, err
	}
	go s.runEval(*e, cases)
	return e, nil
}

func (s *Service) runEval(e model.MetaLeadEval, cases []model.MetaLead) {
	ctx, cancel := context.WithTimeout(context.Background(), evalTimeout)
	defer cancel()
	rows := make([]model.MetaLeadEvalRow, len(e.Models))
	var wg sync.WaitGroup
	for i, spec := range e.Models {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows[i] = s.evalModel(ctx, spec, cases, e.Runs)
		}()
	}
	wg.Wait()
	e.Report, e.Status = rows, model.MetaLeadRunStatusDone
	if ctx.Err() != nil {
		e.Status, e.ErrorMessage = model.MetaLeadRunStatusFailed, "timed out after "+evalTimeout.String()+"; the scores cover what finished"
	}
	if err := s.store.UpdateEval(context.Background(), &e); err != nil {
		s.log.Error("meta eval update", zap.Error(err))
	}
}

// evalVote is one model answer to one case.
type evalVote struct {
	IsLead   *bool // nil: the call failed or gave no verdict
	Grounded bool  // a lead verdict whose evidence quote is in the message
	MS       int64
}

func (s *Service) evalModel(ctx context.Context, spec string, cases []model.MetaLead, runs int) model.MetaLeadEvalRow {
	provider, name, _ := strings.Cut(spec, ":")
	llm, err := s.models(provider, name)
	if err != nil {
		return model.MetaLeadEvalRow{Model: spec, Error: err.Error()}
	}
	a := &Analyser{LLM: llm, Model: name}
	votes := make([][]evalVote, len(cases))
	next := make(chan int)
	var wg sync.WaitGroup
	for range evalWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				for range runs {
					votes[i] = append(votes[i], vote(ctx, a, cases[i]))
				}
			}
		}()
	}
	for i := range cases {
		next <- i
	}
	close(next)
	wg.Wait()
	return scoreEval(spec, cases, votes)
}

func vote(ctx context.Context, a *Analyser, c model.MetaLead) evalVote {
	ctx, cancel := context.WithTimeout(ctx, evalCallLimit)
	defer cancel()
	start := time.Now()
	b, err := a.AnalyseInteraction(ctx, c.Source, c.AuthorName, c.Body)
	v := evalVote{MS: time.Since(start).Milliseconds()}
	if err != nil || b.IsLead == nil {
		return v
	}
	groundEvidence(b, c.Body)
	v.IsLead, v.Grounded = b.IsLead, *b.IsLead && b.Evidence != ""
	return v
}

// scoreEval turns votes[case][run] into a model's row. Each case's verdict is
// its majority; a tie is wrong whichever the label, and for precision and
// recall counts as "not a lead".
func scoreEval(spec string, cases []model.MetaLead, votes [][]evalVote) model.MetaLeadEvalRow {
	row := model.MetaLeadEvalRow{Model: spec}
	var correct, consistent, tp, fp, fn, leadVotes, grounded, calls int
	var ms int64
	for i, c := range cases {
		label := c.LabelIsLead != nil && *c.LabelIsLead
		yes, no := 0, 0
		for _, v := range votes[i] {
			calls++
			ms += v.MS
			switch {
			case v.IsLead == nil:
				row.Failures++
			case *v.IsLead:
				yes++
				leadVotes++
				if v.Grounded {
					grounded++
				}
			default:
				no++
			}
		}
		if n := len(votes[i]); n > 0 && (yes == n || no == n) {
			consistent++
		}
		predicted := yes > no
		switch {
		case yes != no && predicted == label:
			correct++
			if label {
				tp++
			}
		default:
			if predicted {
				fp++
			} else if label {
				fn++
			}
			if len(row.Misses) < 10 {
				row.Misses = append(row.Misses, model.MetaLeadEvalMiss{
					LeadID: c.ID, Body: truncate(c.Body, 200), Label: label,
					Votes: fmt.Sprintf("lead %d/%d", yes, len(votes[i])),
				})
			}
		}
	}
	if n := len(cases); n > 0 {
		row.Accuracy = float64(correct) / float64(n)
		row.Consistency = float64(consistent) / float64(n)
	}
	row.Precision = ratio(tp, tp+fp)
	row.Recall = ratio(tp, tp+fn)
	row.Grounded = ratio(grounded, leadVotes)
	if calls > 0 {
		row.AvgLatencyMS = ms / int64(calls)
	}
	return row
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}
