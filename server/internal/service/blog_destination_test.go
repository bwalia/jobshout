package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

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
		// Left sealed by a server without its SECRETS_ENCRYPTION_KEY.
		"sealed key": func(i *model.Integration) { i.Credentials = map[string]any{"api_key": "enc:v1:AAAA"} },
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

type listIntegrations []model.Integration

func (l listIntegrations) ListByOrg(context.Context, uuid.UUID) ([]model.Integration, error) {
	return l, nil
}

// The add-website form asks for a key only for a namespace the org has not
// saved; the answer never carries the stored key.
func TestCheckWebsite(t *testing.T) {
	key := strings.Join([]string{"opsk", "stored", "abcdef"}, "_")
	svc := &blogService{integrations: listIntegrations{
		{ID: uuid.New(), Name: "JobShout", Provider: model.ProviderOpsAPI, BaseURL: "https://opsapi.workstation.co.uk/",
			Credentials: map[string]any{"api_key": key}, Config: map[string]any{"namespace_slug": "jobshout"}},
		{ID: uuid.New(), Name: "No key", Provider: model.ProviderOpsAPI, BaseURL: "https://opsapi.workstation.co.uk",
			Config: map[string]any{"namespace_slug": "halfdone"}},
	}}
	ctx, org := context.Background(), uuid.New()
	base := "https://OPSAPI.workstation.co.uk"

	got, err := svc.CheckWebsite(ctx, org, " JobShout ", base)
	if err != nil || !got.Exists || !got.Ready || got.Name != "JobShout" || got.IntegrationID == nil {
		t.Fatalf("saved namespace = %+v, %v; want exists and ready", got, err)
	}
	if got.Namespace != "jobshout" {
		t.Errorf("namespace not normalised: %q", got.Namespace)
	}

	if got, _ := svc.CheckWebsite(ctx, org, "halfdone", base); !got.Exists || got.Ready {
		t.Errorf("saved without a key = %+v; want exists, not ready", got)
	}
	if got, _ := svc.CheckWebsite(ctx, org, "workstation", base); got.Exists {
		t.Errorf("new namespace = %+v; want not exists", got)
	}
	// Same namespace on another OpsAPI is another website.
	if got, _ := svc.CheckWebsite(ctx, org, "jobshout", "https://opsapi.example.com"); got.Exists {
		t.Errorf("other OpsAPI = %+v; want not exists", got)
	}

	for _, bad := range [][2]string{{"Job Shout", base}, {"", base}, {"-x", base}, {"jobshout", ""}, {"jobshout", "ftp://x"}} {
		if _, err := svc.CheckWebsite(ctx, org, bad[0], bad[1]); !errors.Is(err, ErrInvalidIntegration) {
			t.Errorf("%q on %q: err %v, want ErrInvalidIntegration", bad[0], bad[1], err)
		}
	}
}
