package video

import (
	"context"
	"fmt"

	"github.com/windfall/uwu_service/internal/infra/client"
)

// Worker names
const (
	WORKER_UPLOAD_VIDEO   = "worker_upload_video"
	WORKER_EVALUATE_RETEL = "worker_evaluate_retel"
)

// RegisterVideoWorkers register video workers to queue
func RegisterVideoWorkers(queue *client.QueueClient, service *VideoService) {

	// Job Upload Video
	queue.RegisterWorker(WORKER_UPLOAD_VIDEO, func(ctx context.Context, job client.Job) error {
		payload, ok := job.Payload.(UploadVideoPayload)
		if !ok {
			return fmt.Errorf("invalid %s payload type", WORKER_UPLOAD_VIDEO)
		}
		service.ProcessUploadVideo(ctx, payload)
		return nil
	})
}

// RegisterEvaluateRetelWorker register evaluate retel worker to queue
func RegisterEvaluateRetelWorker(queue *client.QueueClient, service *VideoService) {

	// Job Evaluate Retel
	queue.RegisterWorker(WORKER_EVALUATE_RETEL, func(ctx context.Context, job client.Job) error {
		payload, ok := job.Payload.(SubmitRetellPayload)
		if !ok {
			return fmt.Errorf("invalid %s payload type", WORKER_EVALUATE_RETEL)
		}
		service.ProcessEvaluateRetel(ctx, payload)
		return nil
	})
}
