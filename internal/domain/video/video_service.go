package video

import (
	"context"
	"encoding/json"
	"os"
	"sort"
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
	ActionID string            `json:"action_id"`
	VideoID  string            `json:"video_id"`
	UserID   string            `json:"user_id"`
	GistQuiz interface{}       `json:"gist_quiz"`
	Attempts []GistQuizAttempt `json:"attempts"`
}

// StartRetellResponse is returned after starting a retell story action.
type StartRetellResponse struct {
	ActionID    string          `json:"action_id"`
	VideoID     string          `json:"video_id"`
	UserID      string          `json:"user_id"`
	RetellStory interface{}     `json:"retell_story"`
	Attempts    []RetellAttempt `json:"attempts"`
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

// GistQuizMetadata represents the metadata for gist quiz actions
type GistQuizMetadata struct {
	GistQuiz *VideoGistQuiz    `json:"gist_quiz,omitempty"`
	Attempts []GistQuizAttempt `json:"attempts"`
}

// RetellStoryMetadata represents the metadata for retell story actions
type RetellStoryMetadata struct {
	RetellStory *VideoRetell    `json:"retell_story,omitempty"`
	Attempts    []RetellAttempt `json:"attempts"`
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
	AttemptID        string    `json:"attempt_id"`
	AudioURL         string    `json:"audio_url"`
	MimeType         string    `json:"mimeType"`
	Transcript       string    `json:"transcript"`
	RetellScore      float64   `json:"retell_score"`
	MatchesKeyPoints []string  `json:"matches_key_points"`
	RetellAnalysis   string    `json:"retell_analysis"`
	SubmittedAt      time.Time `json:"submitted_at"`
}

type gistQuizOption struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	IsCorrect bool   `json:"is_correct"`
}

