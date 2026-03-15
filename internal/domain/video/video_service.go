package video

import (
	"context"
	"encoding/json"
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/pkg/errors"
)

// VideoService handles video operations
type VideoService struct {
	videoRepo VideoRepository
	aiRepo    AIRepository
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
func NewVideoService(videoRepo VideoRepository, aiRepo AIRepository, batchRepo BatchRepository, fileRepo FileRepository) *VideoService {
	return &VideoService{
		videoRepo: videoRepo,
		aiRepo:    aiRepo,
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
		Status:  BATCH_PENDING,
	})

	learningItem := &LearningItem{
		ID:       uuid.Must(uuid.Parse(input.VideoID)),
		Content:  "",             // Will be populated with transcript later
		Language: input.Language, // Default, will be updated detection
		Level:    nil,
		Details:  json.RawMessage("{}"),
		Tags:     json.RawMessage("[]"),
		Metadata: metadataJSON,
		IsActive: false, // Not active until processed
	}
	if err := s.videoRepo.CreateVideo(ctx, learningItem); err != nil {
		return nil, errors.InternalWrap("failed to create video content", err)
	}

	return &VideoUploadResult{
		VideoID: input.VideoID,
		BatchID: input.BatchID,
		Status:  BATCH_PENDING,
	}, nil
}

// ProcessUploadVideo -> process upload video job in background
func (s *VideoService) ProcessUploadVideo(ctx context.Context, payload UploadVideoPayload) {
	// Create variables
	var videoURL, thumbnailURL string
	var videoDetails *VideoDetails

	var wg sync.WaitGroup
	wg.Add(3)

	// Job A1: Upload Video to R2
	go func() {
		defer wg.Done()

		// Upload video to R2
		url, err := s.fileRepo.UploadToR2(ctx, payload.VideoFile, payload.VideoR2Path, payload.VideoPath, payload.VideoContentType)
		if err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.BatchID, PROCESS_UPLOAD_VIDEO, BATCH_FAILED, err.Error())
			return
		}

		// Update video URL
		_ = s.batchRepo.UpdateJob(ctx, payload.BatchID, PROCESS_UPLOAD_VIDEO, BATCH_COMPLETED, "")
		// Send video URL to channel
		videoURL = url
	}()

	// Job A2: Upload Thumbnail to R2
	go func() {
		defer wg.Done()

		// Upload thumbnail to R2
		url, err := s.fileRepo.UploadToR2(ctx, payload.ThumbnailFile, payload.ThumbnailR2Path, payload.ThumbnailPath, payload.ThumbnailContentType)
		if err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.BatchID, PROCESS_UPLOAD_THUMBNAIL, BATCH_FAILED, err.Error())
			return
		}

		// Update thumbnail URL
		_ = s.batchRepo.UpdateJob(ctx, payload.BatchID, PROCESS_UPLOAD_THUMBNAIL, BATCH_COMPLETED, "")
		// Send thumbnail URL to channel
		thumbnailURL = url
	}()

	// Job B: Transcribe & Details
	go func() {
		defer wg.Done()

		// Extract audio from video
		if err := s.fileRepo.ExtractAudio(ctx, payload.VideoPath, payload.AudioPath); err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.BatchID, PROCESS_GENERATE_TRANSCRIPT, BATCH_FAILED, err.Error())
			_ = s.batchRepo.UpdateJob(ctx, payload.BatchID, PROCESS_GENERATE_DETAILS, BATCH_FAILED, "skipped: generate details failed")
			return
		}

		// Generate video transcript
		transcript, err := s.aiRepo.GenerateVideoTranscript(ctx, payload.AudioPath)
		if err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.BatchID, PROCESS_GENERATE_TRANSCRIPT, BATCH_FAILED, err.Error())
			_ = s.batchRepo.UpdateJob(ctx, payload.BatchID, PROCESS_GENERATE_DETAILS, BATCH_FAILED, "skipped: generate details failed")
			return
		}
		_ = s.batchRepo.UpdateJob(ctx, payload.BatchID, PROCESS_GENERATE_TRANSCRIPT, BATCH_COMPLETED, "")

		// Generate video details
		details, err := s.aiRepo.GenerateVideoDetails(ctx, transcript)
		if err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.BatchID, PROCESS_GENERATE_DETAILS, BATCH_FAILED, err.Error())
			return
		}
		_ = s.batchRepo.UpdateJob(ctx, payload.BatchID, PROCESS_GENERATE_DETAILS, BATCH_COMPLETED, "")
		// Send details to channel
		videoDetails = details
	}()

	// Wait for all jobs to complete
	wg.Wait()
	// Remove temporary files
	defer os.Remove(payload.AudioPath)
	defer os.Remove(payload.VideoPath)
	defer os.Remove(payload.ThumbnailPath)

	// Update video content
	metadataJSON, _ := json.Marshal(VideoMetadata{
		BatchID:      payload.BatchID,
		UserID:       payload.UserID,
		Status:       BATCH_COMPLETED,
		VideoURL:     videoURL,
		ThumbnailURL: thumbnailURL,
	})
	detailsJSON, _ := json.Marshal(videoDetails)
	tagsJSON, _ := json.Marshal(videoDetails.Tags)

	learningItem := &LearningItem{
		ID:       uuid.Must(uuid.Parse(payload.VideoID)),
		Content:  videoDetails.Topic,
		Language: videoDetails.Language,
		Level:    &videoDetails.Level,
		Details:  detailsJSON,
		Tags:     tagsJSON,
		Metadata: metadataJSON,
		IsActive: true,
	}

	if err := s.videoRepo.UpdateVideo(ctx, learningItem); err != nil {
		videoJSON, _ := json.Marshal(learningItem)
		_ = s.batchRepo.SetBatchResult(ctx, payload.BatchID, videoJSON)
		return
	}
}
