package server

import (
	"context"
	"log/slog"

	"github.com/windfall/uwu_service/internal/domain/dialog"
	"github.com/windfall/uwu_service/internal/domain/video"
	"github.com/windfall/uwu_service/internal/infra/client"
)

// QueueServer ทำหน้าที่จัดการ Lifecycle และการลงทะเบียน Worker ทั้งหมด
type QueueServer struct {
	queue *client.QueueClient
	log   *slog.Logger

	// Services ที่ Worker ต้องใช้ (ทำ DI เข้ามา)
	videoService  *video.VideoService
	dialogService *dialog.DialogService
}

// NewQueueServer สร้าง Instance ของตัวจัดการ Queue
func NewQueueServer(
	log *slog.Logger,
	queue *client.QueueClient,
	videoService *video.VideoService,
	dialogService *dialog.DialogService,
) *QueueServer {
	return &QueueServer{
		log:           log,
		queue:         queue,
		videoService:  videoService,
		dialogService: dialogService,
	}
}

// SetupWorkers ทำหน้าที่คล้ายๆ การ Setup Routes ใน HTTP Server
func (s *QueueServer) SetupWorkers() {
	s.log.Info("Registering background workers...")

	// มอบหมายให้แต่ละ Domain ลงทะเบียน Worker ของตัวเองเข้าคิวกลาง
	video.RegisterVideoWorkers(s.queue, s.videoService)
	dialog.RegisterDialogWorkers(s.queue, s.dialogService)
}

// Start สั่งรันคิว
func (s *QueueServer) Start(ctx context.Context, numWorkers int) {
	s.log.Info("Starting Queue Server", "workers", numWorkers)

	// ให้คิวกลางเริ่มดึงงานไปทำ
	s.queue.Start(ctx, numWorkers)
}

// Stop สั่งปิดคิวอย่างปลอดภัย (Graceful Shutdown)
func (s *QueueServer) Stop() {
	s.log.Info("Stopping Queue Server...")
	s.queue.Stop()
	s.log.Info("Queue Server stopped completely")
}
