package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/config"
	"github.com/windfall/uwu_service/internal/handler/http"
	"github.com/windfall/uwu_service/internal/logger"
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
	// Initialize clients
	log.Info().Str("gemini_sa_path", cfg.GeminiServiceAccountPath).Str("project_id", cfg.GCPProjectID).Msg("Checking Gemini config")
	var geminiClient *client.GeminiClient

	if cfg.GCPProjectID != "" && cfg.GCPLocation != "" {
		// Try initializing with Service Account first
		if cfg.GeminiServiceAccountPath != "" {
			log.Info().Str("gemini_sa_path", cfg.GeminiServiceAccountPath).Msg("Initializing Gemini with Service Account")
			var err error
			geminiClient, err = client.NewGeminiClientWithServiceAccount(ctx, cfg.GCPProjectID, cfg.GCPLocation, cfg.GeminiServiceAccountPath)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to initialize Gemini with Service Account, falling back to API Key")
			} else {
				log.Info().Msg("Gemini client initialized with Service Account")
			}
		}

		// Fallback to API Key (using Vertex AI) if SA failed or was not provided
		if geminiClient == nil && cfg.GeminiAPIKey != "" {
			log.Info().Msg("Initializing Gemini with API Key")
			var err error
			geminiClient, err = client.NewGeminiClient(ctx, cfg.GCPProjectID, cfg.GCPLocation, cfg.GeminiAPIKey)
			if err != nil {
				log.Error().Err(err).Msg("Failed to initialize Gemini client with API Key")
			} else {
				log.Info().Msg("Gemini client initialized with API Key")
			}
		}
	} else {
		log.Warn().Msg("GCP Project ID or Location is missing, cannot initialize Vertex AI")
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

	// Initialize services
	aiService := service.NewAIService(geminiClient)
	speechService := service.NewSpeechService(azureSpeechClient)
	speakingService := service.NewSpeakingService(azureSpeechClient, geminiClient, redisClient, log)

	// Initialize handlers
	healthHandler := http.NewHealthHandler()
	apiHandler := http.NewAPIHandler(log, aiService, speechService)
	// Initialize Speaking handler
	speakingHandler := http.NewSpeakingHandler(log, speakingService)

	// Initialize HTTP server
	httpServer := server.NewHTTPServer(cfg, log, healthHandler, apiHandler, speakingHandler)

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

	log.Info().Msg("Server stopped")
}
