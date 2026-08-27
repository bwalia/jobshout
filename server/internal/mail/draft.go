package mail

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

// Drafter writes a reply body. It never sends.
type Drafter interface {
	Draft(ctx context.Context, msg InboxMessage, class ClassifyResult, brief *research.Brief) (Draft, error)
}

type llmDrafter struct {
	llm    llm.Client
	logger *zap.Logger
}

// NewDrafter returns an LLM drafter, falling back to HeuristicDraft.
func NewDrafter(client llm.Client, logger *zap.Logger) Drafter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &llmDrafter{llm: client, logger: logger}
}

func (d *llmDrafter) Draft(ctx context.Context, msg InboxMessage, class ClassifyResult, brief *research.Brief) (Draft, error) {
	if d.llm == nil {
		return HeuristicDraft(msg, brief), nil
	}
	var out struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
		To      string `json:"to"`
		CC      string `json:"cc"`
	}
	err := llm.GenerateJSON(ctx, "mail-draft", BuildDraftPrompt(msg, class, brief), &out,
		func(ctx context.Context, prompt string) (string, error) {
			resp, err := d.llm.Generate(ctx, llm.GenerateRequest{
				MaxTokens: 800,
				Messages: []llm.Message{
					{Role: llm.RoleSystem, Content: draftSystem},
					{Role: llm.RoleUser, Content: prompt},
				},
			})
			if err != nil {
				return "", err
			}
			return resp.Content, nil
		},
		func(reply string, err error) {
			d.logger.Warn("mail: draft JSON unreadable, retrying", zap.Error(err))
		},
	)
	if err != nil {
		d.logger.Warn("mail: draft failed, using heuristic", zap.Error(err))
		return HeuristicDraft(msg, brief), nil
	}
	draft := Draft{
		ThreadID: msg.GmailThreadID,
		Body:     strings.TrimSpace(out.Body),
		Subject:  strings.TrimSpace(out.Subject),
		To:       strings.TrimSpace(out.To),
		CC:       strings.TrimSpace(out.CC),
		Status:   model.MailDraftDraft,
	}
	if draft.To == "" {
		draft.To = msg.FromEmail
	}
	if draft.Subject == "" {
		draft.Subject = replySubject(msg.Subject)
	}
	if draft.Body == "" {
		return HeuristicDraft(msg, brief), nil
	}
	return draft, nil
}

const draftSystem = `You draft organisation email replies. Reply with JSON only: subject, body, to, cc.
The body is plain text. Be concise and professional.
You have not sent this message. Never say you have sent it, never invent a message-id, never claim the Research Agent found something it did not.
If research findings are provided, use only those facts; do not invent citations.`

// BuildDraftPrompt is exported so tests can assert research is folded in and
// the prompt forbids claiming the mail was sent.
func BuildDraftPrompt(msg InboxMessage, class ClassifyResult, brief *research.Brief) string {
	body := strings.TrimSpace(msg.Body)
	if len(body) > 4000 {
		body = body[:4000] + "\n…"
	}
	var researchBlock string
	if brief != nil && (brief.Summary != "" || len(brief.Findings) > 0) {
		var b strings.Builder
		b.WriteString("\nResearch findings (use only these facts):\n")
		if brief.Summary != "" {
			b.WriteString(brief.Summary)
			b.WriteByte('\n')
		}
		for i, f := range brief.Findings {
			if i >= 8 {
				break
			}
			fmt.Fprintf(&b, "- %s (source: %s)\n", f.Claim, f.SourceURL)
		}
		researchBlock = b.String()
	}

	return fmt.Sprintf(`Draft a reply to this email. Return JSON:
{"subject":"...","body":"...","to":"%s","cc":""}

Triage: intent=%s action=%s reason=%s
Do not claim the reply has been sent.

From: %s <%s>
Subject: %s

%s
%s`, msg.FromEmail, class.Intent, class.SuggestedAction, class.Reason,
		msg.FromName, msg.FromEmail, msg.Subject, body, researchBlock)
}

// HeuristicDraft is a safe fallback that never claims to have sent.
func HeuristicDraft(msg InboxMessage, brief *research.Brief) Draft {
	body := "Thanks for your email — we have received it and will follow up shortly."
	if brief != nil && strings.TrimSpace(brief.Summary) != "" {
		body = "Thanks for getting in touch.\n\n" + strings.TrimSpace(brief.Summary) +
			"\n\nPlease let us know if you need anything else."
	}
	return Draft{
		ThreadID: msg.GmailThreadID,
		Body:     body,
		Subject:  replySubject(msg.Subject),
		To:       msg.FromEmail,
		Status:   model.MailDraftDraft,
	}
}

func replySubject(subject string) string {
	s := strings.TrimSpace(subject)
	if s == "" {
		return "Re: your email"
	}
	if strings.HasPrefix(strings.ToLower(s), "re:") {
		return s
	}
	return "Re: " + s
}
