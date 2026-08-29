package mail

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
)

// Classifier turns an inbound message into structured triage.
type Classifier interface {
	Classify(ctx context.Context, msg InboxMessage) (ClassifyResult, error)
}

type llmClassifier struct {
	llm    llm.Client
	logger *zap.Logger
}

// NewClassifier returns an LLM classifier, falling back to HeuristicClassify
// when llm is nil or a call fails.
func NewClassifier(client llm.Client, logger *zap.Logger) Classifier {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &llmClassifier{llm: client, logger: logger}
}

func (c *llmClassifier) Classify(ctx context.Context, msg InboxMessage) (ClassifyResult, error) {
	if c.llm == nil {
		return HeuristicClassify(msg), nil
	}
	var out ClassifyResult
	err := llm.GenerateJSON(ctx, "mail-classify", BuildClassifyPrompt(msg), &out,
		func(ctx context.Context, prompt string) (string, error) {
			resp, err := c.llm.Generate(ctx, llm.GenerateRequest{
				MaxTokens: 400,
				Messages: []llm.Message{
					{Role: llm.RoleSystem, Content: classifySystem},
					{Role: llm.RoleUser, Content: prompt},
				},
			})
			if err != nil {
				return "", err
			}
			return resp.Content, nil
		},
		func(reply string, err error) {
			c.logger.Warn("mail: classifier JSON unreadable, retrying", zap.Error(err))
		},
	)
	if err != nil {
		c.logger.Warn("mail: classifier failed, using heuristic", zap.Error(err))
		return HeuristicClassify(msg), nil
	}
	return normaliseClassify(out), nil
}

const classifySystem = `You triage inbound organisation email. Reply with JSON only.
Never claim a message has been sent. You draft later; this step is classification only.`

// BuildClassifyPrompt is exported so tests can assert the fixture email is in the prompt.
func BuildClassifyPrompt(msg InboxMessage) string {
	body := strings.TrimSpace(msg.Body)
	if len(body) > 4000 {
		body = body[:4000] + "\n…"
	}
	return fmt.Sprintf(`Classify this inbound email.

Return JSON:
{
  "intent": "question" | "request" | "fyi" | "spam" | "other",
  "needs_research": boolean,
  "urgency": "low" | "normal" | "high",
  "suggested_action": "reply" | "ignore" | "escalate",
  "reason": "one line",
  "triage_label": "short label e.g. support, sales, newsletter"
}

needs_research is true when a useful reply needs current facts from the web (versions, outages, docs, prices) that are not already in the email, or when the email includes a product/docs URL that should be read before drafting. Small talk, thanks, scheduling, and newsletters do not need research.
suggested_action is ignore for newsletters, bounce mail, no-reply notifications, and obvious spam.

From: %s <%s>
To: %s
Subject: %s

%s`, msg.FromName, msg.FromEmail, msg.ToEmail, msg.Subject, body)
}

// ParseClassifyResult decodes a model reply into ClassifyResult.
func ParseClassifyResult(reply string) (ClassifyResult, error) {
	var out ClassifyResult
	if err := llm.DecodeJSON(reply, &out); err != nil {
		return ClassifyResult{}, err
	}
	return normaliseClassify(out), nil
}

func normaliseClassify(c ClassifyResult) ClassifyResult {
	c.Intent = strings.ToLower(strings.TrimSpace(c.Intent))
	c.Urgency = strings.ToLower(strings.TrimSpace(c.Urgency))
	c.SuggestedAction = strings.ToLower(strings.TrimSpace(c.SuggestedAction))
	c.TriageLabel = strings.TrimSpace(c.TriageLabel)
	c.Reason = strings.TrimSpace(c.Reason)
	switch c.Intent {
	case "question", "request", "fyi", "spam", "other":
	default:
		c.Intent = "other"
	}
	switch c.Urgency {
	case "low", "normal", "high":
	default:
		c.Urgency = "normal"
	}
	switch c.SuggestedAction {
	case "reply", "ignore", "escalate":
	default:
		c.SuggestedAction = "reply"
	}
	if c.TriageLabel == "" {
		c.TriageLabel = "general"
	}
	if c.Reason == "" {
		c.Reason = "Inbound mail."
	}
	return c
}

// HeuristicClassify is the offline fallback used in tests and when the LLM is down.
func HeuristicClassify(msg InboxMessage) ClassifyResult {
	blob := strings.ToLower(msg.Subject + "\n" + msg.FromEmail + "\n" + msg.Body)
	from := strings.ToLower(msg.FromEmail)
	out := ClassifyResult{
		Intent:          "request",
		NeedsResearch:   false,
		Urgency:         "normal",
		SuggestedAction: "reply",
		Reason:          "Heuristic triage (no LLM).",
		TriageLabel:     "general",
	}
	if strings.Contains(from, "noreply") || strings.Contains(from, "no-reply") ||
		(strings.Contains(blob, "unsubscribe") && (strings.Contains(blob, "view in browser") || strings.Contains(blob, "this newsletter"))) {
		out.Intent = "fyi"
		out.SuggestedAction = "ignore"
		out.TriageLabel = "newsletter"
		out.Reason = "Looks like automated or newsletter mail."
		return out
	}
	if strings.Contains(blob, "?") {
		out.Intent = "question"
	}
	researchHints := []string{"latest", "current version", "what's new", "what is the", "documentation", "changelog", "outage", "status of"}
	for _, h := range researchHints {
		if strings.Contains(blob, h) {
			out.NeedsResearch = true
			out.TriageLabel = "research"
			out.Reason = "The question looks like it needs current facts."
			break
		}
	}
	if !out.NeedsResearch && len(ExtractInboundURLs(msg.Subject, msg.Body)) > 0 {
		out.NeedsResearch = true
		out.TriageLabel = "research"
		out.Reason = "The email includes a link to research before drafting."
	}
	return out
}
