package metalead

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientListFormsAndLeads(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v21.0/page1/leadgen_forms", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "access_token=tok") {
			t.Errorf("missing token in %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "form9", "name": "Demo"}},
		})
	})
	mux.HandleFunc("/v21.0/form9/leads", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"id":           "lead1",
				"created_time": "2026-10-01T12:00:00+0000",
				"field_data": []map[string]any{
					{"name": "email", "values": []string{"a@example.com"}},
				},
			}},
		})
	})
	mux.HandleFunc("/v21.0/lead1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "lead1", "form_id": "form9", "created_time": "2026-10-01T12:00:00Z",
			"field_data": []map[string]any{{"name": "email", "values": []string{"a@example.com"}}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewClient("v21.0")
	c.BaseURL = srv.URL
	c.HTTP = srv.Client()

	forms, err := c.ListForms(context.Background(), "page1", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if len(forms) != 1 || forms[0].ID != "form9" {
		t.Fatalf("forms = %+v", forms)
	}

	since := time.Unix(1, 0)
	leads, err := c.ListLeads(context.Background(), "form9", "tok", &since, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(leads) != 1 || leads[0].ID != "lead1" || len(leads[0].FieldData) != 1 {
		t.Fatalf("leads = %+v", leads)
	}

	detail, err := c.GetLead(context.Background(), "lead1", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if detail.FormID != "form9" || detail.FieldData[0].Values[0] != "a@example.com" {
		t.Fatalf("detail = %+v", detail)
	}
}

func TestParseCreatedTime(t *testing.T) {
	if ParseCreatedTime("") != nil {
		t.Fatal("empty should be nil")
	}
	if ParseCreatedTime("2026-10-01T12:00:00+0000") == nil {
		t.Fatal("expected parse")
	}
}
