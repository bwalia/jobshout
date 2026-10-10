package googleauth

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const chartDir = "../../../deploy/helm/jobshout"

// Google rejects https://jobshout.co.uk/api/v1/auth/google/callback
// (redirect_uri_mismatch) on the shared OAuth client; only the www callback is
// registered. Guard prod against falling back to the apex host.
func TestProdGoogleCallbackHost(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(chartDir, "values-prod.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Host        string   `yaml:"host"`
		HostAliases []string `yaml:"hostAliases"`
		GoogleAuth  struct {
			CallbackHost string `yaml:"callbackHost"`
		} `yaml:"googleAuth"`
	}
	if err := yaml.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	const want = "www.jobshout.co.uk"
	if v.GoogleAuth.CallbackHost != want {
		t.Fatalf("googleAuth.callbackHost = %q, want %q", v.GoogleAuth.CallbackHost, want)
	}
	// The callback must land on a host the chart routes to the API.
	if v.Host != want && !slices.Contains(v.HostAliases, want) {
		t.Fatalf("%s is neither host nor a hostAlias", want)
	}

	tmpl, err := os.ReadFile(filepath.Join(chartDir, "templates", "configmap.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(tmpl), "\n") {
		if strings.Contains(line, "GOOGLE_OAUTH_REDIRECT_URL:") {
			if !strings.Contains(line, "googleAuth).callbackHost") {
				t.Fatalf("GOOGLE_OAUTH_REDIRECT_URL ignores googleAuth.callbackHost: %s", strings.TrimSpace(line))
			}
			return
		}
	}
	t.Fatal("GOOGLE_OAUTH_REDIRECT_URL missing from configmap template")
}
