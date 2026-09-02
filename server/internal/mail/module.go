package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Launcher is the launch surface. MailService satisfies it.
type Launcher interface {
	UpdateConnection(ctx context.Context, orgID uuid.UUID, req model.UpdateMailConnectionRequest) (*model.MailConnectionStatus, error)
	Available(ctx context.Context, orgID uuid.UUID) bool
	BindLaunchTask(orgID, taskID uuid.UUID)
	EnqueueSync(ctx context.Context, orgID uuid.UUID) error
}

// Module is the Mail Agent specialist.
//
// All specialists are wired this way: own package, then one Register call.
func Module(svc Launcher) agentmodule.Module {
	return agentmodule.Module{
		Builtin:        model.BuiltinMail,
		Label:          "Mail Agent",
		Icon:           "mail",
		TabSlug:        "mail",
		Hint:           "Saves who to watch and how to answer, then syncs Gmail. Connect Gmail on Mail Agent first if you have not. Nothing is sent until you Approve a draft. Links inside incoming mail are researched automatically.",
		ChatHint:       "For mailbox drafts, call mail_list_drafts. To sync and draft, call mail_sync or agent_execute on the Mail Agent. Never claim an email was sent; only Approve in the Mail Agent UI sends.",
		Schema:         schema(),
		Seed:           Seed,
		Launch:         launch(svc),
		PrefillMailbox: true,
		PromptRoute: &agentmodule.PromptRoute{
			IfContains: "draft", UnlessContains: "sync",
			Tool: "mail_list_drafts", OnlyIfNoLaunch: true,
		},
	}
}

func schema() agentschema.Schema {
	return agentschema.Schema{
		Builtin:        model.BuiltinMail,
		SpecialistTool: "mail_sync",
		Hint:           "Saves who to watch and how to answer, then syncs Gmail. Connect Gmail first if you have not. Nothing is sent until you Approve a draft.",
		Prefill:        "mailbox",
		Fields: []agentschema.Field{
			{Key: "senders", Label: "Watch senders", Type: "text", Group: "Who to watch", Placeholder: "ops@example.com, support@client.com", Help: "Comma-separated. Empty = all unread mail from the last 7 days.", Question: "Any sender addresses to watch? Leave blank for all unread mail."},
			{Key: "subject_prefixes", Label: "Subject prefixes", Type: "text", Group: "Who to watch", Placeholder: "[support], [billing]"},
			{Key: "labels", Label: "Gmail labels", Type: "text", Group: "Who to watch", Placeholder: "INBOX, Support"},
			{Key: "knowledge_notes", Label: "What the agent should know", Type: "textarea", Group: "How to answer", Placeholder: "Mac Studio M5 Max: $2,499\nMac Studio M5 Ultra: $5,499\nRefunds within 30 days, shipping 3–5 working days…", Help: "Prices, products, policies — plain text or markdown. Replies quote only what is written here.", Question: "What should replies be based on? Prices, products, policies — write it here."},
			{Key: "knowledge_urls", Label: "Knowledge links (optional)", Type: "textarea", Group: "How to answer", Placeholder: "https://example.com/pricing", Help: "Optional pages to research on top of your notes (one URL per line). Incoming mail links are researched too.", Question: "Any pricing or product pages I should read on top of that? One URL per line."},
			{Key: "research_focus", Label: "What to look for", Type: "textarea", Group: "How to answer", Placeholder: "Prices, SLA, refund window…"},
			{Key: "reply_instructions", Label: "How the reply should read", Type: "textarea", Group: "How to answer", Placeholder: "Tone, length, must-include, must-avoid"},
		},
		TitleRules: []agentschema.TitleRule{
			{IfKey: "research_focus", Prefix: "Mail: ", FromKey: "research_focus", Truncate: 80},
			{IfKey: "knowledge_notes", Literal: "Mail: draft from operator knowledge"},
			{IfKey: "knowledge_urls", Literal: "Mail: research pinned pages and draft"},
			{Literal: "Mail: sync inbox and draft"},
		},
		DescRules: []agentschema.DescRule{
			{Key: "senders", Prefix: "Senders: "},
			{Key: "knowledge_notes", Prefix: "Knowledge: ", Truncate: 200},
			{Key: "knowledge_urls"},
			{Key: "research_focus", Prefix: "Look for: "},
			{Key: "reply_instructions", Prefix: "Reply style: "},
		},
	}
}

