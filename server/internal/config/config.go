package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration values.
type Config struct {
	DatabaseURL          string `mapstructure:"DATABASE_URL"`
	ServerPort           string `mapstructure:"SERVER_PORT"`
	JWTSecret            string `mapstructure:"JWT_SECRET"`
	JWTExpiryMinutes     int    `mapstructure:"JWT_EXPIRY_MINUTES"`
	JWTRefreshExpiryDays int    `mapstructure:"JWT_REFRESH_EXPIRY_DAYS"`
	CORSOrigins          []string

	// How long to keep retrying the initial database connection at startup
	// before giving up and exiting. 0 fails fast on the first error.
	DatabaseConnectTimeout time.Duration `mapstructure:"DATABASE_CONNECT_TIMEOUT"`

	// AutoModelSelection enables per-task provider/model selection for agents
	// whose ModelProvider is "auto". Agents pinned to a provider are unaffected
	// either way; turning this off just makes "auto" fall back to the default.
	AutoModelSelection bool `mapstructure:"AUTO_MODEL_SELECTION"`

	// MinIO / S3-compatible object storage (optional).
	MinIOEndpoint        string `mapstructure:"MINIO_ENDPOINT"`
	MinIOAccessKey       string `mapstructure:"MINIO_ACCESS_KEY"`
	MinIOSecretKey       string `mapstructure:"MINIO_SECRET_KEY"`
	MinIOUseSSL          bool   `mapstructure:"MINIO_USE_SSL"`
	MinIOBucketAvatars   string `mapstructure:"MINIO_BUCKET_AVATARS"`
	MinIOBucketKnowledge string `mapstructure:"MINIO_BUCKET_KNOWLEDGE"`

	// LLM provider selection. Defaults to "ollama".
	LLMProvider string `mapstructure:"LLM_PROVIDER"`

	// Ollama configuration (used when LLM_PROVIDER=ollama or as fallback).
	OllamaBaseURL      string `mapstructure:"OLLAMA_BASE_URL"`
	OllamaDefaultModel string `mapstructure:"OLLAMA_DEFAULT_MODEL"`
	// OllamaJWTSecret is the shared secret for the auth gateway that fronts
	// Ollama. Empty means there is no gateway (a direct/local Ollama), and
	// requests go out unsigned. There is deliberately no default: a signing
	// secret does not belong in committed source. Set it via the environment
	// or .env, which is gitignored.
	OllamaJWTSecret string `mapstructure:"OLLAMA_JWT_SECRET"`
	// OllamaTimeout bounds a single Ollama request. Large models that are not
	// resident must be loaded before the first token, so this needs headroom.
	OllamaTimeout time.Duration `mapstructure:"OLLAMA_TIMEOUT"`

	// OpenAI (or OpenAI-compatible) configuration.
	// When LLM_PROVIDER=openai, OPENAI_API_KEY must be set.
	OpenAIAPIKey       string `mapstructure:"OPENAI_API_KEY"`
	OpenAIBaseURL      string `mapstructure:"OPENAI_BASE_URL"`
	OpenAIDefaultModel string `mapstructure:"OPENAI_DEFAULT_MODEL"`

	// Claude / Anthropic configuration.
	// When LLM_PROVIDER=claude, CLAUDE_API_KEY must be set.
	ClaudeAPIKey       string `mapstructure:"CLAUDE_API_KEY"`
	ClaudeBaseURL      string `mapstructure:"CLAUDE_BASE_URL"`
	ClaudeDefaultModel string `mapstructure:"CLAUDE_DEFAULT_MODEL"`

	// Embedding configuration (used for RAG / knowledge retrieval).
	// The default provider is OpenAI with text-embedding-3-small (1536 dims).
	// When EMBEDDING_PROVIDER=openai, OPENAI_API_KEY must be set for embeddings
	// to work; otherwise ingestion degrades gracefully (best-effort, logged).
	EmbeddingProvider   string `mapstructure:"EMBEDDING_PROVIDER"`
	EmbeddingModel      string `mapstructure:"EMBEDDING_MODEL"`
	EmbeddingDimensions int    `mapstructure:"EMBEDDING_DIMENSIONS"`

	// Python sidecar (LangChain/LangGraph execution).
	PythonSidecarURL    string `mapstructure:"PYTHON_SIDECAR_URL"`
	PythonSidecarSecret string `mapstructure:"PYTHON_SIDECAR_SECRET"`

	// Telegram Bot integration (optional).
	TelegramBotToken    string `mapstructure:"TELEGRAM_BOT_TOKEN"`
	TelegramWebhookURL  string `mapstructure:"TELEGRAM_WEBHOOK_URL"`
	TelegramSecretToken string `mapstructure:"TELEGRAM_WEBHOOK_SECRET"`
	TelegramRatePerMin  int    `mapstructure:"TELEGRAM_RATE_PER_MIN"`

	// Frontend base URL for generating links in Telegram messages.
	FrontendBaseURL string `mapstructure:"FRONTEND_BASE_URL"`

	// opsapi CMS — where generated articles are filed as drafts. All three of
	// URL, token and namespace are needed before publishing is offered at all;
	// generation works without any of them.
	//
	// OpsAPIToken is a seed JWT, not a permanent credential: opsapi's login
	// requires an emailed OTP, so a token is obtained once by hand and the
	// server keeps it alive through /auth/refresh.
	OpsAPIBaseURL   string        `mapstructure:"OPSAPI_BASE_URL"`
	OpsAPIToken     string        `mapstructure:"OPSAPI_TOKEN"`
	OpsAPINamespace string        `mapstructure:"OPSAPI_NAMESPACE"`
	OpsAPITimeout   time.Duration `mapstructure:"OPSAPI_TIMEOUT"`

	// Blog generator — the directory generated markdown is filed under, which
	// is a label in the UI rather than a path on disk.
	BlogContentDir string `mapstructure:"BLOG_CONTENT_DIR"`
	BlogAuthorName string `mapstructure:"BLOG_AUTHOR_NAME"`

	// GitHubToken is optional. The research agent reads GitHub through its
	// public API, which allows 60 requests an hour unauthenticated — enough to
	// try, not enough for a busy schedule. A token raises the ceiling to 5000
	// and needs no scopes for public repositories.
	GitHubToken string `mapstructure:"GITHUB_TOKEN"`
}

