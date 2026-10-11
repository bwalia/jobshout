package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
)

// --- engines: the sidecar cannot pause, so it refuses gated agents ---

type countingRunner struct{ n int }

func (c *countingRunner) Run(context.Context, uuid.UUID, *model.Agent, string, []string) executor.Result {
	c.n++
	return executor.Result{FinalAnswer: "sidecar"}
}

type namedGate map[string]bool

func (g namedGate) RequiresApproval(_ uuid.UUID, tool string) bool { return g[tool] }

func TestEngineRouter_SidecarRefusesGatedTools(t *testing.T) {
	cases := []struct {
		engine  string
		tools   []string
		gate    ToolGate
		wantErr bool
	}{
		{model.EngineLangChain, []string{"web_search", "send_email"}, namedGate{"send_email": true}, true},
		{model.EngineLangGraph, []string{"send_email"}, namedGate{"send_email": true}, true},
		{model.EngineLangChain, []string{"web_search"}, namedGate{"send_email": true}, false},
		{model.EngineLangGraph, []string{"send_email"}, nil, false},
		{model.EngineGoNative, []string{"send_email"}, namedGate{"send_email": true}, false},
	}
	for _, tc := range cases {
		native, sidecar := &countingRunner{}, &countingRunner{}
		r := NewRouter(native, sidecar, sidecar, zap.NewNop())
		if tc.gate != nil {
			r.WithApprovalGate(tc.gate)
		}
		res := r.For(tc.engine).Run(context.Background(), uuid.New(), &model.Agent{ID: uuid.New()}, "p", tc.tools)
		if (res.Err != nil) != tc.wantErr {
			t.Fatalf("%s %v: err=%v, wantErr=%v", tc.engine, tc.tools, res.Err, tc.wantErr)
		}
		if tc.wantErr && !strings.Contains(res.Err.Error(), model.EngineGoNative) {
			t.Fatalf("%s: the refusal should point at %s: %v", tc.engine, model.EngineGoNative, res.Err)
		}
		if tc.wantErr && sidecar.n != 0 {
			t.Fatalf("%s: a refused run must never reach the sidecar", tc.engine)
		}
	}
}
