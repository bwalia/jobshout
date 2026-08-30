package chatagent

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/platformtools"
)

type fakeRecall struct {
	hits []string
}

func (f fakeRecall) Recall(context.Context, uuid.UUID, string, int) ([]string, error) {
	return f.hits, nil
}

func TestEval_C1_ResearchAsksProjectThenCreatesTask(t *testing.T) {
	var launched map[string]any
	reg := platformtools.NewRegistry()
	reg.Register(platformtools.TestingTool("research_run", "", false, func(_ context.Context, in map[string]any) (*platformtools.Result, error) {
		if strings.TrimSpace(str(in, "topic")) == "" {
			return &platformtools.Result{Missing: []string{"topic"}, Question: "What should I research?"}, nil
		}
		if strings.TrimSpace(str(in, "project")) == "" {
			return &platformtools.Result{
				Missing:  []string{"project"},
				Question: "Which project should this research live on?",
				Options: []model.ClarifyOption{
					{Label: "Platform", Value: "Platform"},
					{Label: "Website", Value: "Website"},
				},
			}, nil
		}
		launched = in
		tid := uuid.New().String()
		pid := uuid.New().String()
		tref := model.EntityRef{Kind: model.EntityTask, ID: tid, Label: "Research: kubernetes cost optimisation", Href: "/panel/task-manager?task=" + tid}
		pref := model.EntityRef{Kind: model.EntityProject, ID: pid, Label: str(in, "project"), Href: "/panel/task-manager?project=" + pid}
		return &platformtools.Result{
			Data:     map[string]any{"kind": "researcher", "task": tref.Label, "status": "done"},
			Entity:   &tref,
			Entities: []model.EntityRef{tref, pref},
		}, nil
	}))
	client := &scriptedLLM{steps: []llm.GenerateResponse{
		{ToolCalls: []llm.ToolCall{{ID: "1", Name: "research_run", Arguments: map[string]any{"topic": "kubernetes cost optimisation"}}}},
	}}
	a := New(client, reg, nil, nil, zap.NewNop())
	tr, err := a.Run(context.Background(), TurnRequest{Ident: ident(), Message: "research kubernetes cost optimisation", Metadata: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Response.Clarify == nil || tr.Response.Clarify.Slot != "project" {
		t.Fatalf("expected project interview, got %+v", tr.Response.Clarify)
	}

	tr2, err := a.Run(context.Background(), TurnRequest{
		Ident: ident(), Message: "Website", Metadata: tr.Metadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	if launched == nil || str(launched, "project") != "Website" {
		t.Fatalf("launch values = %+v", launched)
	}
	ents := readEntities(tr2.Metadata)
	if ents["last_task"].Kind != model.EntityTask {
		t.Fatalf("last_task missing: %+v", ents)
	}
	if ents["last_project"].Label != "Website" {
		t.Fatalf("last_project = %+v", ents["last_project"])
	}
}

func TestEval_C2_ArticleInterviewsTopicThenLaunches(t *testing.T) {
	var launched map[string]any
	reg := platformtools.NewRegistry()
	reg.Register(platformtools.TestingTool("article_generate", "", false, func(_ context.Context, in map[string]any) (*platformtools.Result, error) {
		if strings.TrimSpace(str(in, "topic")) == "" {
			return &platformtools.Result{Missing: []string{"topic"}, Question: "What should I write about?"}, nil
		}
		launched = in
		tid := uuid.New().String()
		tref := model.EntityRef{Kind: model.EntityTask, ID: tid, Label: "Write: edge AI", Href: "/panel/task-manager?task=" + tid}
		return &platformtools.Result{
			Data:   map[string]any{"kind": "article_writer", "task": tref.Label, "status": "started"},
			Entity: &tref,
		}, nil
	}))
	client := &scriptedLLM{steps: []llm.GenerateResponse{
		{ToolCalls: []llm.ToolCall{{ID: "1", Name: "article_generate", Arguments: map[string]any{}}}},
	}}
	a := New(client, reg, nil, nil, zap.NewNop())
	tr, err := a.Run(context.Background(), TurnRequest{Ident: ident(), Message: "write an article", Metadata: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Response.Clarify == nil || tr.Response.Clarify.Slot != "topic" {
		t.Fatalf("expected topic interview, got %+v", tr.Response.Clarify)
	}

	tr2, err := a.Run(context.Background(), TurnRequest{
		Ident: ident(), Message: "edge AI", Metadata: tr.Metadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	if str(launched, "topic") != "edge AI" {
		t.Fatalf("topic = %q", str(launched, "topic"))
	}
	if readEntities(tr2.Metadata)["last_task"].Kind != model.EntityTask {
		t.Fatal("article launch must persist last_task")
	}
}

func TestEval_C3_MailSyncNeverClaimsSent(t *testing.T) {
	reg := platformtools.NewRegistry()
	reg.Register(platformtools.TestingTool("mail_sync", "", false, func(context.Context, map[string]any) (*platformtools.Result, error) {
		tid := uuid.New().String()
		tref := model.EntityRef{Kind: model.EntityTask, ID: tid, Label: "Mail: sync inbox and draft", Href: "/panel/task-manager?task=" + tid}
		return &platformtools.Result{
			Data: map[string]any{
				"kind":        "mail",
				"sync_queued": true,
				"status":      "queued",
				"message":     "Mailbox sync queued. Nothing is sent until you Approve.",
				"task":        tref.Label,
			},
			Entity: &tref,
		}, nil
	}))
	client := &scriptedLLM{steps: []llm.GenerateResponse{
		{ToolCalls: []llm.ToolCall{{ID: "1", Name: "mail_sync", Arguments: map[string]any{}}}},
		{Content: "Mailbox sync is queued. Drafts will appear on Mail Agent. Nothing was sent."},
	}}
	a := New(client, reg, nil, nil, zap.NewNop())
	tr, err := a.Run(context.Background(), TurnRequest{Ident: ident(), Message: "sync gmail and draft replies to support mail"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Response.Actions) != 1 || tr.Response.Actions[0].Status != model.ActionOK {
		t.Fatalf("actions = %+v", tr.Response.Actions)
	}
	msg := strings.ToLower(tr.Response.Message)
	if strings.Contains(msg, "has been sent") || strings.Contains(msg, "i sent") || strings.Contains(msg, "email was sent") {
		t.Fatalf("must not claim sent: %q", tr.Response.Message)
	}
	if readEntities(tr.Metadata)["last_task"].Kind != model.EntityTask {
		t.Fatal("mail launch must create a board task entity")
	}
}

func TestEval_C4_ThinResearchPromptAsksTopic(t *testing.T) {
	invented := false
	reg := platformtools.NewRegistry()
	reg.Register(platformtools.TestingTool("agent_execute", "", false, func(_ context.Context, in map[string]any) (*platformtools.Result, error) {
		topic := strings.TrimSpace(str(in, "topic"))
		prompt := strings.TrimSpace(str(in, "prompt"))
		if topic == "" {
			// A thin "run the research agent" must not be treated as a topic.
			if prompt != "" && !strings.EqualFold(strings.TrimSpace(strings.TrimSuffix(strings.ToLower(prompt), " agent")), "research") {
				if !strings.Contains(strings.ToLower(prompt), "run the research") {
					invented = true
				}
			}
			return &platformtools.Result{Missing: []string{"topic"}, Question: "What should I research?"}, nil
		}
		if strings.EqualFold(topic, "research agent") || strings.EqualFold(topic, "the research agent") {
			invented = true
		}
		return &platformtools.Result{Data: map[string]any{"topic": topic}}, nil
	}))
	client := &scriptedLLM{steps: []llm.GenerateResponse{
		{ToolCalls: []llm.ToolCall{{ID: "1", Name: "agent_execute", Arguments: map[string]any{"name": "Research Agent"}}}},
	}}
	a := New(client, reg, nil, nil, zap.NewNop())
	tr, err := a.Run(context.Background(), TurnRequest{Ident: ident(), Message: "run the research agent", Metadata: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if invented {
		t.Fatal("thin prompt invented a topic")
	}
	if tr.Response.Clarify == nil || tr.Response.Clarify.Slot != "topic" {
		t.Fatalf("expected topic interview, got %+v", tr.Response.Clarify)
	}
}

func TestEval_C5_FollowUpReusesLastAgent(t *testing.T) {
	var second map[string]any
	reg := platformtools.NewRegistry()
	reg.Register(platformtools.TestingTool("agent_execute", "", false, func(_ context.Context, in map[string]any) (*platformtools.Result, error) {
		if strings.TrimSpace(str(in, "topic")) == "" {
			return &platformtools.Result{Missing: []string{"topic"}, Question: "What should I research?"}, nil
		}
		second = in
		aref := model.EntityRef{Kind: model.EntityAgent, ID: "ag-1", Label: "Research Agent", Href: "/panel/task-manager?agent=ag-1"}
		tref := model.EntityRef{Kind: model.EntityTask, ID: uuid.New().String(), Label: "Research: " + str(in, "topic")}
		return &platformtools.Result{
			Data:     map[string]any{"agent": "Research Agent", "topic": str(in, "topic")},
			Entity:   &tref,
			Entities: []model.EntityRef{aref, tref},
		}, nil
	}))
	client := &scriptedLLM{steps: []llm.GenerateResponse{
		{ToolCalls: []llm.ToolCall{{ID: "1", Name: "agent_execute", Arguments: map[string]any{"name": "Research Agent", "topic": "prod costs"}}}},
		{Content: "Research on prod costs is on the board."},
		{ToolCalls: []llm.ToolCall{{ID: "2", Name: "agent_execute", Arguments: map[string]any{"name": "Research Agent", "topic": "staging costs"}}}},
		{Content: "Doing the same for staging."},
	}}
	a := New(client, reg, nil, nil, zap.NewNop())
	tr, err := a.Run(context.Background(), TurnRequest{Ident: ident(), Message: "research prod costs", Metadata: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if readEntities(tr.Metadata)["last_agent"].Label != "Research Agent" {
		t.Fatalf("last_agent = %+v", readEntities(tr.Metadata)["last_agent"])
	}

	tr2, err := a.Run(context.Background(), TurnRequest{
		Ident: ident(), Message: "do the same for staging", Metadata: tr.Metadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	if str(second, "topic") != "staging costs" {
		t.Fatalf("follow-up topic = %q", str(second, "topic"))
	}
	if readEntities(tr2.Metadata)["last_agent"].Label != "Research Agent" {
		t.Fatal("last_agent should survive the follow-up")
	}
}

func TestEval_C6_RememberedConstraintReachesPrompt(t *testing.T) {
	var sawSystem string
	reg := platformtools.NewRegistry()
	reg.Register(platformtools.TestingTool("article_generate", "", false, func(context.Context, map[string]any) (*platformtools.Result, error) {
		return &platformtools.Result{Data: map[string]any{"status": "started"}}, nil
	}))
	client := &captureSysLLM{scriptedLLM: scriptedLLM{steps: []llm.GenerateResponse{
		{ToolCalls: []llm.ToolCall{{ID: "1", Name: "article_generate", Arguments: map[string]any{"topic": "async runtimes"}}}},
		{Content: "Started the article."},
	}}}
	a := New(client, reg, nil, fakeRecall{hits: []string{"we only write about rust"}}, zap.NewNop())
	_, err := a.Run(context.Background(), TurnRequest{Ident: ident(), Message: "write an article about async runtimes"})
	if err != nil {
		t.Fatal(err)
	}
	sawSystem = client.sys
	if !strings.Contains(sawSystem, "we only write about rust") {
		t.Fatalf("remembered constraint missing from system prompt:\n%s", sawSystem)
	}
}

func str(in map[string]any, k string) string {
	if in == nil {
		return ""
	}
	v, _ := in[k].(string)
	return v
}

type captureSysLLM struct {
	scriptedLLM
	sys string
}

func (s *captureSysLLM) Generate(_ context.Context, req llm.GenerateRequest) (*llm.GenerateResponse, error) {
	if len(req.Messages) > 0 && req.Messages[0].Role == llm.RoleSystem {
		s.sys = req.Messages[0].Content
	}
	return s.scriptedLLM.Generate(context.Background(), req)
}
