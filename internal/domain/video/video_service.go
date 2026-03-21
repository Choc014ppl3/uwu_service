package video

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

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

// VideoDetailsResponse is returned for video details.
type VideoDetailsResponse struct {
	Data *LearningItem            `json:"data"`
	Meta *response.MetaProcessing `json:"meta"`
}

// ListVideoContentsResponse is returned when listing video contents.
type ListVideoContentsResponse struct {
	Data []*LearningItem          `json:"data"`
	Meta *response.MetaPagination `json:"meta"`
}

// ToggleSavedResponse is returned after toggling saved state.
type ToggleSavedResponse struct {
	ActionID string `json:"action_id"`
	VideoID  string `json:"video_id"`
	UserID   string `json:"user_id"`
	Saved    bool   `json:"saved"`
}

// StartActionResponse is returned after starting a quiz action.
type StartActionResponse struct {
	ActionID string `json:"action_id"`
	VideoID  string `json:"video_id"`
	UserID   string `json:"user_id"`
}

// ToggleTranscriptResponse is returned after toggling transcript state.
type ToggleTranscriptResponse struct {
	ActionID   string `json:"action_id"`
	VideoID    string `json:"video_id"`
	UserID     string `json:"user_id"`
	Transcript bool   `json:"transcript"`
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

	meta := &response.MetaPagination{
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
	batchProcessing, err := s.batchRepo.CreateBatch(ctx, input.VideoID)
	if err != nil {
		return nil, err
	}

	metadataJSON, _ := json.Marshal(batchProcessing)

	learningItem := &LearningItem{
		ID:        uuid.Must(uuid.Parse(input.VideoID)),
		Content:   "",
		Language:  input.Language,
		Level:     nil,
		Details:   json.RawMessage("{}"),
		Tags:      json.RawMessage("[]"),
		Metadata:  metadataJSON,
		CreatedBy: input.UserID,
		IsActive:  false,
	}
	if err := s.videoRepo.CreateVideo(ctx, learningItem); err != nil {
		return nil, errors.InternalWrap("failed to create video content", err)
	}

	return &VideoDetailsResponse{
		Data: learningItem,
		Meta: batchProcessing,
	}, nil
}

// Worker: ProcessUploadVideo handles the background upload flow for videos.
func (s *VideoService) ProcessUploadVideo(ctx context.Context, payload UploadVideoPayload) {
	var videoURL, thumbnailURL string
	var videoDetails *VideoDetails

	var wg sync.WaitGroup
	wg.Add(3)

	// Job A1: Upload Video to R2
	go func() {
		defer wg.Done()
		_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_UPLOAD_VIDEO, BATCH_PROCESSING, "")

		url, err := s.fileRepo.UploadToR2(ctx, payload.VideoFile, payload.VideoR2Path, payload.VideoPath, payload.VideoContentType)
		if err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_UPLOAD_VIDEO, BATCH_FAILED, err.Error())
			return
		}

		_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_UPLOAD_VIDEO, BATCH_COMPLETED, "")
		videoURL = url
	}()

	// Job A2: Upload Thumbnail to R2
	go func() {
		defer wg.Done()
		_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_UPLOAD_THUMBNAIL, BATCH_PROCESSING, "")

		url, err := s.fileRepo.UploadToR2(ctx, payload.ThumbnailFile, payload.ThumbnailR2Path, payload.ThumbnailPath, payload.ThumbnailContentType)
		if err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_UPLOAD_THUMBNAIL, BATCH_FAILED, err.Error())
			return
		}

		_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_UPLOAD_THUMBNAIL, BATCH_COMPLETED, "")
		thumbnailURL = url
	}()

	// Job B: Transcribe & Details
	go func() {
		defer wg.Done()
		_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_TRANSCRIPT, BATCH_PROCESSING, "")

		if err := s.fileRepo.ExtractAudio(ctx, payload.VideoPath, payload.AudioPath); err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_TRANSCRIPT, BATCH_FAILED, err.Error())
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_FAILED, "skipped: generate details failed")
			return
		}

		transcript, err := s.aiRepo.GenerateVideoTranscript(ctx, payload.AudioPath, payload.Language)
		if err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_TRANSCRIPT, BATCH_FAILED, err.Error())
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_FAILED, "skipped: generate details failed")
			return
		}
		_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_TRANSCRIPT, BATCH_COMPLETED, "")
		_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_PROCESSING, "")

		details, err := s.aiRepo.GenerateVideoDetails(ctx, transcript)
		if err != nil {
			_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_FAILED, err.Error())
			return
		}
		_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_COMPLETED, "")
		videoDetails = details
	}()

	// Wait for all jobs to complete
	wg.Wait()
	defer os.Remove(payload.AudioPath)
	defer os.Remove(payload.VideoPath)
	defer os.Remove(payload.ThumbnailPath)

	// Update video content
	_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_SAVE_VIDEO, BATCH_PROCESSING, "")

	videoDetails.VideoURL = videoURL
	videoDetails.ThumbnailURL = thumbnailURL

	detailsJSON, _ := json.Marshal(videoDetails)
	tagsJSON, _ := json.Marshal(videoDetails.Tags)

	batch, _ := s.batchRepo.GetBatch(ctx, payload.VideoID)
	if batch != nil {
		batch.Status = BATCH_COMPLETED
		batch.CompletedJobs = batch.TotalJobs
		now := time.Now().UTC().Format(time.RFC3339)
		for i := range batch.BatchJobs {
			if batch.BatchJobs[i].Name == PROCESS_SAVE_VIDEO {
				batch.BatchJobs[i].Status = BATCH_COMPLETED
				batch.BatchJobs[i].CompletedAt = now
			}
		}
	}

	metadataJSON, _ := json.Marshal(batch)

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
		_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_SAVE_VIDEO, BATCH_FAILED, err.GetMessage())
		return
	}

	_ = s.batchRepo.UpdateJob(ctx, payload.VideoID, PROCESS_SAVE_VIDEO, BATCH_COMPLETED, "")
}

