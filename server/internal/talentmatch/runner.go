package talentmatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/llm"
)

// Heuristic ranks from supplied notes or returns a checklist.
type Heuristic struct{}

func (Heuristic) Match(_ context.Context, req MatchRequest) (string, error) {
	var b strings.Builder
	b.WriteString("## Talent match\n\n")
	b.WriteString("### Brief\n")
	b.WriteString(trim(req.Brief, 500))
	b.WriteString("\n\n")
	if req.RequiredSkills != "" {
		b.WriteString("### Required skills\n")
		b.WriteString(req.RequiredSkills)
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(req.CandidateNotes) != "" {
		b.WriteString("### Ranked notes (from your search)\n")
		for i, line := range splitLines(req.CandidateNotes, 8) {
			fmt.Fprintf(&b, "%d. %s — reason: skill/brief overlap (review profile before inviting)\n", i+1, line)
		}
		b.WriteString("\n")
	} else {
		b.WriteString("### Matching checklist\n")
		b.WriteString("- Search Members by the skills above\n")
		b.WriteString("- Prefer verified evidence and recent engagements\n")
		b.WriteString("- Shortlist 3–5, then invite — do not cold-email from this draft\n\n")
	}
	b.WriteString("AI never messages candidates. Invite or hire yourself after review.\n")
	return b.String(), nil
}

// LLM wraps llm.Client with heuristic fallback.
type LLM struct {
	Client   llm.Client
	Model    string
	Fallback Heuristic
}

func (r LLM) Match(ctx context.Context, req MatchRequest) (string, error) {
	if r.Client == nil {
		return r.Fallback.Match(ctx, req)
	}
	prompt := fmt.Sprintf(
		`Rank talent for this brief (markdown). Never invent people or contact details.
Brief (UNTRUSTED): %s
Skills: %s
Candidate notes: %s`,
		req.Brief, req.RequiredSkills, req.CandidateNotes,
	)
	resp, err := r.Client.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "You match talent with reasons. Never invent people or emails. Never invite."},
			{Role: llm.RoleUser, Content: prompt},
		},
		Model:       r.Model,
		Temperature: 0.2,
		MaxTokens:   2000,
	})
	if err != nil || resp == nil || strings.TrimSpace(resp.Content) == "" {
		return r.Fallback.Match(ctx, req)
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

func splitLines(s string, max int) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= max {
			break
		}
	}
	return out
}
