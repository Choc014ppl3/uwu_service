package service

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/repository"
)

// VideoService handles video upload and processing.
type VideoService struct {
	repo     repository.VideoRepository
	r2Client *client.CloudflareClient
	log      zerolog.Logger
}

// NewVideoService creates a new VideoService.
func NewVideoService(
	repo repository.VideoRepository,
	r2Client *client.CloudflareClient,
	log zerolog.Logger,
) *VideoService {
	return &VideoService{
		repo:     repo,
		r2Client: r2Client,
		log:      log,
	}
}

// VideoUploadResult is returned after a successful upload.
type VideoUploadResult struct {
	Video *repository.Video `json:"video"`
}

// ProcessUpload handles the full video upload pipeline:
// save to tmp → transcode with FFmpeg → upload to R2 → save metadata to DB.
func (s *VideoService) ProcessUpload(ctx context.Context, userID string, file multipart.File) (*VideoUploadResult, error) {
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, errors.Validation("invalid user ID")
	}

	videoID := uuid.New()
	inputPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_input.mp4", videoID))
	// ❌ ลบ: ไม่ต้องมีไฟล์ Output แยกแล้ว (ฝั่ง client แปลงไฟล์แล้ว)
	// outputPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_output.mp4", videoID))

	// CRITICAL: defer cleanup of temp files immediately
	defer os.Remove(inputPath)
	// defer os.Remove(outputPath)

	// Step 1: Save uploaded file to temp
	if err := s.saveTempFile(inputPath, file); err != nil {
		return nil, errors.InternalWrap("failed to save temp file", err)
	}

	// Step 2: Create initial DB record with "processing" status
	video := &repository.Video{
		UserID:   parsedUserID,
		VideoURL: "",
		Status:   "processing",
	}
	if err := s.repo.Create(ctx, video); err != nil {
		return nil, errors.InternalWrap("failed to create video record", err)
	}

	// Step 3: Transcode with FFmpeg
	// if err := s.transcode(inputPath, outputPath); err != nil {
	// 	_ = s.repo.UpdateStatus(ctx, video.ID, "failed", "")
	// 	return nil, errors.InternalWrap("video transcoding failed", err)
	// }

	// Step 4: Upload to R2
	r2Key := fmt.Sprintf("videos/%s.mp4", videoID)
	videoURL, err := s.uploadToR2(ctx, r2Key, inputPath)
	if err != nil {
		_ = s.repo.UpdateStatus(ctx, video.ID, "failed", "")
		return nil, errors.InternalWrap("failed to upload video to storage", err)
	}

	// Step 5: Update DB record with URL and "ready" status
	if err := s.repo.UpdateStatus(ctx, video.ID, "ready", videoURL); err != nil {
		return nil, errors.InternalWrap("failed to update video record", err)
	}

	video.VideoURL = videoURL
	video.Status = "ready"

	s.log.Info().
		Str("video_id", video.ID.String()).
		Str("user_id", userID).
		Str("video_url", videoURL).
		Msg("Video upload completed")

	return &VideoUploadResult{Video: video}, nil
}

// saveTempFile writes the multipart file to a temp path on disk.
func (s *VideoService) saveTempFile(path string, src multipart.File) error {
	dst, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	return nil
}

// uploadToR2 streams a file from disk to Cloudflare R2.
func (s *VideoService) uploadToR2(ctx context.Context, key, filePath string) (string, error) {
	if s.r2Client == nil {
		return "", fmt.Errorf("cloudflare R2 client not configured")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read output file: %w", err)
	}

	url, err := s.r2Client.UploadR2Object(ctx, key, data, "video/mp4")
	if err != nil {
		return "", fmt.Errorf("upload to R2: %w", err)
	}

	return url, nil
}
