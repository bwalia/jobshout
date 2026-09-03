package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentpack"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/service"
)

type stubPackSvc struct {
	exportErr  error
	previewErr error
	importErr  error
	pkg        *agentpack.Package
	filename   string
	preview    *service.PackPreview
	result     *service.ImportAgentResult
	report     agentpack.Report
}

func (s *stubPackSvc) Export(context.Context, uuid.UUID, uuid.UUID) (*agentpack.Package, string, error) {
	return s.pkg, s.filename, s.exportErr
}
func (s *stubPackSvc) Preview(context.Context, uuid.UUID, *agentpack.Package) (*service.PackPreview, error) {
	return s.preview, s.previewErr
}
func (s *stubPackSvc) ResolvePreview(context.Context, uuid.UUID, service.ImportAgentRequest) (agentpack.Report, error) {
	return s.report, s.previewErr
}
func (s *stubPackSvc) Import(context.Context, uuid.UUID, uuid.UUID, service.ImportAgentRequest) (*service.ImportAgentResult, error) {
	return s.result, s.importErr
}

func withPackAuth(r *http.Request, orgID, userID uuid.UUID) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID.String())
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID.String())
	return r.WithContext(ctx)
}

func TestAgentPackExportNotFound(t *testing.T) {
	h := NewAgentPackHandler(&stubPackSvc{exportErr: repository.ErrAgentPackNotFound}, nil)
	org, user, agent := uuid.New(), uuid.New(), uuid.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+agent.String()+"/export", nil)
	req = withPackAuth(req, org, user)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agentID", agent.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	h.Export(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestAgentPackExportForbiddenOtherOrg(t *testing.T) {
	h := NewAgentPackHandler(&stubPackSvc{exportErr: repository.ErrAgentPackForbidden}, nil)
	org, user, agent := uuid.New(), uuid.New(), uuid.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+agent.String()+"/export", nil)
	req = withPackAuth(req, org, user)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agentID", agent.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	h.Export(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestAgentPackExportDownloadHeaders(t *testing.T) {
	pkg := &agentpack.Package{
		Kind: agentpack.Kind, SchemaVersion: 1,
		Agent:    agentpack.Body{Name: "Helper", Role: "QA"},
		Warnings: []string{"Credentials, API keys, and OAuth tokens are not included."},
	}
	h := NewAgentPackHandler(&stubPackSvc{pkg: pkg, filename: "helper-20260903.jobshout-agent.json"}, nil)
	org, user, agent := uuid.New(), uuid.New(), uuid.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+agent.String()+"/export", nil)
	req = withPackAuth(req, org, user)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agentID", agent.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	h.Export(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != agentpack.ContentType {
		t.Fatalf("content-type %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "helper-20260903.jobshout-agent.json") {
		t.Fatalf("disposition %q", rec.Header().Get("Content-Disposition"))
	}
	if rec.Header().Get("X-Agent-Pack-Warnings") == "" {
		t.Fatal("expected warnings header")
	}
}

func TestAgentPackPreviewRejectsWrongKind(t *testing.T) {
	h := NewAgentPackHandler(&stubPackSvc{previewErr: fmt.Errorf("not a JobShout agent package (kind %q)", "nope")}, nil)
	org, user := uuid.New(), uuid.New()
	body, _ := json.Marshal(map[string]any{
		"package": map[string]any{"kind": "nope", "schema_version": 1, "agent": map[string]any{"name": "A", "role": "B"}},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/import/preview", bytes.NewReader(body))
	req = withPackAuth(req, org, user)
	h.Preview(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestAgentPackPreviewTooLarge(t *testing.T) {
	h := NewAgentPackHandler(&stubPackSvc{previewErr: fmt.Errorf("package exceeds %d bytes", agentpack.MaxJSONBytes)}, nil)
	org, user := uuid.New(), uuid.New()
	body, _ := json.Marshal(map[string]any{
		"package": map[string]any{"kind": agentpack.Kind, "schema_version": 1, "agent": map[string]any{"name": "A", "role": "B"}},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/import/preview", bytes.NewReader(body))
	req = withPackAuth(req, org, user)
	h.Preview(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestAgentPackImportCreated(t *testing.T) {
	id := uuid.New()
	h := NewAgentPackHandler(&stubPackSvc{
		report: agentpack.Report{Mode: agentpack.ModeCreate, CanUndo: true},
		result: &service.ImportAgentResult{
			Agent:   &model.Agent{ID: id, Name: "Imported"},
			Mode:    agentpack.ModeCreate,
			CanUndo: true,
		},
	}, nil)
	org, user := uuid.New(), uuid.New()
	body, _ := json.Marshal(map[string]any{"preview_id": uuid.New().String()})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/import", bytes.NewReader(body))
	req = withPackAuth(req, org, user)
	h.Import(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}
