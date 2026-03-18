package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/windfall/uwu_service/internal/domain/auth"
	"github.com/windfall/uwu_service/internal/domain/dialog"
	"github.com/windfall/uwu_service/internal/domain/profile"
	"github.com/windfall/uwu_service/internal/domain/video"

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

	// Initialize Logger & Queue
	logger := logger.NewLogger(cfg.Environment)
	queue := client.NewQueueClient(logger, cfg.QueueBufferSize)

	// Initialize Database Connection
	db, err := client.NewPostgresClient(context.Background(), cfg.DatabaseURL())
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Initialize Azure AI Client
	chatGPTClient := client.NewAzureChatGPTClient(cfg.AzureGPT5NanoEndpoint, cfg.AzureGPT5NanoKey)
	whisperClient := client.NewAzureWhisperClient(cfg.AzureWhisperEndpoint, cfg.AzureWhisperKey)
	speechClient := client.NewAzureSpeechClient(cfg.AzureAISpeechKey, cfg.AzureServiceRegion)
	imageClient := client.NewAzureImageClient(cfg.AzureImageMiniEndpoint, cfg.AzureImageMiniKey)

	// Initialize Redis Client
	redisClient, err := client.NewRedisClient(cfg.RedisURL)
	if err != nil {
		logger.Error("Failed to initialize Redis client", "error", err)
		os.Exit(1)
	}

	// Initialize Cloudflare R2 Client (using S3 protocol)
	cloudflareClient, err := client.NewCloudflareClient(context.Background(),
		cfg.CloudflareAccessKeyID,
		cfg.CloudflareSecretKey,
		cfg.CloudflareR2Endpoint,
		cfg.CloudflareBucketName,
		cfg.CloudflarePublicURL,
	)
	if err != nil {
		logger.Error("Failed to initialize Cloudflare client", "error", err)
		os.Exit(1)
	}

	// -----------------------------------------
	// 2. Setup Application
	// -----------------------------------------

	// Register Auth Domain
	authRepo := auth.NewAuthRepository(db, []byte(cfg.JWTSecret))
	authService := auth.NewAuthService(authRepo)
	authHandler := auth.NewAuthHandler(authService, logger)

	// Register Video Domain
	videoAIRepo := video.NewAIRepository(whisperClient, chatGPTClient, logger)
	videoBatchRepo := video.NewBatchRepository(redisClient, logger)
	fileRepo := video.NewFileRepository(cloudflareClient, logger)
	videoRepo := video.NewVideoRepository(db)
	videoService := video.NewVideoService(videoRepo, videoAIRepo, videoBatchRepo, fileRepo)
	videoHandler := video.NewVideoHandler(videoService, queue)

	// Register Dialog Domain
	dialogAIRepo := dialog.NewAIRepository(chatGPTClient)
	dialogImageRepo := dialog.NewImageRepository(imageClient)
	dialogAudioRepo := dialog.NewAudioRepository(speechClient)
	dialogFileRepo := dialog.NewFileRepository(cloudflareClient)

	dialogBatchRepo := dialog.NewBatchRepository(redisClient, logger)
	dialogRepo := dialog.NewDialogRepository(db)
	dialogService := dialog.NewDialogService(dialogRepo, dialogAIRepo, dialogImageRepo, dialogAudioRepo, dialogFileRepo, dialogBatchRepo)
	dialogHandler := dialog.NewDialogHandler(dialogService, queue)

	// Register Profile Domain
	profileRepo := profile.NewProfileRepository(db)
	profileService := profile.NewProfileService(profileRepo)
	profileHandler := profile.NewProfileHandler(profileService)

	// -----------------------------------------
	// 3. Setup & Start Queue Server (Background Jobs)
	// -----------------------------------------
	queueServer := server.NewQueueServer(logger, queue, videoService, dialogService)
	queueServer.SetupWorkers()

	// สร้าง Context สำหรับควบคุม Lifecycle ของ Worker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// รัน Queue แบบ Asynchronous (ไม่บล็อก main thread)
	queueServer.Start(ctx, cfg.QueueWorkerCount)

	// -----------------------------------------
	// 4. Setup & Start HTTP Server
	// -----------------------------------------
	httpServer := server.NewHTTPServer(cfg, logger, authRepo, authHandler, videoHandler, dialogHandler, profileHandler)

	// สั่งรัน HTTP Server ใน Goroutine เพื่อให้ main thread ไปรอรับสัญญาณ Shutdown ได้
	go func() {
		if err := httpServer.Start(); err != nil {
			logger.Error("HTTP server failed", "error", err)
			// ถ้าพัง ให้ส่งสัญญาณปิดระบบทั้งหมด
			cancel()
		}
	}()

	// -----------------------------------------
	// 5. Graceful Shutdown
	// -----------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info("Received shutdown signal")
	case <-ctx.Done():
		logger.Info("Context cancelled, initiating shutdown")
	}

	// 1. สั่งยกเลิก Context ให้ Queue เลิกรับงานใหม่
	cancel()

	// 2. สั่งรอคิวเก่าทำงานให้เสร็จ
	queueServer.Stop()

	// 3. สั่งปิด HTTP Server (ถ้ามีเมธอด Stop ใน HTTPServer ของคุณ)
	// httpServer.Stop(ctx)

	logger.Info("Server exited gracefully")
}
