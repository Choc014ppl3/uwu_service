package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/config"
	"github.com/windfall/uwu_service/internal/handler/http"
	"github.com/windfall/uwu_service/internal/logger"
	"github.com/windfall/uwu_service/internal/repository"
	"github.com/windfall/uwu_service/internal/server"
	"github.com/windfall/uwu_service/internal/service"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	// Initialize logger
	log := logger.New(cfg.LogLevel, cfg.LogFormat)
	log.Info().Str("env", cfg.Environment).Msg("Starting uwu_service")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize clients
	var geminiClient *client.GeminiClient

	if cfg.GeminiSABase64 != "" {
		log.Info().Msg("Initializing Gemini with Base64 Service Account")

		// Decode base64 service account
		saJSON, err := base64.StdEncoding.DecodeString(cfg.GeminiSABase64)
		if err != nil {
			log.Error().Err(err).Msg("Failed to decode GEMINI_SA_BASE64")
		} else {
			// Extract project_id from JSON
			var sa struct {
				ProjectID string `json:"project_id"`
			}
			if err := json.Unmarshal(saJSON, &sa); err == nil && sa.ProjectID != "" {
				log.Info().Str("project_id", sa.ProjectID).Str("location", cfg.GCPLocation).Msg("Extracted Project ID from Service Account")

				geminiClient, err = client.NewGeminiClientWithCredentials(ctx, sa.ProjectID, cfg.GCPLocation, saJSON)
				if err != nil {
					log.Error().Err(err).Msg("Failed to initialize Gemini client")
				} else {
					log.Info().Msg("Gemini client initialized successfully")
				}
			} else {
				log.Error().Msg("Could not extract project_id from service account JSON")
			}
		}
	} else {
		log.Warn().Msg("GEMINI_SA_BASE64 not set, skipping Gemini initialization")
	}

	if geminiClient == nil {
		log.Warn().Msg("Gemini client not initialized (no valid credentials)")
	}

	var azureSpeechClient *client.AzureSpeechClient
	if cfg.AzureAISpeechKey != "" && cfg.AzureServiceRegion != "" {
		azureSpeechClient = client.NewAzureSpeechClient(cfg.AzureAISpeechKey, cfg.AzureServiceRegion)
	}

	// Initialize Redis client
	var redisClient *client.RedisClient
	if cfg.RedisURL != "" {
		var err error
		redisClient, err = client.NewRedisClient(cfg.RedisURL)
		if err != nil {
			log.Error().Err(err).Msg("Failed to initialize Redis client")
		} else {
			log.Info().Msg("Redis client initialized")
		}
	}

	// Initialize Cloudflare R2 Client (using S3 protocol)
	var cloudflareClient *client.CloudflareClient
	if cfg.CloudflareAccessKeyID != "" && cfg.CloudflareSecretKey != "" && cfg.CloudflareR2Endpoint != "" && cfg.CloudflareBucketName != "" {
		var err error
		// Use Access Key/Secret if valid (Standard R2)
		// Or if user provided CLOUDFLARE_API_TOKEN, we assume they might want to use it as a static credential?
		// Usually R2 requires S3 credentials. We'll use the specific AccessKey/Secret fields.
		// If they are empty, we might skip.
		// Note: The user requested "add this env CLOUDFLARE_API_TOKEN".
		// If CLOUDFLARE_API_TOKEN is used as "Access Key"? Unlikely.
		// We'll stick to standard fields I added to config: CloudflareAccessKeyID/CloudflareSecretKey.

		cloudflareClient, err = client.NewCloudflareClient(ctx,
			cfg.CloudflareAccessKeyID,
			cfg.CloudflareSecretKey,
			cfg.CloudflareR2Endpoint,
			cfg.CloudflareBucketName,
			cfg.CloudflarePublicURL,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to initialize Cloudflare client")
		} else {
			log.Info().Msg("Cloudflare R2 client initialized")
		}
	} else {
		log.Warn().Msg("Cloudflare configuration missing, skipping R2 initialization")
	}

	// Initialize Postgres Client
	var postgresClient *client.PostgresClient
	dbURL := cfg.DatabaseURL()
	if dbURL != "" {
		var err error
		postgresClient, err = client.NewPostgresClient(ctx, dbURL)
		if err != nil {
			log.Error().Err(err).Msg("Failed to initialize Postgres client")
		} else {
			log.Info().Msg("Postgres client initialized")
		}
	} else {
		log.Warn().Msg("DatabaseURL construction failed, skipping Postgres initialization")
	}

	// Initialize Repositories
	learningItemRepo := repository.NewPostgresLearningItemRepository(postgresClient)
	scenarioRepo := repository.NewPostgresScenarioRepository(postgresClient)
	userRepo := repository.NewPostgresUserRepository(postgresClient)

	// Initialize services
	aiService := service.NewAIService(geminiClient, cloudflareClient, azureSpeechClient)
	scenarioService := service.NewScenarioService(aiService, scenarioRepo)
	speechService := service.NewSpeechService(azureSpeechClient)
	speakingService := service.NewSpeakingService(azureSpeechClient, geminiClient, redisClient, log)
	learningService := service.NewLearningService(aiService, learningItemRepo)
	authService := service.NewAuthService(userRepo, cfg.JWTSecret)

	// Initialize handlers
	healthHandler := http.NewHealthHandler()
	apiHandler := http.NewAPIHandler(log, aiService, speechService, scenarioService)
	speakingHandler := http.NewSpeakingHandler(log, speakingService)
	learningItemHandler := http.NewLearningItemHandler(learningService)
	authHandler := http.NewAuthHandler(log, authService)

	// Initialize HTTP server
	httpServer := server.NewHTTPServer(cfg, log, healthHandler, apiHandler, speakingHandler, learningItemHandler, authHandler, authService)

	// Start servers
	go func() {
		if err := httpServer.Start(); err != nil {
			log.Error().Err(err).Msg("HTTP server error")
			cancel()
		}
	}()

	log.Info().
		Str("http_addr", cfg.HTTPAddress()).
		Msg("Servers started")

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		log.Info().Msg("Shutdown signal received")
	case <-ctx.Done():
		log.Info().Msg("Context cancelled")
	}

	// Graceful shutdown
	log.Info().Msg("Shutting down servers...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	// Close clients
	if geminiClient != nil {
		geminiClient.Close()
	}
	if redisClient != nil {
		redisClient.Close()
	}
	if postgresClient != nil {
		postgresClient.Close()
	}

	log.Info().Msg("Server stopped")
}
