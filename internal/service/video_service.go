package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/repository"
)

// quizSystemPrompt is the system prompt used to generate quiz content from a transcript.
const quizSystemPrompt = `# Role
You are an expert Educational Content Creator and Developer. Your task is to analyze the video transcript and generate a quiz and a checklist in a strict JSON format.

# Instructions
You must analyze the transcript and generate a quiz.

## CRITICAL STEP: THOUGHT PROCESS
Before generating the JSON, you must identify the chronological order of events for the "Sequence" question to ensure accuracy.
1. Identify 4 key events.
2. Verify their order in the transcript.
3. Only then, map them to the JSON output.

## Part 1: Gist Quiz (Total 4-5 Questions)
1.  **Context/Tone (2-3 Questions):**
    * category: "context"
    * type: "multiple_response"
    * Must have 2-3 correct options (set is_correct: true).
2.  **Objective (1 Question):**
    * category: "objective"
    * type: "single_choice"
    * Only 1 correct option.
3.  **Sequence (1 Question):**
    * category: "sequence"
    * type: "ordering"
    * Provide 4 events in options (shuffled/random order).
    * Provide the correct_order array containing the correct sequence of Option IDs (e.g., ["B", "A", "C", "D"]).

## Part 2: Retell Story
* Create a list of "Story Points" that cover the flow of the story which is designed to be retold by the user 4-5 items.

# Output Format (STRICT JSON)
Do not output any markdown text, introductory phrases, or code blocks. Output ONLY the raw JSON object.
Use the structure below:

{
  "gist_quiz": [
    {
      "id": 1,
      "category": "string (context | objective | sequence)",
      "type": "string (multiple_response | single_choice | ordering)",
      "question": "string",
      "options": [
        { "id": "A", "text": "string", "is_correct": true } // is_correct is null for ordering type
      ],
      "correct_order": ["string"] // null for non-ordering types
    }
  ],
  "retell_story": [
    { "id": 1, "point": "string" } // short and concise sentences
  ]
}

* Ensure the JSON is valid and parsable.
`

// VideoService handles video upload and processing.
type VideoService struct {
	learningRepo  repository.LearningItemRepository
	mediaRepo     repository.MediaItemRepository
	quizRepo      repository.QuizRepository
	r2Client      *client.CloudflareClient
	azureSpeech   *client.AzureSpeechClient
	whisperClient *client.AzureWhisperClient
	azureChat     *client.AzureChatClient
	geminiClient  *client.GeminiClient
	batchService  *BatchService
	log           zerolog.Logger
}

// NewVideoService creates a new VideoService.
func NewVideoService(
	learningRepo repository.LearningItemRepository,
	mediaRepo repository.MediaItemRepository,
	quizRepo repository.QuizRepository,
	r2Client *client.CloudflareClient,
	azureSpeech *client.AzureSpeechClient,
	whisperClient *client.AzureWhisperClient,
	azureChat *client.AzureChatClient,
	geminiClient *client.GeminiClient,
	batchService *BatchService,
	log zerolog.Logger,
) *VideoService {
	return &VideoService{
		learningRepo:  learningRepo,
		mediaRepo:     mediaRepo,
		quizRepo:      quizRepo,
		r2Client:      r2Client,
		azureSpeech:   azureSpeech,
		whisperClient: whisperClient,
		azureChat:     azureChat,
		geminiClient:  geminiClient,
		batchService:  batchService,
		log:           log,
	}
}

// VideoUploadResult is returned after a successful upload.
type VideoUploadResult struct {
	Video   *repository.LearningItem `json:"video"`
	BatchID string                   `json:"batch_id"`
	Status  string                   `json:"status"`
}

// GetVideo retrieves a video learning item by its ID.
func (s *VideoService) GetVideo(ctx context.Context, idStr string) (*repository.LearningItem, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, errors.Validation("invalid video ID")
	}

	video, err := s.learningRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errors.InternalWrap("failed to get video", err)
	}

	return video, nil
}

// GetVideoByBatchID retrieves a video learning item by its batch ID.
func (s *VideoService) GetVideoByBatchID(ctx context.Context, batchID string) (*repository.LearningItem, error) {
	items, err := s.learningRepo.GetByBatchID(ctx, batchID)
	if err != nil {
		return nil, errors.InternalWrap("failed to get video by batch ID", err)
	}
	if len(items) == 0 {
		return nil, errors.NotFound("video not found for batch ID")
	}
	// Assuming one video per batch for now
	return items[0], nil
}

