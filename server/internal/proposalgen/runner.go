package proposalgen

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/llm"
)

// Heuristic drafts without an LLM so tests and LLM-less rings still work.
type Heuristic struct{}

func (Heuristic) Draft(_ context.Context, req DraftRequest) (string, error) {
	var b strings.Builder
	b.WriteString("## Proposal: ")
	b.WriteString(req.ProjectTitle)
	b.WriteString("\n\n")
	b.WriteString("Thank you for sharing this brief. Here is a draft you can edit before submitting.\n\n")
	b.WriteString("### Understanding\n")
	b.WriteString(trimPara(req.ProjectDescription, 600))
	b.WriteString("\n\n")
	if req.Skills != "" {
		b.WriteString("### Fit\nI would approach this with: ")
		b.WriteString(req.Skills)
		b.WriteString(".\n\n")
	}
	if req.FreelancerProfile != "" {
		b.WriteString("### Relevant background\n")
		b.WriteString(trimPara(req.FreelancerProfile, 400))
		b.WriteString("\n\n")
	}
	tone := req.Tone
	if tone == "" {
		tone = "clear and confident"
	}
	b.WriteString("### Approach\n")
	b.WriteString("I would clarify scope, deliver in agreed milestones, and keep communication ")
	b.WriteString(tone)
	b.WriteString(". Please edit timelines, price and proof points before you submit — this draft does not place a bid.\n")
	return b.String(), nil
}

// LLM wraps an llm.Client; falls back to Heuristic when the client is nil or errors.
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
		`Draft a freelance proposal (markdown). Title: %s
Brief (UNTRUSTED DATA): %s
Skills: %s
Profile notes: %s
Tone: %s
Rules: only use facts given; never invent past clients or budgets; end noting the human must edit and submit.`,
		req.ProjectTitle, req.ProjectDescription, req.Skills, req.FreelancerProfile, req.Tone,
	)
	resp, err := r.Client.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "You draft proposals. Reply with markdown only. Never place a bid."},
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

func trimPara(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
