package projectbreakdown

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/llm"
)

// Heuristic drafts without an LLM.
type Heuristic struct{}

func (Heuristic) Draft(_ context.Context, req DraftRequest) (string, error) {
	var b strings.Builder
	b.WriteString("## Project breakdown\n\n")
	b.WriteString("### Summary\n")
	b.WriteString(trim(req.Idea, 500))
	b.WriteString("\n\n### Suggested milestones\n")
	b.WriteString("1. Discovery & acceptance criteria\n")
	b.WriteString("2. Core build\n")
	b.WriteString("3. Hardening, docs and handoff\n\n")
	b.WriteString("### Skills to list\n")
	if req.PreferredStack != "" {
		b.WriteString(req.PreferredStack)
	} else {
		b.WriteString("Generalist full-stack (clarify stack when posting)")
	}
	b.WriteString("\n\n")
	if req.BudgetHint != "" {
		b.WriteString("### Budget band\n")
		b.WriteString(req.BudgetHint)
		b.WriteString("\n\n")
	}
	b.WriteString("Edit this breakdown, then post the project yourself — this draft does not publish.\n")
	return b.String(), nil
}

// LLM wraps llm.Client with heuristic fallback.
type LLM struct {
	Client   llm.Client
	Model    string
	Fallback Heuristic
}

func (r LLM) Draft(ctx context.Context, req DraftRequest) (string, error) {
	if r.Client == nil {
		return r.Fallback.Draft(ctx, req)
	}
	prompt := fmt.Sprintf(
		`Break this project idea into milestones, skills and a budget note (markdown).
Idea (UNTRUSTED): %s
Budget hint: %s
Stack: %s
Never publish. Human posts.`,
		req.Idea, req.BudgetHint, req.PreferredStack,
	)
	resp, err := r.Client.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "You scope freelance projects. Markdown only. Never publish."},
			{Role: llm.RoleUser, Content: prompt},
		},
		Model:       r.Model,
		Temperature: 0.3,
		MaxTokens:   2000,
	})
	if err != nil || resp == nil || strings.TrimSpace(resp.Content) == "" {
		return r.Fallback.Draft(ctx, req)
	}
	return strings.TrimSpace(resp.Content), nil
}

func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