// ProcessUpload handles the full video upload pipeline in PARALLEL:
// 1. Create LearningItem (processing)
// 2. Async A: Upload to R2 -> Create MediaItem
// 3. Async B (Optional): Upload Thumbnail to R2 -> Create MediaItem
// 4. Async C: Extract Audio -> Transcribe -> Generate Quiz -> Update LearningItem
func (s *VideoService) ProcessUpload(ctx context.Context, userID string, file multipart.File, language string, thumbnailFile multipart.File, thumbContentType string) (*VideoUploadResult, error) {
	// 1. Setup IDs and Paths
	videoID := uuid.New()
	batchID := uuid.New().String()
	inputPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_input.mp4", videoID))

	thumbInputPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_thumb_input", videoID))

	// 2. Initialize Batch in Redis
	customJobNames := []string{"video_upload", "thumbnail_upload", "transcript", "quiz"}
	_ = s.batchService.CreateBatchWithJobs(ctx, batchID, videoID.String(), customJobNames)

	// 3. Save uploaded files to temp
	if err := s.saveTempFile(inputPath, file); err != nil {
		os.Remove(inputPath) // Clean up immediately on failure
		_ = s.batchService.UpdateJob(ctx, batchID, "video_upload", "failed", err.Error())
		return nil, errors.InternalWrap("failed to save temp video file", err)
	}

	if err := s.saveTempFile(thumbInputPath, thumbnailFile); err != nil {
		os.Remove(inputPath)
		os.Remove(thumbInputPath)
		_ = s.batchService.UpdateJob(ctx, batchID, "video_upload", "failed", "failed to save temp thumbnail file")
		return nil, errors.InternalWrap("failed to save temp thumbnail file", err)
	}

	// 4. Create initial LearningItem in DB
	// We store batch_id in metadata to allow retrieval later
	metadata := map[string]interface{}{
		"batch_id":          batchID,
		"user_id":           userID,
		"processing_status": "processing",
	}
	metadataJSON, _ := json.Marshal(metadata)

	featureID := repository.NativeImmersion
	learningItem := &repository.LearningItem{
		FeatureID: &featureID,
		Content:   "",       // Will be populated with transcript later
		LangCode:  language, // Default, will be updated detection
		Details:   json.RawMessage("{}"),
		Tags:      json.RawMessage("[]"),
		Metadata:  metadataJSON,
		IsActive:  false, // Not active until processed
	}

	// We need to inject the ID we generated earlier or let the DB generate it.
	// The repository Create method allows passing the struct, but usually ID is generated by DB.
	// However, distinct from the previous implementation, we want to return the ID immediately.
	// Let's rely on the DB returning the ID, and we use that ID for the media item link.
	if err := s.learningRepo.Create(ctx, learningItem); err != nil {
		os.Remove(inputPath)
		if thumbInputPath != "" {
			os.Remove(thumbInputPath)
		}
		_ = s.batchService.UpdateJob(ctx, batchID, "video_upload", "failed", err.Error())
		return nil, errors.InternalWrap("failed to create learning item", err)
	}

	// Use the DB-generated ID
	videoID = learningItem.ID

	// 5. Spawn Async Jobs
	// We use a WaitGroup in a separate goroutine to manage cleanup of the temp file
	// strictly after BOTH jobs are done.
	go func() {
		// This outer goroutine ensures the main request returns immediately,
		// while the coordination happens in the background.

		var wg sync.WaitGroup
		wg.Add(3)

		// Job A1: Upload to R2
		go func() {
			defer wg.Done()
			s.processR2Upload(context.Background(), videoID, batchID, inputPath, userID)
		}()

		// Job A2: Upload Thumbnail to R2
		go func() {
			defer wg.Done()
			s.processR2ThumbnailUpload(context.Background(), videoID, batchID, thumbInputPath, thumbContentType, userID)
		}()

		// Job B: Transcribe & Quiz
		go func() {
			defer wg.Done()
			s.processTranscriptionAndQuiz(context.Background(), videoID, batchID, inputPath, language)
		}()

		// Wait for both to finish, then clean up temp file
		wg.Wait()
		os.Remove(inputPath)
		if thumbInputPath != "" {
			os.Remove(thumbInputPath)
		}

		// Update final batch status if needed (Redis service handles job updates)
	}()

	// Fetch the latest state of the learning item to return, as it might have been updated
	// by the goroutines (e.g., thumbnail_url in metadata).
	// This is a best effort to return the most up-to-date item at the time of the main function's return.
	latestItem, err := s.learningRepo.GetByID(ctx, learningItem.ID)
	if err != nil {
		s.log.Error().Err(err).Str("video_id", learningItem.ID.String()).Msg("Failed to fetch latest learning item after spawning async jobs")
		// If we can't fetch the latest, return the initial one, but log the error.
		latestItem = learningItem
	}

	return &VideoUploadResult{
		Video:   latestItem,
		BatchID: batchID,
		Status:  "processing",
	}, nil
}

