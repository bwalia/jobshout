package opsapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateMarketingTemplate_PostsDraftEmail(t *testing.T) {
	var got map[string]any
	var gotAuth, gotNS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/opsapi-marketing/templates" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		gotAuth, gotNS = r.Header.Get("Authorization"), r.Header.Get("X-Namespace-Slug")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"success":true,"data":{"uuid":"t-1","name":"Email: hi","channel":"email","status":"draft"}}`))
	}))
	defer srv.Close()

	tpl, err := newTestClient(t, srv.URL).CreateMarketingTemplate(context.Background(), MarketingTemplateRequest{
		Name: "Email: hi", Subject: "Hi", BodyHTML: "<p>Hi {{contact.first_name}}</p>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tpl.UUID != "t-1" {
		t.Errorf("template = %+v", tpl)
	}
	if gotAuth != "Bearer "+testKey || gotNS != "acme" {
		t.Errorf("auth = %q ns = %q", gotAuth, gotNS)
	}
	if got["channel"] != "email" || got["status"] != "draft" || got["body_html"] != "<p>Hi {{contact.first_name}}</p>" {
		t.Errorf("body = %v", got)
	}
}

func TestSaveMarketingTemplate_StudioReturnsAnalysis(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v2/opsapi-marketing/templates/t-1/studio" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"success":true,"data":{"template":{"uuid":"t-1","name":"Email: hi","status":"draft"},
			"analysis":{"ok":true,"warnings":[],"merge_fields":["contact.first_name","unsubscribe_url"],"unknown_merge":[]}}}`))
	}))
	defer srv.Close()

	tpl, an, err := newTestClient(t, srv.URL).SaveMarketingTemplate(context.Background(), "t-1", MarketingTemplateRequest{
		Name: "Email: hi", Channel: "email", BodyHTML: "<p>x</p>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tpl.UUID != "t-1" || an == nil || !an.OK || len(an.MergeFields) != 2 {
		t.Errorf("template = %+v analysis = %+v", tpl, an)
	}
	if _, has := got["status"]; has {
		t.Errorf("an empty status must be omitted so the studio keeps the current one: %v", got)
	}
}

func TestFindMarketingTemplateByName_ExactMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/v2/opsapi-marketing/templates" || q.Get("search") == "" || q.Get("channel") != "email" {
			t.Errorf("request = %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[
			{"uuid":"near","name":"Email: spring-sale-2"},{"uuid":"exact","name":"Email: Spring-Sale"}],"meta":{"total":2}}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL)
	tpl, err := c.FindMarketingTemplateByName(context.Background(), "Email: spring-sale")
	if err != nil || tpl == nil || tpl.UUID != "exact" {
		t.Fatalf("template = %+v err = %v", tpl, err)
	}
	none, err := c.FindMarketingTemplateByName(context.Background(), "Email: spring")
	if err != nil || none != nil {
		t.Fatalf("none = %+v err = %v", none, err)
	}
}

func TestMarketingTemplate_ForbiddenIsMissingScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"API key is not scoped for this endpoint"}`))
	}))
	defer srv.Close()
	_, err := newTestClient(t, srv.URL).FindMarketingTemplateByName(context.Background(), "x")
	if !errors.Is(err, ErrMarketingScope) || !strings.Contains(err.Error(), "missing opsapi_marketing_templates scope") {
		t.Fatalf("err = %v", err)
	}
}

func TestMarketingTemplate_PluginDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"error":"Marketing is turned off for this workspace","code":"PLUGIN_DISABLED"}`))
	}))
	defer srv.Close()
	_, err := newTestClient(t, srv.URL).CreateMarketingTemplate(context.Background(), MarketingTemplateRequest{Name: "x"})
	if !errors.Is(err, ErrMarketingDisabled) {
		t.Fatalf("err = %v", err)
	}
}
