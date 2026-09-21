package linuxpatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/secretsrot"
)

// VaultSSHOptions pulls an SSH private key (and optional user/password) from Vault KV v2.
type VaultSSHOptions struct {
	Mount    string
	Path     string
	KeyField string // default ssh_private_key
	UserField string
	PassField string
}

// ApplyVaultSSH loads credentials from WSLVault / HashiCorp Vault into cfg.
// Secret values are never returned to callers — only applied onto Config.
func ApplyVaultSSH(ctx context.Context, vaultCfg secretsrot.Config, cfg *Config, opt VaultSSHOptions) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(opt.Path) == "" {
		return "", nil // no vault path — env auth only
	}
	mount := opt.Mount
	if mount == "" {
		mount = "secret"
	}
	keyField := opt.KeyField
	if keyField == "" {
		keyField = "ssh_private_key"
	}
	client := secretsrot.NewClient(vaultCfg)
	if !client.Enabled() {
		return "", fmt.Errorf("vault not configured — set SECRETS_VAULT_TOKEN to fetch SSH keys from %s/%s", mount, opt.Path)
	}
	data, meta, err := client.ReadKV2(ctx, mount, opt.Path, 0)
	if err != nil {
		return "", fmt.Errorf("vault read %s/%s: %w", mount, opt.Path, err)
	}
	note := fmt.Sprintf("loaded SSH material from vault %s/%s v%d (fields not logged)", mount, opt.Path, meta.Version)
	if pem, ok := data[keyField].(string); ok && strings.TrimSpace(pem) != "" {
		cfg.SSHKeyPEM = pem
	} else {
		// try common aliases
		for _, alt := range []string{"private_key", "ssh_key", "pem"} {
			if pem, ok := data[alt].(string); ok && strings.TrimSpace(pem) != "" {
				cfg.SSHKeyPEM = pem
				keyField = alt
				break
			}
		}
	}
	if cfg.SSHKeyPEM == "" && cfg.SSHKeyPath == "" && cfg.SSHPassword == "" {
		return note, fmt.Errorf("vault path %s/%s has no %s (or password) field", mount, opt.Path, keyField)
	}
	userField := opt.UserField
	if userField == "" {
		userField = "ssh_user"
	}
	if u, ok := data[userField].(string); ok && strings.TrimSpace(u) != "" {
		cfg.SSHUser = strings.TrimSpace(u)
	}
	passField := opt.PassField
	if passField == "" {
		passField = "ssh_password"
	}
	if p, ok := data[passField].(string); ok && p != "" {
		cfg.SSHPassword = p
	}
	return note, nil
}