// processR2Upload handles uploading the video file to R2 and creating the MediaItem.
func (s *VideoService) processR2Upload(ctx context.Context, videoID uuid.UUID, batchID, inputPath, userID string) {
	r2Key := fmt.Sprintf("videos/%s.mp4", videoID)
	videoURL, err := s.uploadToR2(ctx, r2Key, inputPath, "video/mp4")
	if err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to upload to R2")
		_ = s.batchService.UpdateJob(ctx, batchID, "video_upload", "failed", err.Error())
		// We should also fail the item in DB? For now, just mark the job.
		return
	}

	// Create MediaItem linked to LearningItem
	mediaMetadata := map[string]interface{}{
		"r2_key":       r2Key,
		"content_type": "video/mp4",
	}
	mediaMetadataJSON, _ := json.Marshal(mediaMetadata)

	mediaItem := &repository.MediaItem{
		FilePath:  videoURL,
		Metadata:  mediaMetadataJSON,
		CreatedBy: userID,
	}

	if err := s.mediaRepo.Create(ctx, mediaItem); err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to create media item")
		_ = s.batchService.UpdateJob(ctx, batchID, "video_upload", "failed", err.Error())
		return
	}

	// Update LearningItem media field was removed.
	// The schema change relies on media_items table which we populated above.
	// If we need to store the video URL in learning_items.details for convenience, we can do it here,
	// but typically we should rely on the join or secondary lookup.
	// For now, doing nothing is safe as the MediaItem is the source of truth.

	_ = s.batchService.UpdateJob(ctx, batchID, "video_upload", "completed", "")
	s.log.Info().Str("video_id", videoID.String()).Str("url", videoURL).Msg("R2 upload and MediaItem created")
}

// processR2ThumbnailUpload handles uploading the thumbnail file to R2 and updating LearningItem metadata.
func (s *VideoService) processR2ThumbnailUpload(ctx context.Context, videoID uuid.UUID, batchID, inputPath, contentType, userID string) {
	_ = s.batchService.UpdateJob(ctx, batchID, "thumbnail_upload", "processing", "")

	ext := ".jpg"
	if strings.Contains(contentType, "png") {
		ext = ".png"
	} else if strings.Contains(contentType, "webp") {
		ext = ".webp"
	}

	r2Key := fmt.Sprintf("thumbnails/%s%s", videoID, ext)
	thumbURL, err := s.uploadToR2(ctx, r2Key, inputPath, contentType)
	if err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to upload thumbnail to R2")
		_ = s.batchService.UpdateJob(ctx, batchID, "thumbnail_upload", "failed", err.Error())
		return
	}

	// Create MediaItem for thumbnail
	mediaMetadata := map[string]interface{}{
		"r2_key":       r2Key,
		"content_type": contentType,
	}
	mediaMetadataJSON, _ := json.Marshal(mediaMetadata)

	mediaItem := &repository.MediaItem{
		FilePath:  thumbURL,
		Metadata:  mediaMetadataJSON,
		CreatedBy: userID,
	}

	if err := s.mediaRepo.Create(ctx, mediaItem); err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to create thumbnail media item")
		_ = s.batchService.UpdateJob(ctx, batchID, "thumbnail_upload", "failed", err.Error())
		return
	}

	// Update LearningItem metadata with thumbnail_url
	item, err := s.learningRepo.GetByID(ctx, videoID)
	if err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to get learning item for thumbnail update")
		_ = s.batchService.UpdateJob(ctx, batchID, "thumbnail_upload", "failed", "db fetch failed")
		return
	}

	var currentMeta map[string]interface{}
	if len(item.Metadata) > 0 {
		_ = json.Unmarshal(item.Metadata, &currentMeta)
	} else {
		currentMeta = make(map[string]interface{})
	}

	currentMeta["thumbnail_url"] = thumbURL
	newMetaJSON, _ := json.Marshal(currentMeta)
	item.Metadata = newMetaJSON

	if err := s.learningRepo.Update(ctx, item); err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to update learning item with thumbnail_url")
		_ = s.batchService.UpdateJob(ctx, batchID, "thumbnail_upload", "failed", err.Error())
		return
	}

	_ = s.batchService.UpdateJob(ctx, batchID, "thumbnail_upload", "completed", "")
	s.log.Info().Str("video_id", videoID.String()).Str("url", thumbURL).Msg("R2 thumbnail upload and MediaItem created")
}

