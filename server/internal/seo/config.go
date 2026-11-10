package seo

import (
	"os"
	"strings"
)

// Config is read from SEO_* env vars so platform config stays agent-agnostic.
// Token sealing mirrors Meta Lead / Mail (AES-GCM via mail.Encrypt).
type Config struct {
	TokenKey string
}

// LoadConfig reads SEO_TOKEN_KEY (optional fallbacks for rings that already seal tokens).
func LoadConfig() Config {
	key := strings.TrimSpace(os.Getenv("SEO_TOKEN_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("META_TOKEN_KEY"))
	}
	if key == "" {
		key = strings.TrimSpace(os.Getenv("GMAIL_TOKEN_KEY"))
	}
	return Config{TokenKey: key}
}

// CanSealTokens is true when GitHub PATs can be stored encrypted.
func (c Config) CanSealTokens() bool {
	return c.TokenKey != ""
}
