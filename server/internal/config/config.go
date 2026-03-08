package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration values.
type Config struct {
	DatabaseURL         string        `mapstructure:"DATABASE_URL"`
	ServerPort          string        `mapstructure:"SERVER_PORT"`
	JWTSecret           string        `mapstructure:"JWT_SECRET"`
	JWTExpiryMinutes    int           `mapstructure:"JWT_EXPIRY_MINUTES"`
	JWTRefreshExpiryDays int          `mapstructure:"JWT_REFRESH_EXPIRY_DAYS"`
	CORSOrigins         []string
	MinIOEndpoint       string        `mapstructure:"MINIO_ENDPOINT"`
	MinIOAccessKey      string        `mapstructure:"MINIO_ACCESS_KEY"`
	MinIOSecretKey      string        `mapstructure:"MINIO_SECRET_KEY"`
	MinIOUseSSL         bool          `mapstructure:"MINIO_USE_SSL"`
	MinIOBucketAvatars  string        `mapstructure:"MINIO_BUCKET_AVATARS"`
	MinIOBucketKnowledge string       `mapstructure:"MINIO_BUCKET_KNOWLEDGE"`
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
	viper.SetDefault("JWT_EXPIRY_MINUTES", 15)
	viper.SetDefault("JWT_REFRESH_EXPIRY_DAYS", 7)
	viper.SetDefault("MINIO_USE_SSL", false)
	viper.SetDefault("MINIO_BUCKET_AVATARS", "avatars")
	viper.SetDefault("MINIO_BUCKET_KNOWLEDGE", "knowledge")
	viper.SetDefault("CORS_ORIGINS", "http://localhost:3000")

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