// processTranscriptionAndQuiz handles audio extraction, transcription, and quiz generation.
func (s *VideoService) processTranscriptionAndQuiz(ctx context.Context, videoID uuid.UUID, batchID, videoPath, language string) {
	// Clean up audio file specifically for this job
	audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_audio.wav", videoID))
	defer os.Remove(audioPath)

	// --- Transcript Job ---
	_ = s.batchService.UpdateJob(ctx, batchID, "transcript", "processing", "")

	// 1. Extract audio with FFmpeg
	if err := s.extractAudio(videoPath, audioPath); err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to extract audio")
		_ = s.batchService.UpdateJob(ctx, batchID, "transcript", "failed", err.Error())
		_ = s.batchService.UpdateJob(ctx, batchID, "quiz", "failed", "skipped: transcript failed")
		return
	}

	// 2. Transcribe with Whisper
	result, err := s.whisperClient.TranscribeFile(ctx, audioPath, language)
	if err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Whisper transcription failed")
		_ = s.batchService.UpdateJob(ctx, batchID, "transcript", "failed", err.Error())
		_ = s.batchService.UpdateJob(ctx, batchID, "quiz", "failed", "skipped: transcript failed")
		return
	}

	// Update LearningItem with transcript
	// We read the current item first to merge metadata
	item, err := s.learningRepo.GetByID(ctx, videoID)
	if err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to get learning item for update")
		// Continue anyway? No, strict failure.
		_ = s.batchService.UpdateJob(ctx, batchID, "transcript", "failed", "db fetch failed")
		return
	}

	// Update Fields
	item.Content = result.Text
	item.LangCode = result.Language

	// Convert transcript segments to map/struct for metadata storage
	// Store in `details` column
	detailsJSON, _ := json.Marshal(result)
	item.Details = detailsJSON

	// Merge with existing metadata
	var currentMeta map[string]interface{}
	if len(item.Metadata) > 0 {
		_ = json.Unmarshal(item.Metadata, &currentMeta)
	} else {
		currentMeta = make(map[string]interface{})
	}

	currentMeta["processing_status"] = "completed"

	newMetaJSON, _ := json.Marshal(currentMeta)
	item.Metadata = newMetaJSON
	item.IsActive = true // Activate item

	if err := s.learningRepo.Update(ctx, item); err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to update learning item with transcript")
		_ = s.batchService.UpdateJob(ctx, batchID, "transcript", "failed", err.Error())
		return
	}

	_ = s.batchService.UpdateJob(ctx, batchID, "transcript", "completed", "")
	s.log.Info().Str("video_id", videoID.String()).Msg("Transcript generation completed")

	// --- Quiz Job ---
	var quizSegments []repository.TranscriptSegment
	for _, ws := range result.Segments {
		quizSegments = append(quizSegments, repository.TranscriptSegment{
			Text:     ws.Text,
			Start:    ws.Start,
			Duration: ws.End - ws.Start,
		})
	}

	s.generateQuiz(ctx, videoID, batchID, quizSegments, item.LangCode)
}

