package repository

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/secretbox"
)

func TestIntegrationCredsSealRoundTrip(t *testing.T) {
	box, _ := secretbox.New("test passphrase")
	r := &integrationRepository{box: box}
	secret := strings.Join([]string{"opsk", "live", "abc123"}, "_")

	raw, err := r.sealCreds(map[string]any{"api_key": secret, "port": 443.0})
	if err != nil || strings.Contains(string(raw), secret) {
		t.Fatalf("sealed creds leak plaintext: %s %v", raw, err)
	}
	var creds map[string]any
	_ = json.Unmarshal(raw, &creds)
	r.openCreds(creds)
	if creds["api_key"] != secret || creds["port"] != 443.0 {
		t.Fatalf("opened = %v", creds)
	}

	// Legacy plaintext rows read as is.
	legacy := map[string]any{"api_key": "plain"}
	r.openCreds(legacy)
	if legacy["api_key"] != "plain" {
		t.Fatalf("legacy = %v", legacy)
	}

	// Without the key a sealed value stays sealed, so a rewrite can't erase it.
	sealed := map[string]any{}
	_ = json.Unmarshal(raw, &sealed)
	(&integrationRepository{}).openCreds(sealed)
	stored, _ := sealed["api_key"].(string)
	if !secretbox.IsSealed(stored) {
		t.Fatalf("no-key open = %v", sealed)
	}
	again, _ := (&integrationRepository{}).sealCreds(sealed)
	if !strings.Contains(string(again), stored) {
		t.Fatal("no-key rewrite changed the sealed value")
	}
}
