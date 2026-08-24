package chatagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/llmtrace"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/platformtools"
)

const MaxIterations = 15

// Recaller is the long-term memory surface chat uses. MemoryService satisfies it.
type Recaller interface {
	Recall(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]string, error)
}

// Agent is the tool-calling loop that drives a chat turn.
type Agent struct {
	client   llm.Client
	reg      *platformtools.Registry
	guard    *platformtools.Guard
	memory   Recaller
	logger   *zap.Logger
	maxIter  int
	extraSys string
}

func New(client llm.Client, reg *platformtools.Registry, guard *platformtools.Guard, memory Recaller, logger *zap.Logger) *Agent {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Agent{
		client:  client,
		reg:     reg,
		guard:   guard,
		memory:  memory,
		logger:  logger,
		maxIter: MaxIterations,
	}
}

// TurnRequest is everything needed to run one user message.
type TurnRequest struct {
	Ident             platformtools.Identity
	Message           string
	ConfirmationToken string
	History           []model.ChatMessage
	Metadata          map[string]any
	Stream            StreamFunc
}

// TurnResult is the envelope plus the session metadata to persist.
type TurnResult struct {
	Response model.ChatResponse
	Metadata map[string]any
}

func (a *Agent) Run(ctx context.Context, req TurnRequest) (*TurnResult, error) {
	start := time.Now()
	meta := req.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" && req.ConfirmationToken == "" {
		resp := model.ChatResponse{Message: "Please send a non-empty message."}
		emit(req.Stream, Event{Type: EventDone, Response: &resp})
		return &TurnResult{Response: resp, Metadata: meta}, nil
	}

	ident := req.Ident
	ctx = platformtools.WithIdentity(ctx, ident)
	ctx = llmtrace.WithTrace(ctx, llmtrace.TraceInfo{
		TraceName: "go-chat-run",
		SessionID: ident.SessionID.String(),
		OrgID:     ident.OrgID.String(),
	})

	var perms map[string]bool
	if a.guard != nil {
		p, err := a.guard.PermissionsFor(ctx, ident)
		if err != nil {
			a.logger.Warn("chatagent: permissions", zap.Error(err))
		}
		perms = p
	}
	ctx = platformtools.WithPermissions(ctx, perms)

	if pending := readConfirm(meta); pending != nil {
		return a.handleConfirmation(ctx, req, meta, pending, start)
	}

	if pending := readPending(meta); pending != nil {
		if isAbandon(msg) {
			writePending(meta, nil)
		} else {
			return a.handlePending(ctx, req, meta, pending, start)
		}
	}

	return a.loop(ctx, req, meta, msg, start)
}

