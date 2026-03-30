package dialog

import (
	"context"

	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/pkg/errors"
)

// ImageRepository generates dialog images.
type ImageRepository interface {
	GenerateImage(ctx context.Context, prompt string) ([]byte, *errors.AppError)
}

type imageRepository struct {
	imageClient *client.GeminiImageClient
}

// NewImageRepository creates a new dialog image repository.
func NewImageRepository(imageClient *client.GeminiImageClient) ImageRepository {
	return &imageRepository{imageClient: imageClient}
}

func (r *imageRepository) GenerateImage(ctx context.Context, prompt string) ([]byte, *errors.AppError) {
	if r.imageClient == nil {
		return nil, errors.Internal("dialog image client not configured")
	}
	return r.imageClient.GenerateImage(ctx, prompt)
}
