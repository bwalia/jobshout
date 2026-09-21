package secretsrot

import (
	"os"
	"strings"
	"time"
)

const (
	DefaultWSLVaultAddr = "https://vault.workstation.co.uk"
	DefaultUILoginURL   = "https://vault-ui.workstation.co.uk/login"
	DefaultDocsURL      = "https://www.wslvault.org/"
)

// Config is loaded from the environment (never logged with token).
type Config struct {
	Addr      string
	Token     string
	Namespace string // HC Vault enterprise / optional tenant header for WSLVault
	UILogin   string
	DocsURL   string
	Timeout   time.Duration
}

// LoadConfig reads WSLVault / HashiCorp Vault settings.
func LoadConfig() Config {
	addr := firstNonEmpty(
		os.Getenv("SECRETS_VAULT_ADDR"),
		os.Getenv("WSLVAULT_ADDR"),
		os.Getenv("VAULT_ADDR"),
		DefaultWSLVaultAddr,
	)
	token := firstNonEmpty(
		os.Getenv("SECRETS_VAULT_TOKEN"),
		os.Getenv("WSLVAULT_TOKEN"),
		os.Getenv("VAULT_TOKEN"),
	)
	ns := firstNonEmpty(os.Getenv("SECRETS_VAULT_NAMESPACE"), os.Getenv("VAULT_NAMESPACE"), os.Getenv("WSLVAULT_TENANT"))
	timeout := 30 * time.Second
	return Config{
		Addr:      strings.TrimRight(strings.TrimSpace(addr), "/"),
		Token:     strings.TrimSpace(token),
		Namespace: strings.TrimSpace(ns),
		UILogin:   DefaultUILoginURL,
		DocsURL:   DefaultDocsURL,
		Timeout:   timeout,
	}
}

func (c Config) Enabled() bool {
	return c.Addr != "" && c.Token != ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
