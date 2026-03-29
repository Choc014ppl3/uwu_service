package dialog

import (
	"context"
	"fmt"

	"github.com/windfall/uwu_service/internal/infra/client"
)

// Worker names
const (
	WORKER_GENERATE_DIALOG    = "GENERATE_DIALOG"
	WORKER_REPLY_CHAT_MESSAGE = "REPLY_CHAT_MESSAGE"
)

// RegisterDialogWorkers register dialog workers to queue
func RegisterDialogWorkers(queue *client.QueueClient, service *DialogService) {

	// Job Generate Dialog
	queue.RegisterWorker(WORKER_GENERATE_DIALOG, func(ctx context.Context, job client.Job) error {
		payload, ok := job.Payload.(GenerateDialogPayload)
		if !ok {
			return fmt.Errorf("invalid payload type")
		}
		service.ProcessGenerateDialog(ctx, payload)
		return nil
	})

	// Job Reply Chat Message
	queue.RegisterWorker(WORKER_REPLY_CHAT_MESSAGE, func(ctx context.Context, job client.Job) error {
		payload, ok := job.Payload.(ReplyChatMessagePayload)
		if !ok {
			return fmt.Errorf("invalid payload type")
		}
		service.ProcessReplyChatMessage(ctx, payload)
		return nil
	})
}
