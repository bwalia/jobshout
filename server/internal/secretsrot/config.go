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

	// DefaultKubeconfigDir holds one kubeconfig per named cluster
	// (<dir>/<cluster> or <dir>/<cluster>.yaml).
	DefaultKubeconfigDir = "/etc/secretsrot/kube"
	DefaultGitHubAPI     = "https://api.github.com"
	DefaultRPURL         = "https://rp.workstation.co.uk"
)

// Config is loaded from the environment (never logged with token).
type Config struct {
	Addr      string
	Token     string
	Namespace string // HC Vault enterprise / optional tenant header for WSLVault
	UILogin   string
	DocsURL   string
	Timeout   time.Duration

	// Propagation credentials. All are server-side and named: a launch only
	// ever carries the cluster / repo / ring NAME, never a credential, because
	// launch values are persisted on the task.
	KubeconfigDir  string // SECRETS_ROT_KUBECONFIG_DIR
	GitHubToken    string // SECRETS_ROT_GITHUB_TOKEN (dedicated; not the research token)
	GitHubAPI      string // SECRETS_ROT_GITHUB_API (tests / GHES); default api.github.com
	RPURL          string // RP_URL
	RPToken        string // RP_API_TOKEN
	RPProdPassword string // RP_PROD_PASSWORD (only sent for ring=prod)

	// Propagation timing; zero means the defaults. Tests shrink these.
	ESOTimeout      time.Duration // ExternalSecret force-sync wait (default 2m)
	ESOPollInterval time.Duration // default 2s
	RPTimeout       time.Duration // per-ring restart wait (default 20m)
	RPPollInterval  time.Duration // default 5s
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

		KubeconfigDir:  firstNonEmpty(os.Getenv("SECRETS_ROT_KUBECONFIG_DIR"), DefaultKubeconfigDir),
		GitHubToken:    strings.TrimSpace(os.Getenv("SECRETS_ROT_GITHUB_TOKEN")),
		GitHubAPI:      strings.TrimRight(firstNonEmpty(os.Getenv("SECRETS_ROT_GITHUB_API"), DefaultGitHubAPI), "/"),
		RPURL:          strings.TrimRight(firstNonEmpty(os.Getenv("RP_URL"), DefaultRPURL), "/"),
		RPToken:        strings.TrimSpace(os.Getenv("RP_API_TOKEN")),
		RPProdPassword: strings.TrimSpace(os.Getenv("RP_PROD_PASSWORD")),
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
