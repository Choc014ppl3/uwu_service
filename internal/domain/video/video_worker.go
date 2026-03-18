package video

import (
	"context"
	"fmt"

	"github.com/windfall/uwu_service/internal/infra/client"
)

// Job names
const (
	JOB_UPLOAD_VIDEO = "UPLOAD_VIDEO"
)

// RegisterVideoWorkers register video workers to queue
func RegisterVideoWorkers(queue *client.QueueClient, service *VideoService) {

	// Job Upload Video
	queue.RegisterWorker(JOB_UPLOAD_VIDEO, func(ctx context.Context, job client.Job) error {
		payload, ok := job.Payload.(UploadVideoPayload)
		if !ok {
			return fmt.Errorf("invalid payload type")
		}
		service.ProcessUploadVideo(ctx, payload)
		return nil
	})
}
