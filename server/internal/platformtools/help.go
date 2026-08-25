package platformtools

import (
	"context"

	"github.com/jobshout/server/internal/tools"
)

const helpText = `I can run your agents, create and update work, start workflows, and look up status — from this conversation.

Things I can do:
• List, describe, create, update, pause or run agents
• Create, update, comment on and move tasks; list and create projects and sprints
• Run workflows and multi-agent jobs, and tell you who is working on what
• Research a topic, write an article, generate an image, review a GitHub pull request, or start a pentest (in scope only)
• Check usage, budgets, policies and what you are allowed to do

Ask in plain language. If I need a project, agent or workflow I will ask. Destructive actions (delete, cancel a pentest) wait for your explicit approval — I will not guess.

Sessions started here in the web app are separate from Telegram. I remember this conversation, including names like "that agent" and "the login timeout task".`

func registerHelp(reg *Registry) {
	reg.Register(newTool(
		"help",
		"Explain what you can do for the user in this conversation.",
		"config",
		"",
		false, true,
		tools.ObjectSchema(map[string]any{}),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			return &Result{Data: map[string]any{"help": helpText}}, nil
		},
	))
}

func registerRemember(reg *Registry, d Deps) {
	if d.Memory == nil {
		return
	}
	reg.Register(newTool(
		"remember",
		"Store a durable fact the user asked you to remember (for example a staging URL). Only call this when the user explicitly asks to remember something.",
		"config",
		"",
		false, false,
		tools.ObjectSchema(map[string]any{
			"fact": map[string]any{"type": "string", "description": "The standalone fact to remember"},
		}, "fact"),
		func(ctx context.Context, input map[string]any) (*Result, error) {
			ident := MustIdentity(ctx)
			fact := strArg(input, "fact")
			if fact == "" {
				return &Result{Missing: []string{"fact"}, Question: "What should I remember?"}, nil
			}
			if err := d.Memory.Append(ctx, ident.UserID, ident.OrgID, fact, fact); err != nil {
				return nil, err
			}
			return &Result{Data: map[string]any{"remembered": fact}}, nil
		},
	))
}
