package service

import (
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
)

func TestOpsapiDestination(t *testing.T) {
	ok := model.Integration{
		Name:        "Workstation Website",
		Provider:    model.ProviderOpsAPI,
		BaseURL:     "https://opsapi.workstation.co.uk/",
		Credentials: map[string]any{"api_key": strings.Join([]string{"opsk", "test"}, "_")},
		Config:      map[string]any{"namespace_slug": "workstation"},
	}
	dest, err := opsapiDestination(ok)
	if err != nil {
		t.Fatalf("valid integration: %v", err)
	}
	if dest.Namespace() != "workstation" {
		t.Errorf("namespace = %q", dest.Namespace())
	}

	broken := map[string]func(*model.Integration){
		"provider":  func(i *model.Integration) { i.Provider = model.ProviderJira },
		"inactive":  func(i *model.Integration) { i.Status = "disabled" },
		"base_url":  func(i *model.Integration) { i.BaseURL = "ftp://x" },
		"api_key":   func(i *model.Integration) { i.Credentials = map[string]any{} },
		"namespace": func(i *model.Integration) { i.Config = map[string]any{} },
	}
	for name, mut := range broken {
		in := ok
		mut(&in)
		if _, err := opsapiDestination(in); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// Runs pick a website by integration name, case-insensitively; the namespace
// comes from that integration, so two websites never share a key.
func TestFindDestination(t *testing.T) {
	key := func(s string) map[string]any {
		return map[string]any{"api_key": strings.Join([]string{"opsk", s}, "_")}
	}
	list := []model.Integration{
		{Name: "Workstation Website", Provider: model.ProviderOpsAPI, BaseURL: "https://opsapi.workstation.co.uk",
			Credentials: key("ws"), Config: map[string]any{"namespace_slug": "workstation"}},
		{Name: "JobShout", Provider: model.ProviderOpsAPI, BaseURL: "https://opsapi.workstation.co.uk",
			Credentials: key("js"), Config: map[string]any{"namespace_slug": "jobshout"}},
		{Name: "Half done", Provider: model.ProviderOpsAPI, BaseURL: "https://opsapi.workstation.co.uk",
			Config: map[string]any{"namespace_slug": "x"}},
	}
	for name, ns := range map[string]string{"jobshout": "jobshout", " Workstation Website ": "workstation"} {
		d, err := findDestination(list, name)
		if err != nil || d.Namespace() != ns {
			t.Errorf("%q: namespace %v, err %v; want %s", name, d, err, ns)
		}
	}
	for _, bad := range []string{"Nowhere", "Half done"} {
		if _, err := findDestination(list, bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}
