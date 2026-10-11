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
