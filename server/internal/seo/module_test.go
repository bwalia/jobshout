package seo

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/model"
)

type stubRunner struct {
	last model.CreateSEORunRequest
}

func (s *stubRunner) CreateRun(_ context.Context, req model.CreateSEORunRequest, _ uuid.UUID, _ *uuid.UUID) (*model.SEORun, error) {
	s.last = req
	return &model.SEORun{ID: uuid.New(), Status: "queued", Mode: req.Mode, Source: req.Source}, nil
}

func TestLaunch_URLHappyPath(t *testing.T) {
	stub := &stubRunner{}
	mod := Module(stub)
	out, err := mod.Launch(context.Background(), agentmodule.LaunchInput{
		OrgID:  uuid.New(),
		UserID: uuid.New(),
		Agent:  Seed(uuid.New()),
		Task:   &model.Task{ID: uuid.New()},
		Values: map[string]string{
			"source":   "urls",
			"keywords": "devops, jobs",
			"urls":     "https://example.com/\nhttps://example.com/about",
			"mode":     "analyze",
			"max_pages": "5",
		},
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if out == nil || out.RunID == nil {
		t.Fatal("expected run id")
	}
	if stub.last.Source != "urls" {
		t.Fatalf("source: %q", stub.last.Source)
	}
	if stub.last.Keywords != "devops, jobs" {
		t.Fatalf("keywords: %q", stub.last.Keywords)
	}
	if stub.last.MaxPages != 5 {
		t.Fatalf("max_pages: %d", stub.last.MaxPages)
	}
	if stub.last.URLs == "" {
		t.Fatal("expected urls payload")
	}
}

func TestLaunch_GitHubRequiresConnection(t *testing.T) {
	stub := &stubRunner{}
	mod := Module(stub)
	_, err := mod.Launch(context.Background(), agentmodule.LaunchInput{
		OrgID:  uuid.New(),
		UserID: uuid.New(),
		Agent:  Seed(uuid.New()),
		Task:   &model.Task{ID: uuid.New()},
		Values: map[string]string{
			"source":   "github",
			"keywords": "hiring",
			"mode":     "analyze",
		},
	})
	if err == nil {
		t.Fatal("expected error without github_connection")
	}
}

func TestSchema_NoTokenField(t *testing.T) {
	for _, f := range schema().Fields {
		switch f.Key {
		case "access_token", "token", "github_token", "pat":
			t.Fatalf("schema must not expose credential field %q", f.Key)
		}
	}
}