// Get Video Details
func (s *VideoService) GetVideoDetails(ctx context.Context, videoID, userID string) (*VideoDetailsResponse, *errors.AppError) {
	// Get video from database
	learningItem, err := s.videoRepo.GetVideo(ctx, videoID)
	if err != nil {
		return nil, err
	}

	var metadata response.MetaProcessing
	if len(learningItem.Metadata) > 0 {
		_ = json.Unmarshal(learningItem.Metadata, &metadata)
		if metadata.Status == BATCH_COMPLETED {
			// Response complete batch processing item from database
			return &VideoDetailsResponse{
				Data: learningItem,
				Meta: &metadata,
			}, nil
		}
	}

	// Get batch from Redis
	metaProcessing, err := s.batchRepo.GetBatch(ctx, videoID)
	if err != nil {
		return nil, err
	}

	return &VideoDetailsResponse{
		Data: learningItem,
		Meta: metaProcessing,
	}, nil
}

// ToggleSaved toggles the saved action for a video.
func (s *VideoService) ToggleSaved(ctx context.Context, videoID, userID string) (*ToggleSavedResponse, *errors.AppError) {
	actionID, saved, err := s.videoRepo.ToggleSaved(ctx, videoID, userID)
	if err != nil {
		return nil, err
	}

	return &ToggleSavedResponse{
		ActionID: actionID,
		VideoID:  videoID,
		UserID:   userID,
		Saved:    saved,
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

// ToggleTranscript toggles the transcript action for a video.
func (s *VideoService) ToggleTranscript(ctx context.Context, videoID, userID string) (*ToggleTranscriptResponse, *errors.AppError) {
	actionID, enabled, err := s.videoRepo.ToggleTranscript(ctx, videoID, userID)
	if err != nil {
		return nil, err
	}

	return &ToggleTranscriptResponse{
		ActionID:   actionID,
		VideoID:    videoID,
		UserID:     userID,
		Transcript: enabled,
	}, nil
}
