package dialog

import (
	"bytes"
	"context"

	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/pkg/errors"
)

// FileRepository uploads generated dialog media.
type FileRepository interface {
	UploadBytes(ctx context.Context, data []byte, key, contentType string) (string, *errors.AppError)
}

type fileRepository struct {
	cloudflare *client.CloudflareClient
}

// NewFileRepository creates a new dialog file repository.
func NewFileRepository(cloudflare *client.CloudflareClient) FileRepository {
	return &fileRepository{cloudflare: cloudflare}
}

func (r *fileRepository) UploadBytes(ctx context.Context, data []byte, key, contentType string) (string, *errors.AppError) {
	if r.cloudflare == nil {
		return "", errors.Internal("dialog storage client not configured")
	}

	url, err := r.cloudflare.UploadR2Object(ctx, key, bytes.NewReader(data), contentType)
	if err != nil {
		return "", errors.InternalWrap("failed to upload dialog media", err)
	}

	return url, nil
}
