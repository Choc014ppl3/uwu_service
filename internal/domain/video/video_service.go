package video

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// StartQuizResponse is returned after starting a gist quiz action.
type StartQuizResponse struct {
	ActionID string      `json:"action_id"`
	VideoID  string      `json:"video_id"`
	UserID   string      `json:"user_id"`
	GistQuiz interface{} `json:"gist_quiz"`
}

// StartRetellResponse is returned after starting a retell story action.
type StartRetellResponse struct {
	ActionID    string      `json:"action_id"`
	VideoID     string      `json:"video_id"`
	UserID      string      `json:"user_id"`
	RetellStory interface{} `json:"retell_story"`
}

// ToggleTranscriptResponse is returned after toggling transcript state.
type ToggleTranscriptResponse struct {
	ActionID   string `json:"action_id"`
	VideoID    string `json:"video_id"`
	UserID     string `json:"user_id"`
	Transcript bool   `json:"transcript"`
}

type VideoGistQuiz []gistQuizQuestion

type VideoRetell struct {
	KeyPoints     []string `json:"key_points"`
	RetellExample string   `json:"retell_example"`
}

// QuizMetadata represents the metadata stored in user_actions for quiz activities
type QuizMetadata struct {
	GistQuiz       *VideoGistQuiz    `json:"gist_quiz,omitempty"`
	RetellStory    *VideoRetell      `json:"retell_story,omitempty"`
	QuizAttempts   []GistQuizAttempt `json:"quiz_attempts,omitempty"`
	RetellAttempts []RetellAttempt   `json:"retell_attempts,omitempty"`
}

// GistQuizAttempt represents a single attempt at the multiple-choice gist quiz
type GistQuizAttempt struct {
	AttemptID   string       `json:"attempt_id"`
	Answers     []QuizAnswer `json:"answers"`
	QuizScore   float64      `json:"quiz_score"`
	SubmittedAt time.Time    `json:"submitted_at"`
}

// RetellAttempt represents a single attempt at the audio retell story
type RetellAttempt struct {
	AttemptID        string         `json:"attempt_id"`
	AudioURL         string         `json:"audio_url"`
	Transcript       string         `json:"transcript"`
	RetellScore      float64        `json:"retell_score"`
	MatchesKeyPoints []string       `json:"matches_key_points"`
	RetellAnalysis   string         `json:"retell_analysis"`
	ScoringBreakdown map[string]any `json:"scoring_breakdown"`
	SubmittedAt      time.Time      `json:"submitted_at"`
}

type SubmitGistQuizResponse struct {
	AttemptID string  `json:"attempt_id"`
	QuizScore float64 `json:"quiz_score"`
}

type SubmitRetellResponse struct {
	AttemptID   string  `json:"attempt_id"`
	RetellScore float64 `json:"retell_score"`
}

// SubmitQuizResponse is returned after submitting a quiz attempt
type SubmitQuizResponse struct {
	Score float64 `json:"score"`
}

type gistQuizOption struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	IsCorrect bool   `json:"is_correct"`
}

