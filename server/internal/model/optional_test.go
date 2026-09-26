package model

import (
	"encoding/json"
	"testing"
)

func TestOptionalString_NullClears(t *testing.T) {
	var req UpdateTaskRequest
	if err := json.Unmarshal([]byte(`{"assigned_agent_id":null}`), &req); err != nil {
		t.Fatal(err)
	}
	if !req.AssignedAgentID.Set {
		t.Fatal("null must be present")
	}
	if req.AssignedAgentID.Value != nil {
		t.Fatalf("null must clear, got %v", *req.AssignedAgentID.Value)
	}
}

func TestOptionalString_OmittedUnchanged(t *testing.T) {
	var req UpdateTaskRequest
	if err := json.Unmarshal([]byte(`{"title":"keep"}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.AssignedAgentID.Set {
		t.Fatal("omitted assigned_agent_id must not be set")
	}
	if req.Title == nil || *req.Title != "keep" {
		t.Fatalf("title = %v", req.Title)
	}
}

func TestOptionalString_Value(t *testing.T) {
	var req UpdateTaskRequest
	if err := json.Unmarshal([]byte(`{"assigned_agent_id":"11111111-1111-1111-1111-111111111111"}`), &req); err != nil {
		t.Fatal(err)
	}
	if !req.AssignedAgentID.Set || req.AssignedAgentID.Value == nil {
		t.Fatal("expected a value")
	}
	if *req.AssignedAgentID.Value != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("got %q", *req.AssignedAgentID.Value)
	}
}

func TestPaginationParams_CapIs200(t *testing.T) {
	p := PaginationParams{Page: 0, PerPage: 500}
	p.Normalize()
	if p.Page != 1 || p.PerPage != 200 {
		t.Fatalf("got page=%d per_page=%d", p.Page, p.PerPage)
	}
}

func TestUpdateTaskRequest_StatusUnmarshals(t *testing.T) {
	var req UpdateTaskRequest
	if err := json.Unmarshal([]byte(`{"status":"done"}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.Status == nil || *req.Status != "done" {
		t.Fatalf("status = %v", req.Status)
	}
}
