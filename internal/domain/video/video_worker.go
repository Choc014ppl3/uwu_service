package video

import (
	"context"
	"fmt"

	"github.com/windfall/uwu_service/internal/infra/client"
)

// Job names
const (
	JobUploadVideo = "upload_video"
)

// RegisterVideoWorkers register video workers to queue
func RegisterVideoWorkers(queue *client.QueueClient, service *VideoService) {

	// Job Upload Video
	queue.RegisterWorker(JobUploadVideo, func(ctx context.Context, job client.Job) error {
		payload, ok := job.Payload.(UploadVideoPayload)
		if !ok {
			return fmt.Errorf("invalid payload type")
		}
		service.ProcessUploadVideo(ctx, payload)
		return nil
	})
}