func (a *Agent) handleConfirmation(ctx context.Context, req TurnRequest, meta map[string]any, pending *model.PendingConfirmation, start time.Time) (*TurnResult, error) {
	tokenOK := req.ConfirmationToken != "" && req.ConfirmationToken == pending.Token
	yes := tokenOK || isAffirmative(req.Message)
	no := isNegative(req.Message) || strings.EqualFold(strings.TrimSpace(req.Message), "cancel")

	if !yes && !no && req.ConfirmationToken == "" {
		resp := model.ChatResponse{
			Message: "This is still waiting for your approval. Approve to continue, or cancel.",
			Confirmation: &model.ConfirmRequest{
				Token:     pending.Token,
				Tool:      pending.Tool,
				Summary:   pending.Summary,
				Effect:    pending.Effect,
				ExpiresAt: pending.ExpiresAt.Format(time.RFC3339),
			},
		}
		emit(req.Stream, Event{Type: EventConfirmation, Confirmation: resp.Confirmation})
		emit(req.Stream, Event{Type: EventDone, Response: &resp})
		return &TurnResult{Response: resp, Metadata: meta}, nil
	}

	writeConfirm(meta, nil)
	if no || (!yes && req.ConfirmationToken != "" && req.ConfirmationToken != pending.Token) {
		resp := model.ChatResponse{Message: "Cancelled. Nothing was changed."}
		emit(req.Stream, Event{Type: EventDone, Response: &resp})
		return &TurnResult{Response: resp, Metadata: meta}, nil
	}

	action, entities, clarify, result, err := a.executeTool(ctx, req.Stream, pending.Tool, pending.Args, true)
	if err != nil {
		resp := failResponse(HumaniseError(err), start)
		emit(req.Stream, Event{Type: EventError, Error: resp.Message})
		return &TurnResult{Response: resp, Metadata: meta}, nil
	}
	if clarify != nil {
		resp := model.ChatResponse{Message: clarify.Question, Clarify: clarify, Actions: []model.ActionRecord{action}, Entities: entities}
		finaliseUsage(&resp, start, "")
		emit(req.Stream, Event{Type: EventClarify, Clarify: clarify})
		emit(req.Stream, Event{Type: EventDone, Response: &resp})
		return &TurnResult{Response: resp, Metadata: meta}, nil
	}
	ents := readEntities(meta)
	upsertEntities(ents, entities)
	writeEntities(meta, ents)

	msg := composeFromResult(action, entities, result)
	if action.Status != model.ActionOK {
		msg = action.Error
		if msg == "" {
			msg = "That didn't go through."
		}
	}
	resp := model.ChatResponse{
		Message:  SanitiseMessage(msg),
		Actions:  []model.ActionRecord{action},
		Entities: entities,
	}
	finaliseUsage(&resp, start, "")
	emit(req.Stream, Event{Type: EventDone, Response: &resp})
	return &TurnResult{Response: resp, Metadata: meta}, nil
}

func (a *Agent) handlePending(ctx context.Context, req TurnRequest, meta map[string]any, pending *model.PendingAction, start time.Time) (*TurnResult, error) {
	args := mergePendingArgs(pending, req.Message)
	action, entities, clarify, result, err := a.executeTool(ctx, req.Stream, pending.Tool, args, false)
	if err != nil && !errors.Is(err, platformtools.ErrNeedsConfirm) {
		writePending(meta, nil)
		resp := failResponse(HumaniseError(err), start)
		emit(req.Stream, Event{Type: EventError, Error: resp.Message})
		return &TurnResult{Response: resp, Metadata: meta}, nil
	}
	if action.Status == model.ActionPendingConfirmation {
		writePending(meta, nil)
		return a.holdConfirmation(req, meta, pending.Tool, args, action, start)
	}
	if clarify != nil {
		writePending(meta, &model.PendingAction{
			Tool: pending.Tool, Args: args, Missing: slotNames(clarify),
			AskedAt: time.Now(), Question: clarify.Question,
		})
		resp := model.ChatResponse{Message: SanitiseMessage(clarify.Question), Clarify: clarify, Actions: []model.ActionRecord{action}}
		finaliseUsage(&resp, start, "")
		emit(req.Stream, Event{Type: EventClarify, Clarify: clarify})
		emit(req.Stream, Event{Type: EventDone, Response: &resp})
		return &TurnResult{Response: resp, Metadata: meta}, nil
	}
	writePending(meta, nil)
	ents := readEntities(meta)
	upsertEntities(ents, entities)
	writeEntities(meta, ents)
	msg := composeFromResult(action, entities, result)
	if action.Status != model.ActionOK {
		msg = action.Error
		if msg == "" {
			msg = "That didn't go through."
		}
	}
	resp := model.ChatResponse{Message: SanitiseMessage(msg), Actions: []model.ActionRecord{action}, Entities: entities}
	finaliseUsage(&resp, start, "")
	emit(req.Stream, Event{Type: EventDone, Response: &resp})
	return &TurnResult{Response: resp, Metadata: meta}, nil
}

