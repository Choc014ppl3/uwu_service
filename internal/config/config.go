package config

import (
	"fmt"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config holds all configuration for the service.
type Config struct {
	// Server
	Host     string `envconfig:"SERVER_HOST" default:"0.0.0.0"`
	HTTPPort int    `envconfig:"SERVER_HTTP_PORT" default:"8080"`

	Environment string `envconfig:"SERVER_ENV" default:"development"`

	// Timeouts
	ReadTimeout     time.Duration `envconfig:"SERVER_READ_TIMEOUT" default:"15s"`
	WriteTimeout    time.Duration `envconfig:"SERVER_WRITE_TIMEOUT" default:"15s"`
	IdleTimeout     time.Duration `envconfig:"SERVER_IDLE_TIMEOUT" default:"60s"`
	ShutdownTimeout time.Duration `envconfig:"SERVER_SHUTDOWN_TIMEOUT" default:"30s"`

	// Logging
	LogLevel  string `envconfig:"LOG_LEVEL" default:"info"`
	LogFormat string `envconfig:"LOG_FORMAT" default:"json"`

	// AI Services
	GeminiSAPath string `envconfig:"GEMINI_SA_PATH"`
	GCPLocation  string `envconfig:"GCP_LOCATION" default:"asia-southeast1"`

	// Azure AI Speech
	AzureAISpeechKey   string `envconfig:"AZURE_AI_SPEECH_KEY"`
	AzureServiceRegion string `envconfig:"AZURE_SERVICE_REGION"`

	// Redis
	RedisURL string `envconfig:"REDIS_URL"`

	// Database
	DatabaseURL string `envconfig:"DATABASE_URL"`

	// Cloudflare R2
	CloudflareAccessKeyID string `envconfig:"CLOUDFLARE_ACCESS_KEY_ID"`
	CloudflareSecretKey   string `envconfig:"CLOUDFLARE_SECRET_ACCESS_KEY"`
	CloudflareR2Endpoint  string `envconfig:"CLOUDFLARE_R2_ENDPOINT"`
	CloudflarePublicURL   string `envconfig:"CLOUDFLARE_PUBLIC_URL"`
	CloudflareBucketName  string `envconfig:"CLOUDFLARE_BUCKET_NAME"`

	// CORS
	CORSAllowedOrigins []string `envconfig:"CORS_ALLOWED_ORIGINS" default:"*"`
	CORSAllowedMethods []string `envconfig:"CORS_ALLOWED_METHODS" default:"GET,POST,PUT,DELETE,OPTIONS"`
	CORSAllowedHeaders []string `envconfig:"CORS_ALLOWED_HEADERS" default:"Accept,Authorization,Content-Type,X-Request-ID"`
}

// Load loads configuration from environment variables.
func Load() (*Config, error) {
	// Load .env file if it exists (ignore error if not found)
	_ = godotenv.Load()

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to process env config: %w", err)
	}
	return &cfg, nil
}

// HTTPAddress returns the HTTP server address.
func (c *Config) HTTPAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.HTTPPort)
}

// IsDevelopment returns true if running in development mode.
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

// IsProduction returns true if running in production mode.
func (c *Config) IsProduction() bool {
	return c.Environment == "production"
}
