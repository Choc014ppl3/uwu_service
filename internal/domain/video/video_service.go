package video

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/pkg/errors"
)

// VideoService handles video operations
type VideoService struct {
	videoRepo VideoRepository
	batchRepo BatchRepository
	fileRepo  FileRepository
}

// VideoUploadResult is returned after a successful upload.
type VideoUploadResult struct {
	VideoID string `json:"video_id"`
	BatchID string `json:"batch_id"`
	Status  string `json:"status"`
}

// NewVideoService creates a new VideoService.
func NewVideoService(videoRepo VideoRepository, batchRepo BatchRepository, fileRepo FileRepository) *VideoService {
	return &VideoService{
		videoRepo: videoRepo,
		batchRepo: batchRepo,
		fileRepo:  fileRepo,
	}
}

// Create Video Content
func (s *VideoService) CreateVideoContent(ctx context.Context, input UploadVideoPayload) (*VideoUploadResult, *errors.AppError) {
	// Create Batch for Processing
	_ = s.batchRepo.CreateBatch(ctx, input.BatchID, input.VideoID, input.UserID)

	// Create initial LearningItem in DB
	metadataJSON, _ := json.Marshal(VideoMetadata{
		BatchID: input.BatchID,
		UserID:  input.UserID,
		Status:  "processing",
	})

	learningItemID, err := uuid.Parse(input.VideoID)
	if err != nil {
		return nil, errors.InternalWrap("failed to parse video ID", err)
	}
	learningItem := &LearningItem{
		ID:        learningItemID,
		FeatureID: 1,              // fixed value for type video
		Content:   "",             // Will be populated with transcript later
		Language:  input.Language, // Default, will be updated detection
		Details:   json.RawMessage("{}"),
		Tags:      json.RawMessage("[]"),
		Metadata:  metadataJSON,
		IsActive:  false, // Not active until processed
	}
	if err := s.videoRepo.CreateVideo(ctx, learningItem); err != nil {
		return nil, errors.InternalWrap("failed to create video content", err)
	}

	return &VideoUploadResult{
		VideoID: input.VideoID,
		BatchID: input.BatchID,
		Status:  "processing",
	}, nil
}

// ProcessUploadVideo -> process upload video job in background
func (s *VideoService) ProcessUploadVideo(ctx context.Context, payload UploadVideoPayload) {
	var wg sync.WaitGroup
	wg.Add(3)

	// Job A1: Upload Video to R2
	var videoURL string
	go func() {
		defer wg.Done()

		r2Path := fmt.Sprintf("videos/%s.%s", input.VideoID, strings.Split(input.VideoContentType, "/")[1])

		// Upload video to R2
		url, err := s.fileRepo.UploadToR2(ctx, r2Path, input.VideoPath, input.VideoContentType)
		if err != nil {
			_ = s.batchRepo.UpdateJob(ctx, input.BatchID, "video_upload", "failed", err.Error())
			return
		}

		// Update video URL
		_ = s.batchRepo.UpdateJob(ctx, input.BatchID, "video_upload", "completed", "")
		videoURL = url
	}()

	// Job A2: Upload Thumbnail to R2
	var thumbnailURL string
	go func() {
		defer wg.Done()

		r2Path := fmt.Sprintf("thumbnails/%s.%s", input.VideoID, strings.Split(input.ThumbnailContentType, "/")[1])

		// Upload thumbnail to R2
		url, err := s.fileRepo.UploadToR2(ctx, r2Path, input.ThumbnailPath, input.ThumbnailContentType)
		if err != nil {
			_ = s.batchRepo.UpdateJob(ctx, input.BatchID, "thumbnail_upload", "failed", err.Error())
			return
		}

		// Update thumbnail URL
		_ = s.batchRepo.UpdateJob(ctx, input.BatchID, "thumbnail_upload", "completed", "")
		thumbnailURL = url
	}()

	// Job B: Transcribe & Details
	var generatedContent GeneratedVideoContent
	go func() {
		defer wg.Done()

		audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_audio.wav", input.VideoID))
		defer os.Remove(audioPath)

		if err := s.fileRepo.ExtractAudio(input.VideoPath, audioPath); err != nil {
			_ = s.batchRepo.UpdateJob(ctx, input.BatchID, "generate_transcripts", "failed", err.Error())
			_ = s.batchRepo.UpdateJob(ctx, input.BatchID, "generate_details", "failed", "skipped: generate details failed")
			return
		}

		_ = s.batchRepo.UpdateJob(ctx, input.BatchID, "generate_transcripts", "completed", "")
	}()

	wg.Wait()
	defer os.Remove(input.VideoPath)
	defer os.Remove(input.ThumbnailPath)
}
