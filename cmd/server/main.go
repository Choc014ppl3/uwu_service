package main

import (
	"context"
	"os"

	"github.com/windfall/uwu_service/internal/domain/auth"

	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/internal/infra/config"
	"github.com/windfall/uwu_service/internal/infra/server"
	"github.com/windfall/uwu_service/pkg/logger"
)

func main() {
	// -----------------------------------------
	// 1. Setup Infrastructure
	// -----------------------------------------

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	// Setup logger
	logger := logger.New(cfg.Environment)

	// Database connection
	db, err := client.NewPostgresClient(context.Background(), cfg.DatabaseURL())
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// -----------------------------------------
	// 2. Setup Application
	// -----------------------------------------

	// Register Auth Domain
	jwtRepo := auth.NewJWTRepository(cfg.JWTSecret)
	userRepo := auth.NewUserRepository(db, logger)
	authService := auth.NewAuthService(userRepo, jwtRepo)
	authHandler := auth.NewAuthHandler(authService, logger)

	// -----------------------------------------
	// 3. Setup & Start HTTP Server
	// -----------------------------------------

	// Setup HTTP server
	httpServer := server.NewHTTPServer(cfg, logger, jwtRepo, authHandler)

	// Start HTTP server
	if err := httpServer.Start(); err != nil {
		logger.Error("HTTP server failed", "error", err)
		os.Exit(1)
	}
}