type gistQuizQuestion struct {
	ID           int              `json:"id"`
	Type         string           `json:"type"`
	Options      []gistQuizOption `json:"options"`
	Category     string           `json:"category"`
	Question     string           `json:"question"`
	CorrectOrder []string         `json:"correct_order"`
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
	batchProcessing, err := s.batchRepo.CreateUploadVideoBatch(ctx, input.VideoID)
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
		_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_UPLOAD_VIDEO, BATCH_PROCESSING, "")

		url, err := s.fileRepo.UploadToR2(ctx, payload.VideoFile, payload.VideoR2Path, payload.VideoPath, payload.VideoContentType)
		if err != nil {
			_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_UPLOAD_VIDEO, BATCH_FAILED, err.Error())
			return
		}

		_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_UPLOAD_VIDEO, BATCH_COMPLETED, "")
		videoURL = url
	}()

	// Job A2: Upload Thumbnail to R2
	go func() {
		defer wg.Done()
		_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_UPLOAD_THUMBNAIL, BATCH_PROCESSING, "")

		url, err := s.fileRepo.UploadToR2(ctx, payload.ThumbnailFile, payload.ThumbnailR2Path, payload.ThumbnailPath, payload.ThumbnailContentType)
		if err != nil {
			_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_UPLOAD_THUMBNAIL, BATCH_FAILED, err.Error())
			return
		}

		_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_UPLOAD_THUMBNAIL, BATCH_COMPLETED, "")
		thumbnailURL = url
	}()

	// Job B: Transcribe & Details
	go func() {
		defer wg.Done()
		_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_GENERATE_TRANSCRIPT, BATCH_PROCESSING, "")

		if err := s.fileRepo.ExtractAudio(ctx, payload.VideoPath, payload.AudioPath); err != nil {
			_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_GENERATE_TRANSCRIPT, BATCH_FAILED, err.Error())
			_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_FAILED, "skipped: generate details failed")
			return
		}

		transcript, err := s.aiRepo.GenerateVideoTranscript(ctx, payload.AudioPath, payload.Language)
		if err != nil {
			_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_GENERATE_TRANSCRIPT, BATCH_FAILED, err.Error())
			_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_FAILED, "skipped: generate details failed")
			return
		}
		_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_GENERATE_TRANSCRIPT, BATCH_COMPLETED, "")
		_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_PROCESSING, "")

		details, err := s.aiRepo.GenerateVideoDetails(ctx, transcript)
		if err != nil {
			_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_FAILED, err.Error())
			return
		}
		_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_GENERATE_DETAILS, BATCH_COMPLETED, "")
		videoDetails = details
	}()

	// Wait for all jobs to complete
	wg.Wait()
	defer os.Remove(payload.AudioPath)
	defer os.Remove(payload.VideoPath)
	defer os.Remove(payload.ThumbnailPath)

	// Update video content
	_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_SAVE_VIDEO, BATCH_PROCESSING, "")

	videoDetails.VideoURL = videoURL
	videoDetails.ThumbnailURL = thumbnailURL

	detailsJSON, _ := json.Marshal(videoDetails)
	tagsJSON, _ := json.Marshal(videoDetails.Tags)

	batch, _ := s.batchRepo.GetUploadVideoBatch(ctx, payload.VideoID)
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
		_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_SAVE_VIDEO, BATCH_FAILED, err.GetMessage())
		return
	}

	_ = s.batchRepo.UpdateUploadVideoJob(ctx, payload.VideoID, PROCESS_SAVE_VIDEO, BATCH_COMPLETED, "")
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
	metaProcessing, err := s.batchRepo.GetUploadVideoBatch(ctx, videoID)
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
		var metadata GistQuizMetadata
		if err := json.Unmarshal(action.Metadata, &metadata); err != nil {
			return nil, errors.InternalWrap("failed to parse gist quiz metadata", err)
		}
		// Fallback for legacy data
		if len(metadata.Attempts) == 0 {
			var legacy struct {
				QuizAttempts []GistQuizAttempt `json:"quiz_attempts"`
			}
			_ = json.Unmarshal(action.Metadata, &legacy)
			if len(legacy.QuizAttempts) > 0 {
				metadata.Attempts = legacy.QuizAttempts
			}
		}

		return &StartQuizResponse{
			ActionID: action.ID,
			VideoID:  videoID,
			UserID:   userID,
			GistQuiz: metadata.GistQuiz,
			Attempts: metadata.Attempts,
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
	metadata := GistQuizMetadata{
		Attempts: []GistQuizAttempt{},
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
		Attempts: metadata.Attempts,
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
		var metadata RetellStoryMetadata
		if err := json.Unmarshal(action.Metadata, &metadata); err != nil {
			return nil, errors.InternalWrap("failed to parse retell metadata", err)
		}
		// Fallback for legacy data
		if len(metadata.Attempts) == 0 {
			var legacy struct {
				RetellAttempts []RetellAttempt `json:"retell_attempts"`
			}
			_ = json.Unmarshal(action.Metadata, &legacy)
			if len(legacy.RetellAttempts) > 0 {
				metadata.Attempts = legacy.RetellAttempts
			}
		}

		return &StartRetellResponse{
			ActionID:    action.ID,
			VideoID:     videoID,
			UserID:      userID,
			RetellStory: metadata.RetellStory,
			Attempts:    metadata.Attempts,
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
	metadata := RetellStoryMetadata{
		Attempts: []RetellAttempt{},
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
		Attempts:    metadata.Attempts,
	}, nil
}

// SubmitGistQuiz handles the submission and scoring of a gist quiz.
func (s *VideoService) SubmitGistQuiz(ctx context.Context, input SubmitGistQuizInput) (*GistQuizAttempt, *errors.AppError) {
	// 1. Get existing action by videoID, userID, and type
	action, exists, err := s.videoRepo.GetActionByUserID(ctx, input.VideoID, input.UserID, "submit_quiz")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NotFound("quiz action not found for this video")
	}

	var metadata GistQuizMetadata
	if err := json.Unmarshal(action.Metadata, &metadata); err != nil {
		return nil, errors.InternalWrap("failed to parse quiz metadata", err)
	}

	// Fallback for legacy data
	if len(metadata.Attempts) == 0 {
		var legacy struct {
			QuizAttempts []GistQuizAttempt `json:"quiz_attempts"`
		}
		_ = json.Unmarshal(action.Metadata, &legacy)
		if len(legacy.QuizAttempts) > 0 {
			metadata.Attempts = legacy.QuizAttempts
		}
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
	metadata.Attempts = append(metadata.Attempts, attempt)

	// Sort by date (desc) to keep 3 latest
	sort.Slice(metadata.Attempts, func(i, j int) bool {
		return metadata.Attempts[i].SubmittedAt.After(metadata.Attempts[j].SubmittedAt)
	})

	// Keep only latest 3
	if len(metadata.Attempts) > 3 {
		metadata.Attempts = metadata.Attempts[:3]
	}

	metadataJSON, _ := json.Marshal(metadata)

	if err := s.videoRepo.UpdateQuizAction(ctx, action.ID, metadataJSON); err != nil {
		return nil, err
	}

	return &attempt, nil
}

// SubmitRetellStory handles the submission and AI evaluation of a retell story.
func (s *VideoService) SubmitRetellStory(ctx context.Context, input SubmitRetellPayload) (*RetellAttempt, *errors.AppError) {
	// 1. Create batch processing
	_, err := s.batchRepo.CreateEvaluateRetellBatch(ctx, input.AttemptID)
	if err != nil {
		return nil, err
	}

	// 2. Get media URL
	audioURL, err := s.fileRepo.GetMediaURL(input.AudioR2Path)
	if err != nil {
		return nil, err
	}

	return &RetellAttempt{
		AttemptID:   input.AttemptID,
		AudioURL:    audioURL,
		MimeType:    input.AudioType,
		Transcript:  "", // Update after process audio
		RetellScore: 0,  // Update after process AI
		SubmittedAt: time.Now().UTC(),
	}, nil
}

// Worker: ProcessEvaluateRetel
func (s *VideoService) ProcessEvaluateRetel(ctx context.Context, payload SubmitRetellPayload) {
	// 1. Get existing action by videoID, userID, and type
	action, exists, err := s.videoRepo.GetActionByUserID(ctx, payload.VideoID, payload.UserID, "submit_retell")
	if err != nil || !exists {
		return
	}

	var metadata RetellStoryMetadata
	if err := json.Unmarshal(action.Metadata, &metadata); err != nil {
		return
	}

	// 2. Process audio
	_ = s.batchRepo.UpdateEvaluateRetellJob(ctx, payload.AttemptID, PROCESS_UPLOAD_RETELL_AUDIO, BATCH_PROCESSING, "")
	tempWav, err := s.fileRepo.CreateTempFile(payload.AudioFile, payload.AudioWavPath)
	if err != nil {
		_ = s.batchRepo.UpdateEvaluateRetellJob(ctx, payload.AttemptID, PROCESS_UPLOAD_RETELL_AUDIO, BATCH_FAILED, err.GetMessage())
		return
	}

	// Defer close and remove temp file
	defer func() {
		tempWav.Close()
		os.Remove(tempWav.Name())
	}()

	transcript, err := s.aiRepo.GenerateVideoTranscript(ctx, tempWav.Name(), payload.Language)
	if err != nil {
		_ = s.batchRepo.UpdateEvaluateRetellJob(ctx, payload.AttemptID, PROCESS_UPLOAD_RETELL_AUDIO, BATCH_FAILED, err.GetMessage())
		return
	}

	if err := s.fileRepo.ConvertAudioToM4A(ctx, tempWav.Name(), payload.AudioM4aPath); err != nil {
		_ = s.batchRepo.UpdateEvaluateRetellJob(ctx, payload.AttemptID, PROCESS_UPLOAD_RETELL_AUDIO, BATCH_FAILED, err.GetMessage())
		return
	}
	defer os.Remove(payload.AudioM4aPath)

	audioURL, err := s.fileRepo.UploadReaderToR2(ctx, payload.AudioM4aPath, payload.AudioR2Path, payload.AudioType)
	if err != nil {
		_ = s.batchRepo.UpdateEvaluateRetellJob(ctx, payload.AttemptID, PROCESS_UPLOAD_RETELL_AUDIO, BATCH_FAILED, err.GetMessage())
		return
	}
	_ = s.batchRepo.UpdateEvaluateRetellJob(ctx, payload.AttemptID, PROCESS_UPLOAD_RETELL_AUDIO, BATCH_COMPLETED, "")

	// 4. AI Evaluation
	_ = s.batchRepo.UpdateEvaluateRetellJob(ctx, payload.AttemptID, PROCESS_EVALUATE_RETEL, BATCH_PROCESSING, "")
	eval, err := s.aiRepo.EvaluateRetellStory(ctx, transcript.Text, metadata.RetellStory.KeyPoints)
	if err != nil {
		_ = s.batchRepo.UpdateEvaluateRetellJob(ctx, payload.AttemptID, PROCESS_EVALUATE_RETEL, BATCH_FAILED, err.GetMessage())
		return
	}
	_ = s.batchRepo.UpdateEvaluateRetellJob(ctx, payload.AttemptID, PROCESS_EVALUATE_RETEL, BATCH_COMPLETED, "")

	// 5. Create attempt
	_ = s.batchRepo.UpdateEvaluateRetellJob(ctx, payload.AttemptID, PROCESS_SAVE_RETEL, BATCH_PROCESSING, "")
	attempt := RetellAttempt{
		AttemptID:        payload.AttemptID,
		AudioURL:         audioURL,
		MimeType:         payload.AudioType,
		Transcript:       transcript.Text,
		RetellScore:      eval.Score,
		MatchesKeyPoints: eval.MatchesKeyPoints,
		RetellAnalysis:   eval.Analysis,
		SubmittedAt:      time.Now().UTC(),
	}

	// 6. Update metadata
	metadata.Attempts = append(metadata.Attempts, attempt)

	// Sort by date (desc) to keep 3 latest
	sort.Slice(metadata.Attempts, func(i, j int) bool {
		return metadata.Attempts[i].SubmittedAt.After(metadata.Attempts[j].SubmittedAt)
	})

	// Keep only latest 3
	if len(metadata.Attempts) > 3 {
		metadata.Attempts = metadata.Attempts[:3]
	}

	metadataJSON, _ := json.Marshal(metadata)

	if err := s.videoRepo.UpdateQuizAction(ctx, action.ID, metadataJSON); err != nil {
		_ = s.batchRepo.UpdateEvaluateRetellJob(ctx, payload.AttemptID, PROCESS_SAVE_RETEL, BATCH_FAILED, err.GetMessage())
		return
	}
	_ = s.batchRepo.UpdateEvaluateRetellJob(ctx, payload.AttemptID, PROCESS_SAVE_RETEL, BATCH_COMPLETED, "")

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

	// Use pointers to modify the original slice elements
	answerMap := map[int]*QuizAnswer{}
	for i := range answers {
		answerMap[answers[i].QuizID] = &answers[i]
	}

	// Question weights by ID: Q1=30%, Q2=30%, Q3=40%
	weights := map[int]float64{
		1: 30.0,
		2: 30.0,
		3: 40.0,
	}

	var total float64
	for _, quiz := range questions {
		ans, ok := answerMap[quiz.ID]
		if !ok {
			continue
		}

		weight, hasWeight := weights[quiz.ID]
		if !hasWeight {
			// Fallback: distribute evenly
			weight = 100.0 / float64(len(questions))
		}

		var qScore float64
		switch quiz.Type {

		// Q2: single_choice — all-or-nothing, no special condition
		case "single_choice":
			correct := ""
			for _, opt := range quiz.Options {
				if opt.IsCorrect {
					correct = opt.ID
					break
				}
			}
			if len(ans.OptionIDs) == 1 && ans.OptionIDs[0] == correct {
				qScore = weight
			} else {
				qScore = 0
			}

		// Q1: multiple_response — per-choice gain/loss
		//   choice_value = weight / total_correct_choices
		//   each correct pick: +choice_value
		//   each wrong pick:   -choice_value
		case "multiple_response":
			correctSet := map[string]struct{}{}
			for _, opt := range quiz.Options {
				if opt.IsCorrect {
					correctSet[opt.ID] = struct{}{}
				}
			}
			totalCorrect := float64(len(correctSet))
			if totalCorrect == 0 {
				qScore = 0
				break
			}
			choiceValue := weight / totalCorrect
			for _, id := range ans.OptionIDs {
				if _, ok := correctSet[id]; ok {
					qScore += choiceValue
				} else {
					qScore -= choiceValue
				}
			}

		// Q3: ordering — check by position, partial credit per correct position
		//   position_value = weight / total_positions
		//   each position where user order matches correct order: +position_value
		case "ordering":
			if len(quiz.CorrectOrder) == 0 {
				qScore = 0
				break
			}
			positionValue := weight / float64(len(quiz.CorrectOrder))
			for i := range quiz.CorrectOrder {
				if i < len(ans.Order) && ans.Order[i] == quiz.CorrectOrder[i] {
					qScore += positionValue
				}
			}
		}

		ans.Score = qScore
		total += qScore
	}

	return total
}
