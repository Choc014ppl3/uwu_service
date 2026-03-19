package video

import (
	"context"
	"encoding/json"
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/pkg/errors"
	"github.com/windfall/uwu_service/pkg/response"
)

// VideoService handles video operations
type VideoService struct {
	videoRepo VideoRepository
	aiRepo    AIRepository
	batchRepo BatchRepository
	fileRepo  FileRepository
}

// VideoDetailsResponse is returned after a successful upload.
type VideoDetailsResponse struct {
	VideoID  string        `json:"video_id"`
	UserID   string        `json:"user_id"`
	Status   string        `json:"status"`
	Progress *BatchResult  `json:"progress"`
	Data     *LearningItem `json:"data"`
}

// ListVideoContentsResponse is returned when listing video contents.
type ListVideoContentsResponse struct {
	Data []*LearningItem `json:"data"`
	Meta *response.Meta  `json:"meta"`
}

// ToggleSavedResponse is returned after toggling saved state.
type ToggleSavedResponse struct {
	VideoID string `json:"video_id"`
	UserID  string `json:"user_id"`
	Saved   bool   `json:"saved"`
}

// StartActionResponse is returned after starting a quiz action.
type StartActionResponse struct {
	ActionID string `json:"action_id"`
	VideoID  string `json:"video_id"`
	UserID   string `json:"user_id"`
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

// List Video Contents
func (s *VideoService) ListVideoContents(ctx context.Context, input ListVideoContentsInput) (*ListVideoContentsResponse, *errors.AppError) {
	// 1. Get video contents from database
	videos, total, err := s.videoRepo.ListVideos(ctx, input.Limit, input.Offset)
	if err != nil {
		return nil, err
	}

	// 2. Calculate total pages
	totalPages := 0
	if input.PageSize > 0 {
		totalPages = (total + input.PageSize - 1) / input.PageSize
	}

	meta := &response.Meta{
		Page:       input.Page,
		PerPage:    input.PageSize,
		Total:      total,
		TotalPages: totalPages,
	}

	return &ListVideoContentsResponse{
		Data: videos,
		Meta: meta,
	}, nil
}

// Create Video Content
func (s *VideoService) CreateVideoContent(ctx context.Context, input UploadVideoPayload) (*VideoDetailsResponse, *errors.AppError) {
	// Create Batch for Processing
	_ = s.batchRepo.CreateBatch(ctx, input.VideoID)

	// Create initial LearningItem in DB
	metadataJSON, _ := json.Marshal(VideoMetadata{
		UserID: input.UserID,
		Status: BATCH_PENDING,
	})

	learningItem := &LearningItem{
		ID:        uuid.Must(uuid.Parse(input.VideoID)),
		Content:   "",             // Will be populated with transcript later
		Language:  input.Language, // Default, will be updated detection
		Level:     nil,
		Details:   json.RawMessage("{}"),
		Tags:      json.RawMessage("[]"),
		Metadata:  metadataJSON,
		CreatedBy: input.UserID,
		IsActive:  false, // Not active until processed
	}
	if err := s.videoRepo.CreateVideo(ctx, learningItem); err != nil {
		return nil, errors.InternalWrap("failed to create video content", err)
	}

	return &VideoDetailsResponse{
		VideoID: input.VideoID,
		UserID:  input.UserID,
		Status:  BATCH_PENDING,
	}, nil
}

// Process Upload Video -> background job for CreateVideoContent
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
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_UPLOAD_VIDEO, BATCH_FAILED, err.Error())
			return
		}

		// Update video URL
		_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_UPLOAD_VIDEO, BATCH_COMPLETED, "")
		// Send video URL to channel
		videoURL = url
	}()

	// Job A2: Upload Thumbnail to R2
	go func() {
		defer wg.Done()

		// Upload thumbnail to R2
		url, err := s.fileRepo.UploadToR2(ctx, payload.ThumbnailFile, payload.ThumbnailR2Path, payload.ThumbnailPath, payload.ThumbnailContentType)
		if err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_UPLOAD_THUMBNAIL, BATCH_FAILED, err.Error())
			return
		}

		// Update thumbnail URL
		_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_UPLOAD_THUMBNAIL, BATCH_COMPLETED, "")
		// Send thumbnail URL to channel
		thumbnailURL = url
	}()

	// Job B: Transcribe & Details
	go func() {
		defer wg.Done()

		// Extract audio from video
		if err := s.fileRepo.ExtractAudio(ctx, payload.VideoPath, payload.AudioPath); err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_TRANSCRIPT, BATCH_FAILED, err.Error())
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_FAILED, "skipped: generate details failed")
			return
		}

		// Generate video transcript
		transcript, err := s.aiRepo.GenerateVideoTranscript(ctx, payload.AudioPath, payload.Language)
		if err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_TRANSCRIPT, BATCH_FAILED, err.Error())
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_FAILED, "skipped: generate details failed")
			return
		}
		_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_TRANSCRIPT, BATCH_COMPLETED, "")

		// Generate video details
		details, err := s.aiRepo.GenerateVideoDetails(ctx, transcript)
		if err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_FAILED, err.Error())
			return
		}
		_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_COMPLETED, "")
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
	batch, _ := s.batchRepo.GetBatch(ctx, payload.VideoID)
	metadataJSON, _ := json.Marshal(VideoMetadata{
		UserID:       payload.UserID,
		Status:       BATCH_COMPLETED,
		VideoURL:     videoURL,
		ThumbnailURL: thumbnailURL,
		Batch:        batch,
	})
	detailsJSON, _ := json.Marshal(videoDetails)
	tagsJSON, _ := json.Marshal(videoDetails.Tags)

	learningItem := &LearningItem{
		ID:        uuid.Must(uuid.Parse(payload.VideoID)),
		Content:   videoDetails.Topic,
		Language:  videoDetails.Language,
		Level:     &videoDetails.Level,
		Details:   detailsJSON,
		Tags:      tagsJSON,
		Metadata:  metadataJSON,
		CreatedBy: payload.UserID,
		IsActive:  true,
	}

	if err := s.videoRepo.UpdateVideo(ctx, learningItem); err != nil {
		learningItem.IsActive = false // Set to false if update failed
		videoJSON, _ := json.Marshal(learningItem)
		_ = s.batchRepo.SetBatchResult(ctx, payload.VideoID, videoJSON)
		return
	}
}

