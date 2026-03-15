package video

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"os"
	"os/exec"

	"github.com/windfall/uwu_service/internal/infra/client"
)

// FileRepository interface
type FileRepository interface {
	ExtractAudio(videoPath, audioPath string) error
	SaveTempFile(path string, src multipart.File) error
	UploadToR2(ctx context.Context, key, filePath, contentType string) (string, error)
}

// fileRepository is the implementation of the FileRepository interface
type fileRepository struct {
	cloudflare *client.CloudflareClient
	log        *slog.Logger
}

// NewFileRepository creates a new fileRepository
func NewFileRepository(log *slog.Logger) *fileRepository {
	return &fileRepository{log: log}
}

// ExtractAudio extracts audio from a video file
func (r *fileRepository) ExtractAudio(ctx context.Context, videoPath, audioPath string) error {
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
		return fmt.Errorf("ffmpeg audio extraction: %w", err)
	}

	return nil
}

// SaveTempFile saves a file to a temporary location
func (r *fileRepository) SaveTempFile(path string, src multipart.File) error {
	dst, err := os.Create(path)
	if err != nil {
		os.Remove(path) // Clean up immediately on failure
		return fmt.Errorf("create temp file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(path) // Clean up immediately on failure
		return fmt.Errorf("write temp file: %w", err)
	}

	return nil
}

// UploadToR2 uploads a file to R2
func (r *fileRepository) UploadToR2(ctx context.Context, key, filePath, contentType string) (string, error) {
	if r.cloudflare == nil {
		return "", fmt.Errorf("cloudflare R2 client not configured")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open output file: %w", err)
	}
	defer file.Close()

	url, err := r.cloudflare.UploadR2Object(ctx, key, file, contentType)
	if err != nil {
		return "", fmt.Errorf("upload to R2: %w", err)
	}

	return url, nil
}
