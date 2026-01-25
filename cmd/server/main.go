package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/config"
	"github.com/windfall/uwu_service/internal/handler/http"
	"github.com/windfall/uwu_service/internal/handler/ws"
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
	var openaiClient *client.OpenAIClient
	if cfg.OpenAIAPIKey != "" {
		openaiClient = client.NewOpenAIClient(cfg.OpenAIAPIKey)
	}

	var geminiClient *client.GeminiClient
	if cfg.GeminiAPIKey != "" {
		var err error
		geminiClient, err = client.NewGeminiClient(ctx, cfg.GeminiAPIKey)
		if err != nil {
			log.Error().Err(err).Msg("Failed to initialize Gemini client")
		}
	}

	var storageClient *client.StorageClient
	if cfg.GCPProjectID != "" && cfg.GCSBucketName != "" {
		var err error
		storageClient, err = client.NewStorageClient(ctx, cfg.GCSBucketName)
		if err != nil {
			log.Error().Err(err).Msg("Failed to initialize storage client")
		}
	}

	var pubsubClient *client.PubSubClient
	if cfg.GCPProjectID != "" && cfg.PubSubTopicID != "" {
		var err error
		pubsubClient, err = client.NewPubSubClient(ctx, cfg.GCPProjectID, cfg.PubSubTopicID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to initialize pubsub client")
		}
	}

	var azureSpeechClient *client.AzureSpeechClient
	if cfg.AzureAISpeechKey != "" && cfg.AzureServiceRegion != "" {
		azureSpeechClient = client.NewAzureSpeechClient(cfg.AzureAISpeechKey, cfg.AzureServiceRegion)
	}

	// Initialize services
	aiService := service.NewAIService(openaiClient, geminiClient)
	exampleService := service.NewExampleService(storageClient, pubsubClient)
	speechService := service.NewSpeechService(azureSpeechClient)

	// Initialize handlers
	healthHandler := http.NewHealthHandler()
	apiHandler := http.NewAPIHandler(log, aiService, exampleService, speechService)
	wsHandler := ws.NewHandler(log)

	// Initialize WebSocket hub
	wsHub := server.NewWebSocketHub(log)
	go wsHub.Run(ctx)

	// Initialize HTTP server
	httpServer := server.NewHTTPServer(cfg, log, healthHandler, apiHandler, wsHandler, wsHub)

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
	if storageClient != nil {
		storageClient.Close()
	}
	if pubsubClient != nil {
		pubsubClient.Close()
	}
	if geminiClient != nil {
		geminiClient.Close()
	}

	log.Info().Msg("Server stopped")
}
