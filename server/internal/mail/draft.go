package mail

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

// Drafter writes a reply body. It never sends.
type Drafter interface {
	Draft(ctx context.Context, msg InboxMessage, class ClassifyResult, brief *research.Brief, opts DraftOptions) (Draft, error)
}

// DraftOptions is operator guidance applied after research (or instead of it).
type DraftOptions struct {
	// ReplyInstructions is how the reply should read. Empty keeps the default
	// concise professional tone.
	ReplyInstructions string
	// PinnedKnowledge is true when findings (if any) came from the org's
	// pinned knowledge pages rather than an open-web search.
	PinnedKnowledge bool
	// Mailbox is the address the reply goes out from. Without it the model has
	// no one to sign as and reaches for "[Your Name]" — a draft with a blank in
	// it cannot be sent by the person approving it, which is the whole point of
	// the draft.
	Mailbox string
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

func (d *llmDrafter) Draft(ctx context.Context, msg InboxMessage, class ClassifyResult, brief *research.Brief, opts DraftOptions) (Draft, error) {
	if d.llm == nil {
		return HeuristicDraft(msg, brief), nil
	}
	prompt := BuildDraftPrompt(msg, class, brief, opts)
	draft, err := d.generate(ctx, msg, prompt)
	if err != nil {
		d.logger.Warn("mail: draft failed, using heuristic", zap.Error(err))
		return HeuristicDraft(msg, brief), nil
	}
	// When the reply is grounded in research, every money amount in the draft
	// must come from the findings, the sender's own email, or the operator's
	// instructions. The model has been observed inventing stale prices when the
	// pinned page states none — an honest "not listed" beats a wrong figure.
	if brief != nil || opts.PinnedKnowledge {
		allowed := []string{briefText(brief), msg.Subject, msg.Body, opts.ReplyInstructions}
		bad := unsupportedAmounts(draft.Body, allowed...)
		if len(bad) == 0 {
			return draft, nil
		}
		d.logger.Warn("mail: draft quoted amounts not in research, redrafting",
			zap.Strings("amounts", bad))
		redo, rerr := d.generate(ctx, msg, prompt+amountCorrection(bad))
		if rerr == nil && len(unsupportedAmounts(redo.Body, allowed...)) == 0 {
			return redo, nil
		}
		d.logger.Warn("mail: redraft still quoted unsourced amounts, using safe fallback")
		return noFigureDraft(msg), nil
	}
	return draft, nil
}

func (d *llmDrafter) generate(ctx context.Context, msg InboxMessage, prompt string) (Draft, error) {
	var out struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
		To      string `json:"to"`
		CC      string `json:"cc"`
	}
	err := llm.GenerateJSON(ctx, "mail-draft", prompt, &out,
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
		return Draft{}, err
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
		return Draft{}, errors.New("mail: model returned an empty draft body")
	}
	return draft, nil
}

const draftSystem = `You draft organisation email replies. Reply with JSON only: subject, body, to, cc.
The body is plain text. Be concise and professional.
You have not sent this message. Never say you have sent it, never invent a message-id, never claim the Research Agent found something it did not.
If research findings are provided, use only those facts; do not invent citations.
Write a finished reply. Never leave a blank for someone to fill in: no [Your Name],
no [Company], no {{placeholder}}. A person approves and sends this as written.`

