package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/integration"
	"github.com/jobshout/server/internal/model"
)

// --- fakes ---

type fakeIntegStore struct {
	cfgs []model.Integration
	err  error
}

func (f *fakeIntegStore) ListByOrg(context.Context, uuid.UUID) ([]model.Integration, error) {
	return f.cfgs, f.err
}

type fakeNotifStore struct {
	cfgs []model.NotificationConfig
	err  error
}

func (f *fakeNotifStore) ListByOrg(context.Context, uuid.UUID) ([]model.NotificationConfig, error) {
	return f.cfgs, f.err
}

type fakeTaskAdapter struct {
	name      string
	createID  string
	createURL string
	getIssue  *integration.ExternalIssue
	err       error
	lastIssue integration.ExternalIssue
}

func (a *fakeTaskAdapter) Name() string { return a.name }
func (a *fakeTaskAdapter) CreateIssue(_ context.Context, issue integration.ExternalIssue) (string, string, error) {
	a.lastIssue = issue
	return a.createID, a.createURL, a.err
}
func (a *fakeTaskAdapter) UpdateIssue(context.Context, string, integration.ExternalIssue) error {
	return a.err
}
func (a *fakeTaskAdapter) GetIssue(context.Context, string) (*integration.ExternalIssue, error) {
	return a.getIssue, a.err
}
func (a *fakeTaskAdapter) DeleteIssue(context.Context, string) error { return a.err }

type fakeNotifAdapter struct {
	name    string
	err     error
	lastMsg integration.NotificationMessage
	sent    bool
}

func (a *fakeNotifAdapter) Name() string { return a.name }
func (a *fakeNotifAdapter) Send(_ context.Context, msg integration.NotificationMessage) error {
	a.lastMsg = msg
	a.sent = true
	return a.err
}
func (a *fakeNotifAdapter) Test(context.Context) error { return a.err }

type fakeAdapters struct {
	task  integration.TaskAdapter
	notif integration.NotificationAdapter
	err   error
}

func (f *fakeAdapters) GetTask(model.Integration) (integration.TaskAdapter, error) {
	return f.task, f.err
}
func (f *fakeAdapters) GetNotification(model.NotificationConfig) (integration.NotificationAdapter, error) {
	return f.notif, f.err
}

func orgCtx() context.Context {
	return WithOrg(context.Background(), uuid.New())
}

// --- tests ---

func TestCreateIssueTool(t *testing.T) {
	jiraCfg := []model.Integration{{Provider: "jira"}}

	tests := []struct {
		name       string
		ctx        context.Context
		store      IntegrationConfigStore
		adapters   AdapterBuilder
		input      map[string]any
		wantErr    string
		wantSubstr string
	}{
		{
			name:       "success",
			ctx:        orgCtx(),
			store:      &fakeIntegStore{cfgs: jiraCfg},
			adapters:   &fakeAdapters{task: &fakeTaskAdapter{name: "jira", createID: "PROJ-1", createURL: "https://x/browse/PROJ-1"}},
			input:      map[string]any{"title": "Fix login", "priority": "high"},
			wantSubstr: "PROJ-1",
		},
		{
			name:     "no org in context",
			ctx:      context.Background(),
			store:    &fakeIntegStore{cfgs: jiraCfg},
			adapters: &fakeAdapters{task: &fakeTaskAdapter{}},
			input:    map[string]any{"title": "x"},
			wantErr:  "no organization in context",
		},
		{
			name:     "no jira integration configured",
			ctx:      orgCtx(),
			store:    &fakeIntegStore{cfgs: []model.Integration{{Provider: "github"}}},
			adapters: &fakeAdapters{task: &fakeTaskAdapter{}},
			input:    map[string]any{"title": "x"},
			wantErr:  "no jira integration is configured",
		},
		{
			name:     "missing required title",
			ctx:      orgCtx(),
			store:    &fakeIntegStore{cfgs: jiraCfg},
			adapters: &fakeAdapters{task: &fakeTaskAdapter{}},
			input:    map[string]any{},
			wantErr:  "missing required parameter",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool := &createIssueTool{provider: "jira", store: tc.store, adapters: tc.adapters}
			out, err := tool.Execute(tc.ctx, tc.input)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(out, tc.wantSubstr) {
				t.Fatalf("want output containing %q, got %q", tc.wantSubstr, out)
			}
		})
	}

	// The correct tool name is derived from the provider.
	if got := (&createIssueTool{provider: "github"}).Name(); got != "github_create_issue" {
		t.Fatalf("name: got %q", got)
	}
}

func TestGetIssueTool(t *testing.T) {
	tool := &getIssueTool{
		provider: "jira",
		store:    &fakeIntegStore{cfgs: []model.Integration{{Provider: "jira"}}},
		adapters: &fakeAdapters{task: &fakeTaskAdapter{getIssue: &integration.ExternalIssue{ExternalID: "PROJ-7", Title: "Bug", Status: "in_progress"}}},
	}
	out, err := tool.Execute(orgCtx(), map[string]any{"external_id": "PROJ-7"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "PROJ-7") || !strings.Contains(out, "in_progress") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestSendMessageTool(t *testing.T) {
	slackEnabled := []model.NotificationConfig{{ChannelType: "slack", Enabled: true}}

	t.Run("success", func(t *testing.T) {
		na := &fakeNotifAdapter{name: "slack"}
		tool := &sendMessageTool{name: "slack_send_message", channel: "slack",
			store: &fakeNotifStore{cfgs: slackEnabled}, adapters: &fakeAdapters{notif: na}}
		out, err := tool.Execute(orgCtx(), map[string]any{"message": "deploy finished"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out != "sent" || !na.sent || na.lastMsg.TaskTitle != "deploy finished" {
			t.Fatalf("send not wired correctly: out=%q sent=%v msg=%q", out, na.sent, na.lastMsg.TaskTitle)
		}
	})

	t.Run("channel disabled", func(t *testing.T) {
		tool := &sendMessageTool{name: "slack_send_message", channel: "slack",
			store:    &fakeNotifStore{cfgs: []model.NotificationConfig{{ChannelType: "slack", Enabled: false}}},
			adapters: &fakeAdapters{notif: &fakeNotifAdapter{}}}
		_, err := tool.Execute(orgCtx(), map[string]any{"message": "x"})
		if err == nil || !strings.Contains(err.Error(), "no enabled slack") {
			t.Fatalf("want disabled-channel error, got %v", err)
		}
	})

	t.Run("store error propagates", func(t *testing.T) {
		tool := &sendMessageTool{name: "slack_send_message", channel: "slack",
			store: &fakeNotifStore{err: errors.New("db down")}, adapters: &fakeAdapters{}}
		_, err := tool.Execute(orgCtx(), map[string]any{"message": "x"})
		if err == nil || !strings.Contains(err.Error(), "db down") {
			t.Fatalf("want propagated store error, got %v", err)
		}
	})
}

func TestNewIntegrationToolsSet(t *testing.T) {
	got := NewIntegrationTools(&fakeIntegStore{}, &fakeNotifStore{}, &fakeAdapters{})
	want := map[string]bool{
		"jira_create_issue": true, "jira_get_issue": true,
		"github_create_issue": true, "github_get_issue": true,
		"slack_send_message": true, "teams_send_message": true, "email_send": true,
	}
	if len(got) != len(want) {
		t.Fatalf("want %d tools, got %d", len(want), len(got))
	}
	for _, tl := range got {
		if !want[tl.Name()] {
			t.Fatalf("unexpected tool %q", tl.Name())
		}
	}
}