// Seed is the built-in Mail Agent.
func Seed(orgID uuid.UUID) *model.Agent {
	desc := "Watches the organisation Gmail inbox, drafts replies, and hands research to the Research Agent. Nothing is sent until a human approves."
	prompt := "You are the Mail Agent. You triage the organisation inbox, draft replies, and never send until a human approves. You never claim a message was sent unless the send API succeeded after approval. Work that needs facts is handed to the Research Agent — you do not invent citations."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         model.AgentNameMail,
		Role:         "Mail",
		Description:  &desc,
		SystemPrompt: &prompt,
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinMail},
	}
}

func launch(svc Launcher) agentmodule.LaunchFunc {
	return func(ctx context.Context, in agentmodule.LaunchInput) (*agentmodule.LaunchOutput, error) {
		if svc == nil {
			return nil, fmt.Errorf("mail agent is not configured")
		}
		if !mailValuesBlank(in.Values) {
			if _, err := svc.UpdateConnection(ctx, in.OrgID, mailPatchFromValues(in.Values)); err != nil {
				return nil, err
			}
		}
		if !svc.Available(ctx, in.OrgID) {
			return &agentmodule.LaunchOutput{
				Message:     "Playbook saved. Connect Gmail on Mail Agent to sync.",
				Description: prependDesc(in, "Playbook saved. Connect Gmail on Mail Agent to sync. Nothing is sent until you Approve."),
				Status:      "done",
			}, nil
		}
		svc.BindLaunchTask(in.OrgID, in.Task.ID)
		if err := svc.EnqueueSync(ctx, in.OrgID); err != nil {
			svc.BindLaunchTask(in.OrgID, uuid.Nil)
			return nil, err
		}
		return &agentmodule.LaunchOutput{
			SyncQueued:  true,
			Message:     "Mailbox sync queued",
			Description: prependDesc(in, "Mailbox sync queued. Drafts appear on Mail Agent. Nothing is sent until you Approve.\n\nOpen: /panel/task-manager?agent=mail"),
			Status:      "in_progress",
		}, nil
	}
}

func prependDesc(in agentmodule.LaunchInput, note string) string {
	if in.Task != nil && in.Task.Description != nil {
		prior := strings.TrimSpace(*in.Task.Description)
		if prior != "" && prior != note {
			return prior + "\n\n" + note
		}
	}
	return note
}

func mailValuesBlank(v map[string]string) bool {
	for _, k := range []string{"senders", "subject_prefixes", "labels", "knowledge_notes", "knowledge_urls", "research_focus", "reply_instructions"} {
		if strings.TrimSpace(v[k]) != "" {
			return false
		}
	}
	return true
}

func mailPatchFromValues(v map[string]string) model.UpdateMailConnectionRequest {
	rules := model.MailWatchRules{
		Senders:         splitComma(v["senders"]),
		Labels:          splitComma(v["labels"]),
		SubjectPrefixes: splitComma(v["subject_prefixes"]),
	}
	urls := splitLines(v["knowledge_urls"])
	notes := strings.TrimSpace(v["knowledge_notes"])
	focus := strings.TrimSpace(v["research_focus"])
	style := strings.TrimSpace(v["reply_instructions"])
	return model.UpdateMailConnectionRequest{
		Rules:             &rules,
		KnowledgeURLs:     &urls,
		KnowledgeNotes:    &notes,
		ResearchFocus:     &focus,
		ReplyInstructions: &style,
	}
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