func (a *Agent) loop(ctx context.Context, req TurnRequest, meta map[string]any, userMsg string, start time.Time) (*TurnResult, error) {
	if a.client == nil {
		resp := failResponse("Chat is not configured with a language model.", start)
		emit(req.Stream, Event{Type: EventError, Error: resp.Message})
		return &TurnResult{Response: resp, Metadata: meta}, nil
	}

	kept, evicted := Window(req.History, windowTokenBudget)
	summary := readSummary(meta)
	if len(evicted) > 0 {
		summary = rollSummary(summary, evicted)
		meta[model.ChatMetaSummary] = summary
	}
	entities := readEntities(meta)
	pending := readPending(meta)

	var memories []string
	if a.memory != nil && userMsg != "" {
		hits, err := a.memory.Recall(ctx, req.Ident.UserID, userMsg, 5)
		if err != nil {
			a.logger.Warn("chatagent: recall", zap.Error(err))
		} else {
			memories = hits
		}
	}

	sys := systemPrompt(time.Now(), summary, entities, memories, pending, a.extraSys)
	messages := []llm.Message{{Role: llm.RoleSystem, Content: sys}}
	messages = append(messages, toLLMHistory(kept)...)
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: userMsg})

	disclosed := readDisclosed(meta)
	var (
		actions      []model.ActionRecord
		allEntities  []model.EntityRef
		inputTokens  int
		outputTokens int
		modelName    string
		anyFailed    bool
	)

	maxIter := a.maxIter
	if maxIter <= 0 {
		maxIter = MaxIterations
	}

	caller, mode := turnCallerFor(a.client, a.logger)
	if len(req.History) == 0 {
		a.logger.Info("chatagent: tool mode selected",
			zap.String("mode", mode),
			zap.String("provider", a.client.ProviderName()),
			zap.String("session", req.Ident.SessionID.String()))
	}

	for iteration := 1; iteration <= maxIter; iteration++ {
		toolsForTurn := a.reg.SelectForTurn(platformtools.PermissionsFrom(ctx), disclosed)
		defs := platformtools.ToolDefs(toolsForTurn)

		llmResp, err := caller.next(ctx, messages, defs)
		if err != nil {
			resp := failResponse(HumaniseError(err), start)
			emit(req.Stream, Event{Type: EventError, Error: resp.Message})
			return &TurnResult{Response: resp, Metadata: meta}, nil
		}
		inputTokens += llmResp.InputTokens
		outputTokens += llmResp.OutputTokens
		if llmResp.Model != "" {
			modelName = llmResp.Model
		}

		if len(llmResp.ToolCalls) == 0 {
			text := SanitiseMessage(llmResp.Content)
			if anyFailed && looksAffirmativeSuccess(text) {
				text = lastFailureMessage(actions)
			}
			if ContainsDeveloperFacing(text) {
				text = SanitiseMessage(text)
			}
			resp := model.ChatResponse{
				Message:  text,
				Actions:  actions,
				Entities: allEntities,
			}
			if text == "" && len(actions) > 0 {
				resp.Message = composeFromResult(actions[len(actions)-1], allEntities, nil)
			}
			if text == "" {
				resp.Message = "I'm not sure how to help with that. Try asking me to list agents, create a task, or run a workflow."
			}
			resp.Usage = &model.UsageInfo{Model: modelName, InputTokens: inputTokens, OutputTokens: outputTokens, LatencyMs: time.Since(start).Milliseconds()}
			if len(resp.Message) > 0 {
				emit(req.Stream, Event{Type: EventToken, Token: resp.Message})
			}
			emit(req.Stream, Event{Type: EventDone, Response: &resp})
			writeEntities(meta, entities)
			return &TurnResult{Response: resp, Metadata: meta}, nil
		}

		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   llmResp.Content,
			ToolCalls: llmResp.ToolCalls,
		})

		for _, tc := range llmResp.ToolCalls {
			args := tc.Arguments
			if args == nil {
				args = map[string]any{}
			}
			action, ents, clarify, result, _ := a.executeTool(ctx, req.Stream, tc.Name, args, false)
			actions = append(actions, action)
			allEntities = append(allEntities, ents...)
			upsertEntities(entities, ents)

			if action.Status == model.ActionFailed || action.Status == model.ActionDenied {
				anyFailed = true
			}

			if names := disclosedFromResult(tc.Name, result); len(names) > 0 {
				disclosed = appendUnique(disclosed, names)
				addDisclosed(meta, names)
			}

			if action.Status == model.ActionPendingConfirmation {
				tr, err := a.holdConfirmation(req, meta, tc.Name, args, action, start)
				if tr != nil {
					tr.Response.Actions = actions
					tr.Response.Entities = allEntities
				}
				writeEntities(meta, entities)
				return tr, err
			}

			if clarify != nil {
				writePending(meta, &model.PendingAction{
					Tool: tc.Name, Args: copyArgs(args), Missing: slotNames(clarify),
					AskedAt: time.Now(), Question: clarify.Question,
				})
				writeEntities(meta, entities)
				resp := model.ChatResponse{
					Message:  SanitiseMessage(clarify.Question),
					Actions:  actions,
					Entities: allEntities,
					Clarify:  clarify,
					Usage:    &model.UsageInfo{Model: modelName, InputTokens: inputTokens, OutputTokens: outputTokens, LatencyMs: time.Since(start).Milliseconds()},
				}
				emit(req.Stream, Event{Type: EventClarify, Clarify: clarify})
				emit(req.Stream, Event{Type: EventDone, Response: &resp})
				return &TurnResult{Response: resp, Metadata: meta}, nil
			}

			content := wrapToolResult(tc.Name, action, result, ents)
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: tc.ID,
				Content:    content,
			})
		}

		if anyFailed {
			messages = append(messages, llm.Message{
				Role:    llm.RoleSystem,
				Content: "A tool failed. You must tell the user it did not work. Never claim success.",
			})
		}
	}

	resp := model.ChatResponse{
		Message:  "I ran out of steps before I could finish. Some of the work above may have completed — ask me for status if you need a check.",
		Actions:  actions,
		Entities: allEntities,
		Usage:    &model.UsageInfo{Model: modelName, InputTokens: inputTokens, OutputTokens: outputTokens, LatencyMs: time.Since(start).Milliseconds()},
	}
	writeEntities(meta, entities)
	emit(req.Stream, Event{Type: EventDone, Response: &resp})
	return &TurnResult{Response: resp, Metadata: meta}, nil
}

