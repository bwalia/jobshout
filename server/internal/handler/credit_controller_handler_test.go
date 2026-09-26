package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/creditcontroller"
	"github.com/jobshout/server/internal/service"
)

func TestCreditControllerDemoModeHTTP(t *testing.T) {
	t.Parallel()
	h := NewCreditControllerHandler(service.NewCreditControllerService(
		creditcontroller.NewClient(creditcontroller.Config{}, nil)))

	cases := []struct {
		name string
		fn   http.HandlerFunc
		req  *http.Request
		want int
	}{
		{"status", h.Status, httptest.NewRequest(http.MethodGet, "/status", nil), http.StatusOK},
		{"summary", h.Summary, httptest.NewRequest(http.MethodGet, "/summary", nil), http.StatusOK},
		{"invoices", h.ListInvoices, httptest.NewRequest(http.MethodGet, "/invoices", nil), http.StatusOK},
		{"queue", h.Queue, httptest.NewRequest(http.MethodGet, "/queue", nil), http.StatusOK},
		{"triage", h.Triage, httptest.NewRequest(http.MethodPost, "/triage", strings.NewReader(`{"invoice_id":"INV-1005"}`)), http.StatusConflict},
		{"generate", h.Generate, httptest.NewRequest(http.MethodPost, "/invoices/generate", strings.NewReader(`{"cadence":"weekly"}`)), http.StatusConflict},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		tc.fn(rec, tc.req)
		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	h.Status(rec, httptest.NewRequest(http.MethodGet, "/status", nil))
	var st map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st["mode"] != "demo" || st["ok"] != true {
		t.Fatalf("status body = %v", st)
	}
}
