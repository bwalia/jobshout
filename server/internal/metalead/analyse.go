package metalead

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
)

// Completer is the LLM call analysis needs.
type Completer interface {
	Complete(ctx context.Context, model, prompt string, maxTokens int, jsonMode bool) (string, error)
}

type clientCompleter struct {
	client       llm.Client
	defaultModel string
}

// NewClientCompleter adapts an llm.Client.
func NewClientCompleter(c llm.Client, defaultModel string) Completer {
	if c == nil {
		return nil
	}
	return &clientCompleter{client: c, defaultModel: defaultModel}
}

func (c *clientCompleter) Complete(ctx context.Context, modelName, prompt string, maxTokens int, jsonMode bool) (string, error) {
	if strings.TrimSpace(modelName) == "" {
		modelName = c.defaultModel
	}
	resp, err := c.client.Generate(ctx, llm.GenerateRequest{
		Model:       modelName,
		MaxTokens:   maxTokens,
		Temperature: 0.3,
		JSON:        jsonMode,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: systemPrompt},
			{Role: llm.RoleUser, Content: prompt},
		},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

const systemPrompt = `You are a B2B lead qualification analyst for Meta Lead Ads Instant Forms.
You only use the field data provided — never invent emails, phones, names or company facts.
Reply with a single JSON object and nothing else.`

// Analyser turns lead field data into briefs and a rollup.
type Analyser struct {
	LLM   Completer
	Model string
}

// AnalyseLead returns a per-lead brief.
func (a *Analyser) AnalyseLead(ctx context.Context, fields []FieldData, focus string) (*model.MetaLeadBrief, error) {
	if a == nil || a.LLM == nil {
		return nil, fmt.Errorf("meta lead analyst: no LLM")
	}
	prompt := leadPrompt(fields, focus)
	raw, err := a.LLM.Complete(ctx, a.Model, prompt, 1200, true)
	if err != nil {
		return nil, err
	}
	return decodeBrief(raw)
}

// AnalyseRollup summarises a batch of briefs.
func (a *Analyser) AnalyseRollup(ctx context.Context, briefs []model.MetaLeadBrief, focus string) (*model.MetaLeadRollup, error) {
	if a == nil || a.LLM == nil {
		return nil, fmt.Errorf("meta lead analyst: no LLM")
	}
	prompt := rollupPrompt(briefs, focus)
	raw, err := a.LLM.Complete(ctx, a.Model, prompt, 2000, true)
	if err != nil {
		return nil, err
	}
	return decodeRollup(raw, len(briefs))
}

func leadPrompt(fields []FieldData, focus string) string {
	var sb strings.Builder
	sb.WriteString("Analyse this Meta Instant Form lead. Return JSON:\n")
	sb.WriteString(`{"intent":"...","fit_score":0-100,"next_action":"...","spam_risk":"low|medium|high","flags":["..."],"summary":"..."}` + "\n\n")
	if strings.TrimSpace(focus) != "" {
		fmt.Fprintf(&sb, "Qualification focus: %s\n\n", strings.TrimSpace(focus))
	}
	sb.WriteString("Field data:\n")
	for _, f := range fields {
		fmt.Fprintf(&sb, "- %s: %s\n", f.Name, strings.Join(f.Values, ", "))
	}
	return sb.String()
}

func rollupPrompt(briefs []model.MetaLeadBrief, focus string) string {
	var sb strings.Builder
	sb.WriteString("Summarise these lead briefs. Return JSON:\n")
	sb.WriteString(`{"themes":["..."],"quality_mix":"...","recommended_followups":["..."],"lead_count":N}` + "\n\n")
	if strings.TrimSpace(focus) != "" {
		fmt.Fprintf(&sb, "Qualification focus: %s\n\n", strings.TrimSpace(focus))
	}
	b, _ := json.MarshalIndent(briefs, "", "  ")
	sb.Write(b)
	return sb.String()
}

func decodeBrief(raw string) (*model.MetaLeadBrief, error) {
	raw = extractJSON(raw)
	var b model.MetaLeadBrief
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		return nil, fmt.Errorf("brief JSON: %w", err)
	}
	if b.FitScore < 0 {
		b.FitScore = 0
	}
	if b.FitScore > 100 {
		b.FitScore = 100
	}
	if b.SpamRisk == "" {
		b.SpamRisk = "low"
	}
	if b.Flags == nil {
		b.Flags = []string{}
	}
	return &b, nil
}

func decodeRollup(raw string, n int) (*model.MetaLeadRollup, error) {
	raw = extractJSON(raw)
	var r model.MetaLeadRollup
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, fmt.Errorf("rollup JSON: %w", err)
	}
	if r.LeadCount == 0 {
		r.LeadCount = n
	}
	if r.Themes == nil {
		r.Themes = []string{}
	}
	if r.RecommendedFollowups == nil {
		r.RecommendedFollowups = []string{}
	}
	return &r, nil
}

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			return s[i : j+1]
		}
	}
	return s
}
