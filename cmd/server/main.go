package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

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
	// 3. Setup & Start Queue Server (Background Jobs)
	// -----------------------------------------
	queue := client.NewQueueClient(logger, cfg.QueueBufferSize)
	queueServer := server.NewQueueServer(logger, queue, videoService)
	queueServer.SetupWorkers()

	// สร้าง Context สำหรับควบคุม Lifecycle ของ Worker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// รัน Queue แบบ Asynchronous (ไม่บล็อก main thread)
	queueServer.Start(ctx, cfg.QueueWorkerCount)

	// -----------------------------------------
	// 4. Setup & Start HTTP Server
	// -----------------------------------------
	httpServer := server.NewHTTPServer(cfg, logger, jwtRepo, authHandler)

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