// Get Video Details
func (s *VideoService) GetVideoDetails(ctx context.Context, videoID, userID string) (*VideoDetailsResponse, *errors.AppError) {
	// Get video from database
	learningItem, err := s.videoRepo.GetVideo(ctx, videoID)
	if err != nil {
		return nil, err
	}

	if learningItem != nil {
		var videoMetadata *VideoMetadata
		_ = json.Unmarshal(learningItem.Metadata, &videoMetadata)

		return &VideoDetailsResponse{
			VideoID:  videoID,
			UserID:   userID,
			Status:   BATCH_COMPLETED,
			Data:     learningItem,
			Progress: videoMetadata.Batch,
		}, nil
	}

	// Get video from batch
	batch, err := s.batchRepo.GetBatch(ctx, videoID)
	if err != nil {
		return nil, err
	}

	videoResponse := &VideoDetailsResponse{VideoID: videoID, UserID: userID}
	if batch != nil {
		var videoData *LearningItem
		_ = json.Unmarshal(batch.Result, &videoData)

		videoResponse.Data = videoData
		videoResponse.Status = batch.Status
		videoResponse.Progress = &BatchResult{
			BatchID:       batch.BatchID,
			Status:        batch.Status,
			TotalJobs:     batch.TotalJobs,
			CompletedJobs: batch.CompletedJobs,
			Jobs:          batch.Jobs,
			CreatedAt:     batch.CreatedAt,
		}

		return videoResponse, nil
	}

	// Return video details
	return nil, errors.NotFound("video not found")
}

// ToggleSaved toggles the saved action for a video.
func (s *VideoService) ToggleSaved(ctx context.Context, videoID, userID string) (*ToggleSavedResponse, *errors.AppError) {
	saved, err := s.videoRepo.ToggleSaved(ctx, videoID, userID)
	if err != nil {
		return nil, err
	}

	return &ToggleSavedResponse{
		VideoID: videoID,
		UserID:  userID,
		Saved:   saved,
	}, nil
}

// StartQuiz starts a quiz action for a video.
func (s *VideoService) StartQuiz(ctx context.Context, videoID, userID string) (*StartActionResponse, *errors.AppError) {
	actionID, err := s.videoRepo.StartQuiz(ctx, videoID, userID)
	if err != nil {
		return nil, err
	}

	return &StartActionResponse{
		ActionID: actionID,
		VideoID:  videoID,
		UserID:   userID,
	}, nil
}