// BuildDraftPrompt is exported so tests can assert research is folded in and
// the prompt forbids claiming the mail was sent.
func BuildDraftPrompt(msg InboxMessage, class ClassifyResult, brief *research.Brief, opts DraftOptions) string {
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
%s
From: %s <%s>
Subject: %s

%s
%s`, msg.FromEmail, class.Intent, class.SuggestedAction, class.Reason,
		draftOperatorGuidance(opts),
		msg.FromName, msg.FromEmail, msg.Subject, body, researchBlock)
}

func draftOperatorGuidance(opts DraftOptions) string {
	reply := strings.TrimSpace(opts.ReplyInstructions)
	mailbox := strings.TrimSpace(opts.Mailbox)
	if !opts.PinnedKnowledge && reply == "" && mailbox == "" {
		return ""
	}
	var b strings.Builder
	// The sign-off comes first because it is the rule most often broken: with
	// nobody named, a model invents a bracket for a human to fill in.
	if mailbox != "" {
		fmt.Fprintf(&b, "\nYou are writing from the shared mailbox %s. Sign off as that team.\n", mailbox)
		b.WriteString("Do not sign with a personal name and never leave a placeholder to fill in.\n")
	}
	if reply != "" || opts.PinnedKnowledge {
		if reply == "" {
			reply = "(none — be concise and professional)"
		}
		b.WriteString("\nREPLY INSTRUCTIONS FROM THE OPERATOR (follow these; they override default tone):\n")
		b.WriteString(reply)
		b.WriteByte('\n')
	}
	if opts.PinnedKnowledge {
		b.WriteString("Research findings come from the organisation's pinned knowledge pages.\n")
		b.WriteString("Use only those facts. Quote a price, version, date, or other figure ONLY if it appears in the findings.\n")
		b.WriteString("If the findings do not state the answer (or are empty), say it is not listed on those pages and we will follow up with exact details.\n")
		b.WriteString("Never fill the gap from memory, and never tell the sender to visit a website or retailer instead of answering.\n")
	}
	b.WriteString("Never claim this reply has been sent.\n")
	return b.String()
}

// amountRe matches money amounts like $1,999, £40, €2.499,00, "$ 5499".
var amountRe = regexp.MustCompile(`[$£€]\s?\d+(?:[.,]\d+)*`)

// unsupportedAmounts returns the money amounts in body that appear in none of
// the allowed source texts. Amounts compare with spaces and thousands commas
// stripped, so "$1,999" in the draft matches "$1999" in a finding.
func unsupportedAmounts(body string, allowed ...string) []string {
	ok := make(map[string]struct{})
	for _, src := range allowed {
		for _, a := range amountRe.FindAllString(src, -1) {
			ok[normalizeAmountKey(a)] = struct{}{}
		}
	}
	var out []string
	seen := make(map[string]struct{})
	for _, a := range amountRe.FindAllString(body, -1) {
		key := normalizeAmountKey(a)
		if _, fine := ok[key]; fine {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, strings.TrimSpace(a))
	}
	return out
}

func normalizeAmountKey(a string) string {
	a = strings.ReplaceAll(a, " ", "")
	return strings.ReplaceAll(a, ",", "")
}

// briefText flattens a brief into one string for the amount check.
func briefText(brief *research.Brief) string {
	if brief == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(brief.Summary)
	for _, f := range brief.Findings {
		b.WriteByte('\n')
		b.WriteString(f.Claim)
		b.WriteByte('\n')
		b.WriteString(f.Quote)
	}
	return b.String()
}

func amountCorrection(amounts []string) string {
	return fmt.Sprintf("\n\nIMPORTANT: your previous draft quoted %s, which appears neither in the research findings nor in the sender's email. Those figures may be wrong. Rewrite the reply without any figure that is not in the findings. If the findings do not state the figure, say it is not listed on our reference pages and we will follow up with exact details.",
		strings.Join(amounts, ", "))
}

// noFigureDraft is the fallback when the model keeps quoting figures that are
// not in the findings: an honest "we will confirm" beats an invented price.
func noFigureDraft(msg InboxMessage) Draft {
	return Draft{
		ThreadID: msg.GmailThreadID,
		Body: "Thanks for your email. The exact figures you asked about are not stated in our reference pages, " +
			"so rather than guess we will confirm the details and follow up with you shortly.",
		Subject: replySubject(msg.Subject),
		To:      msg.FromEmail,
		Status:  model.MailDraftDraft,
	}
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