func (a *Agent) holdConfirmation(req TurnRequest, meta map[string]any, tool string, args map[string]any, action model.ActionRecord, start time.Time) (*TurnResult, error) {
	token := newConfirmToken()
	effect := action.Error
	if effect == "" {
		effect = fmt.Sprintf("This will run %s. It cannot be undone from this conversation.", platformtools.HumanLabel(tool))
	}
	summary := fmt.Sprintf("Approve %s?", platformtools.HumanLabel(tool))
	pc := &model.PendingConfirmation{
		Token: token, Tool: tool, Args: copyArgs(args),
		Summary: summary, Effect: effect, ExpiresAt: time.Now().Add(confirmTTL),
	}
	writeConfirm(meta, pc)
	confirm := &model.ConfirmRequest{
		Token: token, Tool: tool, Summary: summary, Effect: effect,
		ExpiresAt: pc.ExpiresAt.Format(time.RFC3339),
	}
	action.Error = ""
	resp := model.ChatResponse{
		Message:      SanitiseMessage(effect),
		Actions:      []model.ActionRecord{action},
		Confirmation: confirm,
	}
	finaliseUsage(&resp, start, "")
	emit(req.Stream, Event{Type: EventConfirmation, Confirmation: confirm})
	emit(req.Stream, Event{Type: EventDone, Response: &resp})
	return &TurnResult{Response: resp, Metadata: meta}, nil
}

