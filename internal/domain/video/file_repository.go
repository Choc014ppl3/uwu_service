package video

import (
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"os"
	"os/exec"

	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/pkg/errors"
)

// FileRepository interface
type FileRepository interface {
	ExtractAudio(ctx context.Context, videoPath, audioPath string) *errors.AppError
	UploadToR2(ctx context.Context, src multipart.File, key, path, contentType string) (string, *errors.AppError)
}

// fileRepository is the implementation of the FileRepository interface
type fileRepository struct {
	cloudflare *client.CloudflareClient
	log        *slog.Logger
}

// NewFileRepository creates a new fileRepository
func NewFileRepository(cloudflare *client.CloudflareClient, log *slog.Logger) *fileRepository {
	return &fileRepository{cloudflare: cloudflare, log: log}
}

// ExtractAudio extracts audio from a video file
func (r *fileRepository) ExtractAudio(ctx context.Context, videoPath, audioPath string) *errors.AppError {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", videoPath,
		"-vn",
		"-acodec", "pcm_s16le",
		"-ar", "16000",
		"-ac", "1",
		"-y",
		audioPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		r.log.Error("FFmpeg audio extraction failed", "error", err.Error(), "ffmpeg_output", string(output))
		return errors.InternalWrap("ffmpeg audio extraction", err)
	}

	return nil
}

// UploadToR2 uploads a file to R2
func (r *fileRepository) UploadToR2(ctx context.Context, src multipart.File, key, path, contentType string) (string, *errors.AppError) {
	// Save file to temp location
	dst, err := os.Create(path)
	if err != nil {
		return "", errors.InternalWrap("create temp file", err)
	}
	defer dst.Close()

	// Copy file to temp location
	if _, err := io.Copy(dst, src); err != nil {
		return "", errors.InternalWrap("write temp file", err)
	}

	// Open file
	file, err := os.Open(path)
	if err != nil {
		return "", errors.InternalWrap("open output file", err)
	}
	defer file.Close()

	// Upload file to R2
	url, err := r.cloudflare.UploadR2Object(ctx, key, file, contentType)
	if err != nil {
		return "", errors.InternalWrap("upload to R2", err)
	}

	return url, nil
}