// generateQuiz generates quiz, saves it to LearningItem (or linked tables).
func (s *VideoService) generateQuiz(ctx context.Context, videoID uuid.UUID, batchID string, segments []repository.TranscriptSegment, detectedLang string) {
	_ = s.batchService.UpdateJob(ctx, batchID, "quiz", "processing", "")

	// ... reuse existing quiz generation logic prompt ...
	// Build transcript text
	var sb strings.Builder
	for _, seg := range segments {
		sb.WriteString(seg.Text)
		sb.WriteString(" ")
	}
	transcriptText := strings.TrimSpace(sb.String())

	if transcriptText == "" {
		_ = s.batchService.UpdateJob(ctx, batchID, "quiz", "failed", "empty transcript")
		return
	}

	userMessage := fmt.Sprintf("Transcript:\n\"\"\"\n%s\n\"\"\"\n\nLanguage: %s", transcriptText, detectedLang)

	// Call AI
	responseText, _, err := s.callQuizAI(ctx, videoID, userMessage)
	if err != nil {
		_ = s.batchService.UpdateJob(ctx, batchID, "quiz", "failed", err.Error())
		return
	}

	// Clean response
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var quizContent repository.QuizContent
	if err := json.Unmarshal([]byte(responseText), &quizContent); err != nil {
		_ = s.batchService.UpdateJob(ctx, batchID, "quiz", "failed", "invalid JSON: "+err.Error())
		return
	}

	latestItem, err := s.learningRepo.GetByID(ctx, videoID)
	if err != nil {
		_ = s.batchService.UpdateJob(ctx, batchID, "quiz", "failed", "refetch parent item failed")
		return
	}

	// Create GistQuiz LearningItem
	gistQuizDetailsJSON, _ := json.Marshal(quizContent.GistQuiz)
	gistMeta := map[string]interface{}{
		"parent_id": videoID,
		"batch_id":  batchID,
	}
	gistMetaJSON, _ := json.Marshal(gistMeta)

	gistFeature := repository.GistQuiz
	gistItem := &repository.LearningItem{
		FeatureID: &gistFeature,
		Content:   "Gist Quiz",
		LangCode:  latestItem.LangCode,
		Details:   gistQuizDetailsJSON,
		Tags:      json.RawMessage("[]"),
		Metadata:  gistMetaJSON,
		IsActive:  true,
	}

	if err := s.learningRepo.Create(ctx, gistItem); err != nil {
		s.log.Error().Err(err).Str("parent_video_id", videoID.String()).Msg("Failed to create Gist Quiz learning item")
		_ = s.batchService.UpdateJob(ctx, batchID, "quiz", "failed", "failed to save gist quiz")
		return
	}

	// Create RetellStory LearningItem
	retellStoryDetailsJSON, _ := json.Marshal(quizContent.RetellStory)
	retellMeta := map[string]interface{}{
		"parent_id": videoID,
		"batch_id":  batchID,
	}
	retellMetaJSON, _ := json.Marshal(retellMeta)

	retellFeature := repository.RetellStory
	retellItem := &repository.LearningItem{
		FeatureID: &retellFeature,
		Content:   "Retell Story",
		LangCode:  latestItem.LangCode,
		Details:   retellStoryDetailsJSON,
		Tags:      json.RawMessage("[]"),
		Metadata:  retellMetaJSON,
		IsActive:  true,
	}

	if err := s.learningRepo.Create(ctx, retellItem); err != nil {
		s.log.Error().Err(err).Str("parent_video_id", videoID.String()).Msg("Failed to create Retell Story learning item")
		_ = s.batchService.UpdateJob(ctx, batchID, "quiz", "failed", "failed to save retell story")
		return
	}

	_ = s.batchService.UpdateJob(ctx, batchID, "quiz", "completed", "")
	s.log.Info().Str("video_id", videoID.String()).Msg("Quiz generated and split into separate learning items")
}

// callQuizAI tries Azure Chat first, then falls back to Gemini.
// Returns the response text, the provider name used, and any error.
func (s *VideoService) callQuizAI(ctx context.Context, videoID uuid.UUID, userMessage string) (string, string, error) {
	// Try Azure Chat first
	if s.azureChat != nil {
		responseText, err := s.azureChat.ChatCompletion(ctx, quizSystemPrompt, userMessage)
		if err == nil {
			return responseText, "azure", nil
		}
		s.log.Warn().Err(err).Str("video_id", videoID.String()).Msg("Azure Chat failed, falling back to Gemini")
	}

	// Fallback to Gemini
	if s.geminiClient != nil {
		// Gemini Chat takes a single message, so combine system prompt + user message
		fullPrompt := quizSystemPrompt + "\n" + userMessage
		responseText, err := s.geminiClient.Chat(ctx, fullPrompt)
		if err == nil {
			return responseText, "gemini", nil
		}
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Gemini fallback also failed")
		return "", "", fmt.Errorf("all AI providers failed: gemini: %w", err)
	}

	return "", "", fmt.Errorf("no AI provider configured for quiz generation")
}

// extractAudio uses FFmpeg to extract audio from a video file into WAV format
// suitable for Azure Speech (16kHz, mono, PCM S16LE).
func (s *VideoService) extractAudio(videoPath, audioPath string) error {
	cmd := exec.Command("ffmpeg",
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
		s.log.Error().
			Err(err).
			Str("ffmpeg_output", string(output)).
			Msg("FFmpeg audio extraction failed")
		return fmt.Errorf("ffmpeg audio extraction: %w", err)
	}

	return nil
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

// uploadToR2 reads a file from disk and uploads it to Cloudflare R2.
func (s *VideoService) uploadToR2(ctx context.Context, key, filePath string, contentType string) (string, error) {
	if s.r2Client == nil {
		return "", fmt.Errorf("cloudflare R2 client not configured")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read output file: %w", err)
	}

	url, err := s.r2Client.UploadR2Object(ctx, key, data, contentType)
	if err != nil {
		return "", fmt.Errorf("upload to R2: %w", err)
	}

	return url, nil
}