// executeTool runs guard + tool. The returned error is reserved for unexpected failures.
func (a *Agent) executeTool(ctx context.Context, stream StreamFunc, name string, args map[string]any, confirmed bool) (model.ActionRecord, []model.EntityRef, *model.ClarifyRequest, *platformtools.Result, error) {
	start := time.Now()
	action := model.ActionRecord{Tool: name, Args: stripSecretArgs(args), Status: model.ActionFailed}
	t, ok := a.reg.Get(name)
	if !ok {
		action.Error = "I can't do that from this conversation."
		action.DurationMs = time.Since(start).Milliseconds()
		return action, nil, nil, nil, nil
	}

	emit(stream, Event{Type: EventToolCall, Tool: name, Label: platformtools.HumanLabel(name), Args: action.Args})

	if a.guard != nil {
		if err := a.guard.Check(ctx, t, args, confirmed); err != nil {
			action.DurationMs = time.Since(start).Milliseconds()
			switch {
			case errors.Is(err, platformtools.ErrNeedsConfirm):
				action.Status = model.ActionPendingConfirmation
				action.Error = destructiveEffect(t, args)
			case errors.Is(err, platformtools.ErrDenied), errors.Is(err, platformtools.ErrPolicy):
				action.Status = model.ActionDenied
				action.Error = HumaniseError(err)
			default:
				action.Error = HumaniseError(err)
			}
			emit(stream, Event{Type: EventToolResult, Tool: name, Status: action.Status, DurationMs: action.DurationMs})
			return action, nil, nil, nil, nil
		}
	}

	toolCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	res, err := t.Run(toolCtx, platformtools.StripOrgArgs(args))
	cancel()
	action.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		action.Status = model.ActionFailed
		action.Error = HumaniseError(err)
		emit(stream, Event{Type: EventToolResult, Tool: name, Status: action.Status, DurationMs: action.DurationMs})
		return action, nil, nil, nil, nil
	}
	if res != nil && len(res.Missing) > 0 {
		q := res.Question
		if q == "" {
			q = "I need a bit more detail to continue."
		}
		clarify := &model.ClarifyRequest{Question: q, Slot: res.Missing[0], Options: res.Options}
		action.Status = model.ActionOK
		action.Error = ""
		emit(stream, Event{Type: EventToolResult, Tool: name, Status: "needs_input", DurationMs: action.DurationMs})
		return action, nil, clarify, res, nil
	}
	if res != nil && res.Question != "" && res.Data == nil {
		clarify := &model.ClarifyRequest{Question: res.Question, Options: res.Options}
		action.Status = model.ActionOK
		emit(stream, Event{Type: EventToolResult, Tool: name, Status: "needs_input", DurationMs: action.DurationMs})
		return action, nil, clarify, res, nil
	}

	action.Status = model.ActionOK
	var ents []model.EntityRef
	if res != nil {
		if res.Entity != nil {
			ents = append(ents, *res.Entity)
			action.ResultRef = res.Entity
		}
		ents = append(ents, res.Entities...)
	}
	var entity *model.EntityRef
	if len(ents) > 0 {
		entity = &ents[0]
	}
	emit(stream, Event{Type: EventToolResult, Tool: name, Status: action.Status, DurationMs: action.DurationMs, Entity: entity})
	return action, uniqueEntities(ents), nil, res, nil
}

func stripSecretArgs(args map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range args {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "token") || strings.Contains(lk, "secret") || strings.Contains(lk, "password") || lk == "api_key" || strings.HasSuffix(lk, "_key") {
			continue
		}
		out[k] = v
	}
	return out
}

func wrapToolResult(tool string, action model.ActionRecord, res *platformtools.Result, ents []model.EntityRef) string {
	payload := map[string]any{"status": action.Status}
	if action.Error != "" {
		payload["error"] = action.Error
	}
	if res != nil && res.Data != nil {
		payload["data"] = res.Data
	}
	if len(ents) > 0 {
		labels := make([]string, 0, len(ents))
		for _, e := range ents {
			labels = append(labels, e.Label)
		}
		payload["entities"] = labels
	}
	return platformtools.WrapUntrusted(tool, payload)
}