// AccessTokenExpiry returns the access token expiry duration.
func (c *Config) AccessTokenExpiry() time.Duration {
	return time.Duration(c.JWTExpiryMinutes) * time.Minute
}

// RefreshTokenExpiry returns the refresh token expiry duration.
func (c *Config) RefreshTokenExpiry() time.Duration {
	return time.Duration(c.JWTRefreshExpiryDays) * 24 * time.Hour
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	viper.AutomaticEnv()

	viper.SetDefault("SERVER_PORT", ":8080")
	viper.SetDefault("DATABASE_CONNECT_TIMEOUT", "5m")
	viper.SetDefault("AUTO_MODEL_SELECTION", true)
	viper.SetDefault("JWT_EXPIRY_MINUTES", 15)
	viper.SetDefault("JWT_REFRESH_EXPIRY_DAYS", 7)
	viper.SetDefault("MINIO_USE_SSL", false)
	viper.SetDefault("MINIO_BUCKET_AVATARS", "avatars")
	viper.SetDefault("MINIO_BUCKET_KNOWLEDGE", "knowledge")
	viper.SetDefault("CORS_ORIGINS", "http://localhost:3001")

	// LLM defaults — Ollama running locally is the out-of-the-box provider.
	viper.SetDefault("LLM_PROVIDER", "ollama")
	viper.SetDefault("OLLAMA_BASE_URL", "http://localhost:11434")
	viper.SetDefault("OLLAMA_DEFAULT_MODEL", "llama3")
	// No OLLAMA_JWT_SECRET default on purpose — see the field comment.
	viper.SetDefault("OLLAMA_TIMEOUT", "3m")
	viper.SetDefault("OPENAI_BASE_URL", "https://api.openai.com")
	viper.SetDefault("OPENAI_DEFAULT_MODEL", "gpt-4o-mini")
	viper.SetDefault("CLAUDE_BASE_URL", "https://api.anthropic.com")
	viper.SetDefault("CLAUDE_DEFAULT_MODEL", "claude-sonnet-4-20250514")

	// Embedding defaults — OpenAI text-embedding-3-small (1536 dims).
	viper.SetDefault("EMBEDDING_PROVIDER", "openai")
	viper.SetDefault("EMBEDDING_MODEL", "text-embedding-3-small")
	viper.SetDefault("EMBEDDING_DIMENSIONS", 1536)
	viper.SetDefault("PYTHON_SIDECAR_URL", "http://localhost:8001")
	viper.SetDefault("PYTHON_SIDECAR_SECRET", "change-me-sidecar-secret")

	// Telegram defaults.
	viper.SetDefault("TELEGRAM_RATE_PER_MIN", 20)
	viper.SetDefault("FRONTEND_BASE_URL", "http://localhost:3001")

	// Blog + CMS defaults. The opsapi credentials are intentionally empty:
	// publishing stays disabled until an operator supplies all three, and a
	// default URL would only produce confusing failures against the wrong host.
	viper.SetDefault("OPSAPI_TIMEOUT", "30s")
	viper.SetDefault("BLOG_CONTENT_DIR", "content/blogs")
	viper.SetDefault("BLOG_AUTHOR_NAME", "JobShout Article Writer")

	cfg := &Config{
		DatabaseURL:          viper.GetString("DATABASE_URL"),
		ServerPort:           viper.GetString("SERVER_PORT"),
		JWTSecret:            viper.GetString("JWT_SECRET"),
		JWTExpiryMinutes:     viper.GetInt("JWT_EXPIRY_MINUTES"),
		JWTRefreshExpiryDays: viper.GetInt("JWT_REFRESH_EXPIRY_DAYS"),
		MinIOEndpoint:        viper.GetString("MINIO_ENDPOINT"),
		MinIOAccessKey:       viper.GetString("MINIO_ACCESS_KEY"),
		MinIOSecretKey:       viper.GetString("MINIO_SECRET_KEY"),
		MinIOUseSSL:          viper.GetBool("MINIO_USE_SSL"),
		MinIOBucketAvatars:   viper.GetString("MINIO_BUCKET_AVATARS"),
		MinIOBucketKnowledge: viper.GetString("MINIO_BUCKET_KNOWLEDGE"),
		LLMProvider:          viper.GetString("LLM_PROVIDER"),
		OllamaBaseURL:        viper.GetString("OLLAMA_BASE_URL"),
		OllamaDefaultModel:   viper.GetString("OLLAMA_DEFAULT_MODEL"),
		OllamaJWTSecret:      viper.GetString("OLLAMA_JWT_SECRET"),
		OllamaTimeout:        viper.GetDuration("OLLAMA_TIMEOUT"),
		OpenAIAPIKey:         viper.GetString("OPENAI_API_KEY"),
		OpenAIBaseURL:        viper.GetString("OPENAI_BASE_URL"),
		OpenAIDefaultModel:   viper.GetString("OPENAI_DEFAULT_MODEL"),
		ClaudeAPIKey:         viper.GetString("CLAUDE_API_KEY"),
		ClaudeBaseURL:        viper.GetString("CLAUDE_BASE_URL"),
		ClaudeDefaultModel:   viper.GetString("CLAUDE_DEFAULT_MODEL"),
		EmbeddingProvider:    viper.GetString("EMBEDDING_PROVIDER"),
		EmbeddingModel:       viper.GetString("EMBEDDING_MODEL"),
		EmbeddingDimensions:  viper.GetInt("EMBEDDING_DIMENSIONS"),
		PythonSidecarURL:     viper.GetString("PYTHON_SIDECAR_URL"),
		PythonSidecarSecret:  viper.GetString("PYTHON_SIDECAR_SECRET"),
		TelegramBotToken:     viper.GetString("TELEGRAM_BOT_TOKEN"),
		TelegramWebhookURL:   viper.GetString("TELEGRAM_WEBHOOK_URL"),
		TelegramSecretToken:  viper.GetString("TELEGRAM_WEBHOOK_SECRET"),
		TelegramRatePerMin:   viper.GetInt("TELEGRAM_RATE_PER_MIN"),
		FrontendBaseURL:      viper.GetString("FRONTEND_BASE_URL"),
		OpsAPIBaseURL:        viper.GetString("OPSAPI_BASE_URL"),
		OpsAPIToken:          viper.GetString("OPSAPI_TOKEN"),
		OpsAPINamespace:      viper.GetString("OPSAPI_NAMESPACE"),
		OpsAPITimeout:        viper.GetDuration("OPSAPI_TIMEOUT"),
		BlogContentDir:       viper.GetString("BLOG_CONTENT_DIR"),
		BlogAuthorName:       viper.GetString("BLOG_AUTHOR_NAME"),
		GitHubToken:          viper.GetString("GITHUB_TOKEN"),

		DatabaseConnectTimeout: viper.GetDuration("DATABASE_CONNECT_TIMEOUT"),
		AutoModelSelection:     viper.GetBool("AUTO_MODEL_SELECTION"),
	}

	origins := viper.GetString("CORS_ORIGINS")
	cfg.CORSOrigins = strings.Split(origins, ",")
	for i, o := range cfg.CORSOrigins {
		cfg.CORSOrigins[i] = strings.TrimSpace(o)
	}

	if cfg.DatabaseURL == "" {
		return nil, ErrMissingDatabaseURL
	}
	if cfg.JWTSecret == "" {
		return nil, ErrMissingJWTSecret
	}

	return cfg, nil
}

// Sentinel errors for missing required configuration.
var (
	ErrMissingDatabaseURL = configError("DATABASE_URL is required")
	ErrMissingJWTSecret   = configError("JWT_SECRET is required")
)

type configError string

func (e configError) Error() string {
	return string(e)
}
