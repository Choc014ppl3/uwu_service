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
	OpenAIAPIKey        string `envconfig:"OPENAI_API_KEY"`
	GeminiAPIKey        string `envconfig:"GEMINI_API_KEY"`
	GeminiFlashLitePath string `envconfig:"GEMINI_FLASH_LITE_PATH"`

	// Azure AI Speech
	AzureAISpeechKey   string `envconfig:"AZURE_AI_SPEECH_KEY"`
	AzureServiceRegion string `envconfig:"AZURE_SERVICE_REGION"`

	// Google Cloud
	GCPProjectID         string `envconfig:"GCP_PROJECT_ID"`
	GCSBucketName        string `envconfig:"GCS_BUCKET_NAME"`
	PubSubTopicID        string `envconfig:"PUBSUB_TOPIC_ID"`
	PubSubSubscriptionID string `envconfig:"PUBSUB_SUBSCRIPTION_ID"`

	// Database
	DatabaseURL string `envconfig:"DATABASE_URL"`

	// Redis
	RedisURL string `envconfig:"REDIS_URL"`

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