func disclosedFromResult(tool string, res *platformtools.Result) []string {
	if tool != "catalog_search" || res == nil {
		return nil
	}
	data, _ := res.Data.(map[string]any)
	if data == nil {
		return nil
	}
	switch v := data["names"].(type) {
	case []string:
		return v
	case []any:
		var out []string
		for _, x := range v {
			if s := asString(x); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func composeFromResult(action model.ActionRecord, ents []model.EntityRef, res *platformtools.Result) string {
	if action.Status == model.ActionDenied {
		if action.Error != "" {
			return action.Error
		}
		return "You don't have permission to do that."
	}
	if action.Status != model.ActionOK {
		if action.Error != "" {
			return action.Error
		}
		return "That didn't work."
	}
	if res != nil {
		if data, ok := res.Data.(map[string]any); ok {
			if h, ok := data["help"].(string); ok && h != "" {
				return h
			}
			if t := asString(data["title"]); t != "" {
				if p := asString(data["project"]); p != "" {
					return fmt.Sprintf("Created %s in %s.", t, p)
				}
				return fmt.Sprintf("Created %s.", t)
			}
			if fact := asString(data["remembered"]); fact != "" {
				return "I'll remember that."
			}
		}
	}
	if len(ents) > 0 {
		return fmt.Sprintf("Done. %s is ready.", ents[0].Label)
	}
	return "Done."
}

func lastFailureMessage(actions []model.ActionRecord) string {
	for i := len(actions) - 1; i >= 0; i-- {
		if actions[i].Status == model.ActionFailed || actions[i].Status == model.ActionDenied {
			if actions[i].Error != "" {
				return actions[i].Error
			}
		}
	}
	return "That didn't work. Nothing was completed."
}

func slotNames(c *model.ClarifyRequest) []string {
	if c == nil {
		return nil
	}
	if c.Slot != "" {
		return []string{c.Slot}
	}
	return []string{"input"}
}

func uniqueEntities(in []model.EntityRef) []model.EntityRef {
	seen := map[string]bool{}
	out := make([]model.EntityRef, 0, len(in))
	for _, e := range in {
		key := e.Kind + ":" + e.ID
		if e.ID == "" {
			key = e.Kind + ":" + e.Label
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return out
}

func appendUnique(dst, src []string) []string {
	set := map[string]bool{}
	for _, s := range dst {
		set[s] = true
	}
	for _, s := range src {
		if s != "" && !set[s] {
			dst = append(dst, s)
			set[s] = true
		}
	}
	return dst
}

func rollSummary(existing string, evicted []model.ChatMessage) string {
	var b strings.Builder
	if existing != "" {
		b.WriteString(existing)
		b.WriteString("\n")
	}
	for _, m := range evicted {
		if m.Role != model.ChatRoleUser && m.Role != model.ChatRoleAgent {
			continue
		}
		line := strings.TrimSpace(m.Content)
		if line == "" {
			continue
		}
		if len(line) > 240 {
			line = line[:240] + "…"
		}
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	s := b.String()
	if len(s) > 3000 {
		s = s[len(s)-3000:]
	}
	return s
}

func failResponse(msg string, start time.Time) model.ChatResponse {
	resp := model.ChatResponse{Message: SanitiseMessage(msg)}
	finaliseUsage(&resp, start, "")
	return resp
}

func finaliseUsage(resp *model.ChatResponse, start time.Time, modelName string) {
	if resp.Usage == nil {
		resp.Usage = &model.UsageInfo{}
	}
	resp.Usage.Model = modelName
	resp.Usage.LatencyMs = time.Since(start).Milliseconds()
}

func destructiveEffect(t platformtools.PlatformTool, args map[string]any) string {
	name := ""
	for _, k := range []string{"name", "title", "agent", "workflow", "target"} {
		if s := asString(args[k]); s != "" {
			name = s
			break
		}
	}
	label := platformtools.HumanLabel(t.Name())
	if name != "" {
		return fmt.Sprintf("%s %s. This cannot be undone from chat.", strings.TrimSuffix(label, "…"), name)
	}
	return fmt.Sprintf("%s This cannot be undone from chat.", label)
}
