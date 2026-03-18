package dialog

import (
	"context"
	"fmt"

	"github.com/windfall/uwu_service/internal/infra/client"
)

// Job names
const (
	JOB_GENERATE_DIALOG = "GENERATE_DIALOG"
)

// RegisterDialogWorkers register dialog workers to queue
func RegisterDialogWorkers(queue *client.QueueClient, service *DialogService) {

	// Job Generate Dialog
	queue.RegisterWorker(JOB_GENERATE_DIALOG, func(ctx context.Context, job client.Job) error {
		payload, ok := job.Payload.(GenerateDialogPayload)
		if !ok {
			return fmt.Errorf("invalid payload type")
		}
		service.ProcessGenerateDialog(ctx, payload)
		return nil
	})
}