type gistQuizQuestion struct {
	ID          int              `json:"id"`
	Type        string           `json:"type"`
	Options     []gistQuizOption  `json:"options"`
	Category    string           `json:"category"`
	Question    string           `json:"question"`
	CorrectOrder []string        `json:"correct_order"`
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
func (s *VideoService) GetVideoDetails(ctx context.Context, videoID string) (*VideoDetailsResponse, *errors.AppError) {
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

	if metaProcessing == nil {
		metaProcessing = &metadata
	}

	return &VideoDetailsResponse{
		Data: learningItem,
		Meta: metaProcessing,
	}, nil
}

// ToggleSaved toggles the saved action for a video.
func (s *VideoService) ToggleSaved(ctx context.Context, input ToggleSavedInput) (*ToggleSavedResponse, *errors.AppError) {
	actionID, saved, err := s.videoRepo.ToggleSaved(ctx, input.VideoID, input.UserID)
	if err != nil {
		return nil, err
	}

	return &ToggleSavedResponse{
		ActionID: actionID,
		VideoID:  input.VideoID,
		UserID:   input.UserID,
		Saved:    saved,
	}, nil
}

// StartQuiz starts a gist quiz action for a video.
func (s *VideoService) StartQuiz(ctx context.Context, input StartQuizInput) (*StartQuizResponse, *errors.AppError) {
	videoID := input.VideoID
	userID := input.UserID

	// 1. Check if user already started this action (Idempotency)
	action, exists, err := s.videoRepo.GetActionByUserID(ctx, videoID, userID, "submit_quiz")
	if err != nil {
		return nil, err
	}

	if exists {
		var metadata QuizMetadata
		_ = json.Unmarshal(action.Metadata, &metadata)
		return &StartQuizResponse{
			ActionID: action.ID,
			VideoID:  videoID,
			UserID:   userID,
			GistQuiz: metadata.GistQuiz,
		}, nil
	}

	// 2. Fetch video details to get quiz snapshot
	videoItem, err := s.videoRepo.GetVideo(ctx, videoID)
	if err != nil {
		return nil, err
	}

	var videoDetails VideoDetails
	if err := json.Unmarshal(videoItem.Details, &videoDetails); err != nil {
		return nil, errors.InternalWrap("failed to parse video details", err)
	}

	// 3. Create initial metadata snapshot
	metadata := QuizMetadata{
		QuizAttempts: []GistQuizAttempt{},
	}
	gistJSON, _ := json.Marshal(videoDetails.GistQuiz)
	_ = json.Unmarshal(gistJSON, &metadata.GistQuiz)
	metadataJSON, _ := json.Marshal(metadata)

	// 4. Create action record
	actionID, err := s.videoRepo.StartQuiz(ctx, videoID, userID, metadataJSON)
	if err != nil {
		return nil, err
	}

	return &StartQuizResponse{
		ActionID: actionID,
		VideoID:  videoID,
		UserID:   userID,
		GistQuiz: metadata.GistQuiz,
	}, nil
}

// StartRetell starts a retell story action for a video.
func (s *VideoService) StartRetell(ctx context.Context, input StartRetellInput) (*StartRetellResponse, *errors.AppError) {
	videoID := input.VideoID
	userID := input.UserID

	// 1. Check if user already started this action (Idempotency)
	action, exists, err := s.videoRepo.GetActionByUserID(ctx, videoID, userID, "submit_retell")
	if err != nil {
		return nil, err
	}

	if exists {
		var metadata QuizMetadata
		_ = json.Unmarshal(action.Metadata, &metadata)
		return &StartRetellResponse{
			ActionID:    action.ID,
			VideoID:     videoID,
			UserID:      userID,
			RetellStory: metadata.RetellStory,
		}, nil
	}

	// 2. Fetch video details to get retell snapshot
	videoItem, err := s.videoRepo.GetVideo(ctx, videoID)
	if err != nil {
		return nil, err
	}

	var videoDetails VideoDetails
	if err := json.Unmarshal(videoItem.Details, &videoDetails); err != nil {
		return nil, errors.InternalWrap("failed to parse video details", err)
	}

	// 3. Create initial metadata snapshot
	metadata := QuizMetadata{
		RetellAttempts: []RetellAttempt{},
	}
	retellJSON, _ := json.Marshal(videoDetails.RetellStory)
	_ = json.Unmarshal(retellJSON, &metadata.RetellStory)
	metadataJSON, _ := json.Marshal(metadata)

	// 4. Create action record
	actionID, err := s.videoRepo.StartRetell(ctx, videoID, userID, metadataJSON)
	if err != nil {
		return nil, err
	}

	return &StartRetellResponse{
		ActionID:    actionID,
		VideoID:     videoID,
		UserID:      userID,
		RetellStory: metadata.RetellStory,
	}, nil
}

// SubmitGistQuiz handles the submission and scoring of a gist quiz.
func (s *VideoService) SubmitGistQuiz(ctx context.Context, input SubmitGistQuizInput) (*SubmitGistQuizResponse, *errors.AppError) {
	// 1. Get existing action by videoID, userID, and type
	action, exists, err := s.videoRepo.GetActionByUserID(ctx, input.VideoID, input.UserID, "submit_quiz")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NotFound("quiz action not found for this video")
	}

	var metadata QuizMetadata
	if err := json.Unmarshal(action.Metadata, &metadata); err != nil {
		return nil, errors.InternalWrap("failed to parse quiz metadata", err)
	}

	// 2. Score answers
	quizScore := scoreQuizAnswers(metadata.GistQuiz, input.Answers)

	// 3. Create attempt
	attemptID := uuid.New().String()
	attempt := GistQuizAttempt{
		AttemptID:   attemptID,
		Answers:     input.Answers,
		QuizScore:   quizScore,
		SubmittedAt: time.Now().UTC(),
	}

	// 4. Update metadata
	metadata.QuizAttempts = append(metadata.QuizAttempts, attempt)
	metadataJSON, _ := json.Marshal(metadata)

	if err := s.videoRepo.UpdateQuizAction(ctx, action.ID, metadataJSON); err != nil {
		return nil, err
	}

	return &SubmitGistQuizResponse{
		AttemptID: attemptID,
		QuizScore: quizScore,
	}, nil
}

