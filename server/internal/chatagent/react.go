package chatagent

import (
	"context"
	"encoding/json"
	"strings"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
)

// turnCaller produces the next model step: either tool calls or a final answer.
//
// It is the seam that lets the rest of the loop — guard chain, confirmation,
// clarify, entities, disclosure — run identically whether the model does native
// tool-calling or the ReAct JSON-in-prompt fallback. Both implementations
// return tool requests as llm.ToolCall values, which is all the loop reads.
type turnCaller interface {
	next(ctx context.Context, messages []llm.Message, defs []llm.ToolDef) (*llm.GenerateResponse, error)
}

// nativeCaller drives models whose client supports native tool-calling.
type nativeCaller struct {
	client llm.Client
}

func (n nativeCaller) next(ctx context.Context, messages []llm.Message, defs []llm.ToolDef) (*llm.GenerateResponse, error) {
	return n.client.Generate(ctx, llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   2048,
		Temperature: 0.2,
		ToolDefs:    defs,
	})
}

// reactCaller drives models with no native tool-calling: the tools are rendered
// into the prompt and the model must answer with one JSON object per turn,
// either {"tool":"<name>","args":{...}} or {"final":"<answer>"}.
type reactCaller struct {
	client llm.Client
	logger *zap.Logger
}

// reactRetryNudge corrects a reply that was not the required JSON object.
const reactRetryNudge = "Your previous reply was not a single valid JSON object. " +
	`Reply with ONLY one JSON object — either {"tool":"<name>","args":{...}} to call a tool, ` +
	`or {"final":"<your answer>"} to answer the user. No prose, no code fence.`

func (r reactCaller) next(ctx context.Context, messages []llm.Message, defs []llm.ToolDef) (*llm.GenerateResponse, error) {
	msgs := reactMessages(messages, defs)

	resp, err := r.client.Generate(ctx, llm.GenerateRequest{
		Messages:    msgs,
		MaxTokens:   2048,
		Temperature: 0.2,
	})
	if err != nil {
		return nil, err
	}

	if parsed, ok := parseReactReply(resp); ok {
		return parsed, nil
	}

	// One corrective retry, then treat whatever came back as the final answer —
	// an honest degradation: unparsed text can never masquerade as a tool run.
	r.logger.Warn("chatagent: unparseable ReAct reply, retrying once",
		zap.String("raw", snippet(resp.Content)))
	retryMsgs := append(append([]llm.Message{}, msgs...), llm.Message{
		Role: llm.RoleSystem, Content: reactRetryNudge,
	})
	retry, err := r.client.Generate(ctx, llm.GenerateRequest{
		Messages:    retryMsgs,
		MaxTokens:   2048,
		Temperature: 0.2,
	})
	if err != nil {
		return nil, err
	}
	retry.InputTokens += resp.InputTokens
	retry.OutputTokens += resp.OutputTokens
	if parsed, ok := parseReactReply(retry); ok {
		return parsed, nil
	}
	r.logger.Warn("chatagent: ReAct retry still unparseable, treating as final answer",
		zap.String("raw", snippet(retry.Content)))
	retry.ToolCalls = nil
	return retry, nil
}

// parseReactReply maps the protocol JSON onto a GenerateResponse the loop can
// consume: a "tool" reply becomes a one-element ToolCalls slice, a "final"
// reply becomes plain content. ok is false when the reply fits neither shape.
func parseReactReply(resp *llm.GenerateResponse) (*llm.GenerateResponse, bool) {
	var reply struct {
		Tool  string         `json:"tool"`
		Args  map[string]any `json:"args"`
		Final string         `json:"final"`
	}
	if err := llm.DecodeJSON(resp.Content, &reply); err != nil {
		return nil, false
	}
	switch {
	case reply.Tool != "":
		args := reply.Args
		if args == nil {
			args = map[string]any{}
		}
		out := *resp
		// Content keeps the raw JSON so the assistant turn echoed into history
		// shows the model its own request in the protocol's shape.
		out.ToolCalls = []llm.ToolCall{{ID: "call_0", Name: reply.Tool, Arguments: args}}
		return &out, true
	case reply.Final != "":
		out := *resp
		out.Content = reply.Final
		out.ToolCalls = nil
		return &out, true
	}
	return nil, false
}

// reactMessages translates the native-shaped history into the ReAct protocol:
// the system prompt gains the protocol instruction and tool list, assistant
// tool requests stay as their raw JSON content, and tool results become user
// messages (their content is already wrapped with the untrusted delimiters and
// tool name). Native-only fields are stripped so no provider echoes tool_calls
// to a model that cannot accept them.
func reactMessages(messages []llm.Message, defs []llm.ToolDef) []llm.Message {
	out := make([]llm.Message, 0, len(messages))
	for i, m := range messages {
		switch {
		case i == 0 && m.Role == llm.RoleSystem:
			out = append(out, llm.Message{Role: llm.RoleSystem, Content: m.Content + "\n" + reactProtocol(defs)})
		case m.Role == llm.RoleTool:
			out = append(out, llm.Message{Role: llm.RoleUser, Content: m.Content})
		case len(m.ToolCalls) > 0:
			content := m.Content
			if strings.TrimSpace(content) == "" {
				content = renderCallsJSON(m.ToolCalls)
			}
			out = append(out, llm.Message{Role: m.Role, Content: content})
		default:
			out = append(out, llm.Message{Role: m.Role, Content: m.Content})
		}
	}
	return out
}

// reactProtocol renders the protocol instruction plus the turn's available
// tools (name, description, compact parameter schema).
func reactProtocol(defs []llm.ToolDef) string {
	var b strings.Builder
	b.WriteString(`
How to act — you have no function-calling mechanism, so you MUST follow this protocol exactly.
Reply with ONLY one JSON object per turn, nothing else:
- To call a tool: {"tool":"<name>","args":{...}}
- To answer the user: {"final":"<your answer>"}
Call one tool at a time and wait for its result before the next step.

Available tools:
`)
	for _, d := range defs {
		b.WriteString("- ")
		b.WriteString(d.Name)
		if d.Description != "" {
			b.WriteString(": ")
			b.WriteString(d.Description)
		}
		if len(d.Parameters) > 0 {
			if schema, err := json.Marshal(d.Parameters); err == nil {
				b.WriteString(" | args schema: ")
				b.Write(schema)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderCallsJSON(calls []llm.ToolCall) string {
	var b strings.Builder
	for _, tc := range calls {
		args := tc.Arguments
		if args == nil {
			args = map[string]any{}
		}
		raw, err := json.Marshal(map[string]any{"tool": tc.Name, "args": args})
		if err != nil {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.Write(raw)
	}
	return b.String()
}

func snippet(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

// turnCallerFor selects the mode once per Run, mirroring the executor's guard:
// native function-calling when the client advertises it for its model, the
// ReAct fallback otherwise. There is no third state — a model with no tool
// mechanism at all never sees action-shaped requests unprotocolled.
func turnCallerFor(client llm.Client, logger *zap.Logger) (turnCaller, string) {
	if tc, ok := client.(llm.ToolCapableClient); ok && tc.SupportsTools() {
		return nativeCaller{client: client}, "native"
	}
	return reactCaller{client: client, logger: logger}, "react"
}
