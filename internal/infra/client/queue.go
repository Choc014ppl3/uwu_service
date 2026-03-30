package client

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/windfall/uwu_service/pkg/errors"
)

// Job คือโครงสร้างของงานที่จะส่งเข้า Queue
type Job struct {
	Type    string      // ชื่อประเภทงาน เช่น "process_upload_video"
	Payload interface{} // ข้อมูลที่ต้องการส่ง (ใช้ any หรือ interface{})
}

// WorkerFunc คือหน้าตาของฟังก์ชันที่แต่ละ Domain ต้องเขียนมารับงาน
type WorkerFunc func(ctx context.Context, job Job) error

// QueueClient คือตัวจัดการ Queue กลาง
type QueueClient struct {
	log      *slog.Logger
	jobsChan chan Job
	workers  map[string]WorkerFunc // เก็บว่างาน Type ไหน ต้องเรียกฟังก์ชันอะไร
	wg       sync.WaitGroup
}

// NewQueueClient สร้างคิวใหม่ตามขนาด Buffer ที่ต้องการ
func NewQueueClient(log *slog.Logger, bufferSize int) *QueueClient {
	return &QueueClient{
		log:      log,
		jobsChan: make(chan Job, bufferSize),
		workers:  make(map[string]WorkerFunc),
	}
}

// RegisterWorker ให้แต่ละ Domain นำ Worker ของตัวเองมาลงทะเบียน
// หมายเหตุ: ควร Register ให้เสร็จก่อนเรียก Start() เพื่อป้องกันปัญหา Data Race
func (c *QueueClient) RegisterWorker(jobType string, fn WorkerFunc) {
	c.workers[jobType] = fn
}

// Enqueue โยนงานเข้า Queue (เรียกจาก Handler)
func (c *QueueClient) Enqueue(job Job) *errors.AppError {
	select {
	case c.jobsChan <- job:
		return nil
	default:
		// ถ้า Buffer เต็ม จะคืนค่า Error ทันที (Non-blocking)
		return errors.ConflictWrap("queue is full, cannot enqueue job", fmt.Errorf("job type: %s", job.Type))
	}
}

// Start เริ่มเปิดรับงานด้วยจำนวน Goroutine (Workers) ตามที่ระบุ
func (c *QueueClient) Start(ctx context.Context, numWorkers int) {
	for i := range numWorkers {
		c.wg.Add(1)
		go c.process(ctx, i)
	}
}

// process คือลูปที่ Goroutine จะดึงงานไปทำ
func (c *QueueClient) process(ctx context.Context, workerID int) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done(): // รอรับสัญญาณ Shutdown
			c.log.Info("Worker shutting down", "worker_id", workerID)
			return
		case job := <-c.jobsChan:
			// ดึงงานมาหาว่าต้องเรียกฟังก์ชันไหน
			fn, exists := c.workers[job.Type]
			if !exists {
				c.log.Warn("No worker registered",
					"worker_id", workerID,
					"job_type", job.Type,
				)
				continue
			}

			// สั่งรันฟังก์ชันของ Domain นั้นๆ
			if err := fn(ctx, job); err != nil {
				c.log.Error("Failed to process job",
					"worker_id", workerID,
					"job_type", job.Type,
					"error", err,
				)
			} else {
				c.log.Info("Successfully processed job",
					"worker_id", workerID,
					"job_type", job.Type,
				)
			}
		}
	}
}

// Stop รอจนกว่า Worker ทุกตัวจะทำงานที่ค้างอยู่ให้เสร็จ (Graceful Shutdown)
func (c *QueueClient) Stop() {
	c.wg.Wait()
	close(c.jobsChan)
}