// SubmitRetellStory handles the submission and AI evaluation of a retell story.
func (s *VideoService) SubmitRetellStory(ctx context.Context, input SubmitRetellInput) (*SubmitRetellResponse, *errors.AppError) {
	// 1. Get existing action by videoID, userID, and type
	action, exists, err := s.videoRepo.GetActionByUserID(ctx, input.VideoID, input.UserID, "submit_retell")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NotFound("retell action not found for this video")
	}

	var metadata QuizMetadata
	if err := json.Unmarshal(action.Metadata, &metadata); err != nil {
		return nil, errors.InternalWrap("failed to parse quiz metadata", err)
	}

	// 2. Fetch video details for key points
	videoItem, err := s.videoRepo.GetVideo(ctx, action.LearningID)
	if err != nil {
		return nil, err
	}

	var videoDetails VideoDetails
	if err := json.Unmarshal(videoItem.Details, &videoDetails); err != nil {
		return nil, errors.InternalWrap("failed to parse video details", err)
	}

	// 3. Process audio
	tempWav, err := s.fileRepo.SaveMultipartToTemp(input.AudioFile, "retell_*.wav")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempWav.Name())

	transcript, err := s.aiRepo.GenerateVideoTranscript(ctx, tempWav.Name(), videoItem.Language)
	if err != nil {
		return nil, err
	}

	attemptID := uuid.New().String()
	objectKey := fmt.Sprintf("retell-quiz/%s-%s.m4a", action.ID, attemptID)
	m4aPath := filepath.Join(os.TempDir(), attemptID+".m4a")
	defer os.Remove(m4aPath)

	if err := s.fileRepo.ConvertAudioToM4A(ctx, tempWav.Name(), m4aPath); err != nil {
		return nil, err
	}

	m4aFile, openErr := os.Open(m4aPath)
	if openErr != nil {
		return nil, errors.InternalWrap("failed to open m4a file", openErr)
	}
	defer m4aFile.Close()

	audioURL, err := s.fileRepo.UploadToR2(ctx, m4aFile, objectKey, m4aPath, "audio/mp4")
	if err != nil {
		return nil, err
	}

	// 4. AI Evaluation
	eval, err := s.aiRepo.EvaluateRetellStory(ctx, transcript.Text, videoDetails.RetellStory.KeyPoints)
	if err != nil {
		return nil, err
	}

	// 5. Create attempt
	attempt := RetellAttempt{
		AttemptID:        attemptID,
		AudioURL:         audioURL,
		Transcript:       transcript.Text,
		RetellScore:      eval.Score,
		MatchesKeyPoints: eval.MatchesKeyPoints,
		RetellAnalysis:   eval.Analysis,
		ScoringBreakdown: map[string]any{
			"retell_score": eval.Score,
		},
		SubmittedAt: time.Now().UTC(),
	}

	// 6. Update metadata
	metadata.RetellAttempts = append(metadata.RetellAttempts, attempt)
	metadataJSON, _ := json.Marshal(metadata)

	if err := s.videoRepo.UpdateQuizAction(ctx, action.ID, metadataJSON); err != nil {
		return nil, err
	}

	return &SubmitRetellResponse{
		AttemptID:   attemptID,
		RetellScore: eval.Score,
	}, nil
}

func scoreQuizAnswers(gistQuiz any, answers []QuizAnswer) float64 {
	raw, err := json.Marshal(gistQuiz)
	if err != nil {
		return 0
	}

	var questions []gistQuizQuestion
	if err := json.Unmarshal(raw, &questions); err != nil {
		return 0
	}

	if len(questions) == 0 {
		return 0
	}

	answerMap := map[int]QuizAnswer{}
	for _, ans := range answers {
		answerMap[ans.QuizID] = ans
	}

	var total float64
	for _, quiz := range questions {
		ans, ok := answerMap[quiz.ID]
		if !ok {
			continue
		}
		switch quiz.Type {
		case "single_choice":
			correct := ""
			for _, opt := range quiz.Options {
				if opt.IsCorrect {
					correct = opt.ID
					break
				}
			}
			if len(ans.OptionIDs) == 1 && ans.OptionIDs[0] == correct {
				total += 1
			}
		case "multiple_response":
			correctSet := map[string]struct{}{}
			for _, opt := range quiz.Options {
				if opt.IsCorrect {
					correctSet[opt.ID] = struct{}{}
				}
			}
			if len(ans.OptionIDs) == len(correctSet) {
				match := true
				for _, id := range ans.OptionIDs {
					if _, ok := correctSet[id]; !ok {
						match = false
						break
					}
				}
				if match {
					total += 1
				}
			}
		case "ordering":
			if len(ans.Order) == len(quiz.CorrectOrder) {
				match := true
				for i := range quiz.CorrectOrder {
					if ans.Order[i] != quiz.CorrectOrder[i] {
						match = false
						break
					}
				}
				if match {
					total += 1
				}
			}
		}
	}

	return (total / float64(len(questions))) * 100
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
