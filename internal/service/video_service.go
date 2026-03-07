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
	"golang.org/x/text/cases"
	lang "golang.org/x/text/language"

	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/repository"
)

// contentAnalysisSystemPrompt is the unified system prompt used to generate details and quiz from a transcript.
const contentAnalysisSystemPrompt = `# Role
You are an expert Linguistic and Educational Content Analyzer. Your task is to analyze the description and generate content details and a quiz in a strict JSON format.

# Instructions
You must analyze the description and determine:
1. topic: Identify the main topic of the video based solely on the transcript. The topic should be concise (1 short sentence or a short phrase).
2. description: Generate a clear and summarizing the video content. The description must be based only on the transcript. Do not invent information that is not present in the transcript. Keep it 3-5 sentences long.
3. level: The estimated language proficiency level required to understand the description. You must use the official or most widely recognized standard framework specific to the identified language. For example:
    * For English: Use the CEFR scale (A1, A2, B1, B2, C1, C2).
    * For Chinese: Use the HSK scale (HSK1, HSK2, HSK3, HSK4, HSK5, HSK6).
    * For Japanese: Use the JLPT scale (N5, N4, N3, N2, N1).
    * For Spanish: Use the DELE scale (A1, A2, B1, B2, C1, C2).
    * For French: Use the DELF/DALF scale (A1, A2, B1, B2, C1, C2).
	* For Russian: Use the TORFL scale (TORFL1, TORFL2, TORFL3, TORFL4, TORFL5, TORFL6).
	* For Portuguese: Use the CAPLE scale (A1, A2, B1, B2, C1, C2).
4. tags: A list of 3-5 relevant topic or thematic tags for the video (e.g., ["travel", "food", "daily life"]).

## CRITICAL STEP: THOUGHT PROCESS FOR QUIZ
Before generating the JSON quiz, you must identify the chronological order of events for the "Sequence" question to ensure accuracy.
1. Identify 4 key events.
2. Verify their order in the description.
3. Only then, map them to the JSON output.

## Part 1: Gist Quiz 3 Questions
1.  **Context/Tone (1 Question):**
    * category: "context"
    * type: "multiple_response"
    * Must have 1-2 correct options (set is_correct: true).
2.  **Main Idea (1 Question):**
    * category: "main_idea"
    * type: "single_choice"
    * Only 1 correct option.
3.  **Sequence (1 Question):**
    * category: "sequence"
    * type: "ordering"
    * Provide 4 events in options (shuffled/random order).
    * Provide the correct_order array containing the correct sequence of Option IDs (e.g., ["B", "A", "C", "D"]).

## Part 2: Retell Story
Generate a concise example of how the user could retell the story based on the transcript following elements:
1. "retell_example": Create a concise, natural-sounding summary of the story. This serves as a model answer or a good example for a student to follow. It should use clear chronological order and appropriate transition words.
2. "key_points": Extract 3 to 5 essential plot points, main events, or key takeaways that the student MUST include in their retelling like in "retell_example" to be considered complete and accurate.

2. "key_points": Extract 3 to 5 essential plot points, main events, or key takeaways that the student MUST include in their retelling like in "retell_example" to be considered complete and accurate.

# Output Format (STRICT JSON)
Do not output any markdown text, introductory phrases, or code blocks. Output ONLY the raw JSON object.
Use the structure below:

{
  "topic": "string",
  "description": "string",
  "level": "string",
  "tags": ["string"],
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
  "retell_story": {
    "retell_example": "string",
	"key_points": ["string"] // 3-5 
  }
}

* Ensure the JSON is valid and parsable.
`

// VideoService handles video upload and processing.
type VideoService struct {
	learningRepo       repository.LearningItemRepository
	learningSourceRepo repository.LearningSourceRepository
	mediaRepo          repository.MediaItemRepository
	quizRepo           repository.QuizRepository
	r2Client           *client.CloudflareClient
	azureSpeech        *client.AzureSpeechClient
	whisperClient      *client.AzureWhisperClient
	azureChat          *client.AzureChatClient
	geminiClient       *client.GeminiClient
	batchService       *BatchService
	log                zerolog.Logger
}

// NewVideoService creates a new VideoService.
func NewVideoService(
	learningRepo repository.LearningItemRepository,
	learningSourceRepo repository.LearningSourceRepository,
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
		learningRepo:       learningRepo,
		learningSourceRepo: learningSourceRepo,
		mediaRepo:          mediaRepo,
		quizRepo:           quizRepo,
		r2Client:           r2Client,
		azureSpeech:        azureSpeech,
		whisperClient:      whisperClient,
		azureChat:          azureChat,
		geminiClient:       geminiClient,
		batchService:       batchService,
		log:                log,
	}
}

// VideoUploadResult is returned after a successful upload.
type VideoUploadResult struct {
	VideoID string `json:"video_id"`
	BatchID string `json:"batch_id"`
	Status  string `json:"status"`
}

// BatchImmersionResult contains all items generated for a batch.
type BatchImmersionResult struct {
	Video       *repository.LearningItem `json:"video"`
	GistQuiz    *repository.LearningItem `json:"gist_quiz,omitempty"`
	RetellStory *repository.LearningItem `json:"retell_story,omitempty"`
	BatchID     string                   `json:"batch_id"`
	Status      string                   `json:"status"`
}

type GeneratedVideoContent struct {
	Topic       string                         `json:"topic"`
	Description string                         `json:"description"`
	Language    string                         `json:"language"`
	Level       string                         `json:"level"`
	Tags        []string                       `json:"tags"`
	GistQuiz    []map[string]interface{}       `json:"gist_quiz"`
	RetellStory json.RawMessage                `json:"retell_story"`
	Segments    []repository.TranscriptSegment `json:"segments"`
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

// GetVideoPlaylist retrieves the video playlist filtered by new, saved, and done within the last 2 weeks.
func (s *VideoService) GetVideoPlaylist(ctx context.Context, userID string, status string, limit, offset int) ([]*repository.LearningItem, int, error) {
	items, total, err := s.learningRepo.GetVideoPlaylist(ctx, userID, status, limit, offset)
	if err != nil {
		return nil, 0, errors.InternalWrap("failed to retrieve video playlist", err)
	}
	return items, total, nil
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

// GetImmersionByBatchID retrieves all immersion learning items by its batch ID.
func (s *VideoService) GetImmersionByBatchID(ctx context.Context, batchID string) (*BatchImmersionResult, error) {
	items, err := s.learningRepo.GetByBatchID(ctx, batchID)
	if err != nil {
		return nil, errors.InternalWrap("failed to get items by batch ID", err)
	}
	if len(items) == 0 {
		return nil, errors.NotFound("items not found for batch ID")
	}

	result := &BatchImmersionResult{
		BatchID: batchID,
		Status:  "completed",
	}

	for _, item := range items {
		if item.FeatureID == nil {
			result.Video = item
		} else if *item.FeatureID == repository.GistQuiz {
			result.GistQuiz = item
		} else if *item.FeatureID == repository.RetellStory {
			result.RetellStory = item
		}
	}

	return result, nil
}

// ProcessUpload handles the full video upload pipeline in PARALLEL
func (s *VideoService) ProcessUpload(ctx context.Context, userID string, videoFile multipart.File, videoContentType string, thumbnailFile multipart.File, thumbContentType string, language string) (*VideoUploadResult, error) {
	// 1. Setup IDs and Paths
	videoID := uuid.New()
	batchID := uuid.New().String()
	videoInputPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_video_input.%s", videoID, strings.Split(videoContentType, "/")[1]))
	thumbInputPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_thumb_input.%s", videoID, strings.Split(thumbContentType, "/")[1]))

	// 2. Initialize Batch in Redis
	customJobNames := []string{"video_upload", "thumbnail_upload", "generate_transcripts", "generate_details"}
	_ = s.batchService.CreateBatchWithJobs(ctx, batchID, videoID.String(), customJobNames)

	// 3. Save uploaded files to temp
	if err := s.saveTempFile(videoInputPath, videoFile); err != nil {
		os.Remove(videoInputPath) // Clean up immediately on failure
		_ = s.batchService.UpdateJob(ctx, batchID, "video_upload", "failed", err.Error())
		return nil, errors.InternalWrap("failed to save temp video file", err)
	}

	if err := s.saveTempFile(thumbInputPath, thumbnailFile); err != nil {
		os.Remove(thumbInputPath)
		_ = s.batchService.UpdateJob(ctx, batchID, "thumbnail_upload", "failed", "failed to save temp thumbnail file")
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

	featureID := repository.NativeVideo
	learningItem := &repository.LearningItem{
		FeatureID: &featureID,
		Content:   "",       // Will be populated with transcript later
		Language:  language, // Default, will be updated detection
		Details:   json.RawMessage("{}"),
		Tags:      json.RawMessage("[]"),
		Metadata:  metadataJSON,
		IsActive:  false, // Not active until processed
	}

	if err := s.learningRepo.Create(ctx, learningItem); err != nil {
		os.Remove(videoInputPath)
		os.Remove(thumbInputPath)
		_ = s.batchService.UpdateJob(ctx, batchID, "video_upload", "failed", err.Error())
		_ = s.batchService.UpdateJob(ctx, batchID, "thumbnail_upload", "failed", err.Error())
		_ = s.batchService.UpdateJob(ctx, batchID, "generate_transcripts", "failed", err.Error())
		_ = s.batchService.UpdateJob(ctx, batchID, "generate_details", "failed", err.Error())
		return nil, errors.InternalWrap("failed to create learning item", err)
	}

	// Use the DB-generated ID
	videoID = learningItem.ID

	bgCtx := context.Background()

	// 5. Spawn Async Jobs
	// We use a WaitGroup in a separate goroutine to manage cleanup of the temp file
	// strictly after BOTH jobs are done.
	go func() {
		// This outer goroutine ensures the main request returns immediately,
		// while the coordination happens in the background.

		var wg sync.WaitGroup
		wg.Add(3)

		// Job A1: Upload Video to R2
		var videoURL string
		go func() {
			defer wg.Done()

			_ = s.batchService.UpdateJob(bgCtx, batchID, "video_upload", "processing", "")
			r2Path := fmt.Sprintf("videos/%s.%s", videoID, strings.Split(videoContentType, "/")[1])
			url, err := s.processR2Upload(bgCtx, videoID, batchID, videoInputPath, r2Path, videoContentType)
			if err != nil {
				_ = s.batchService.UpdateJob(bgCtx, batchID, "video_upload", "failed", err.Error())
				return
			}
			_ = s.batchService.UpdateJob(bgCtx, batchID, "video_upload", "completed", "")
			videoURL = url
		}()

		// Job A2: Upload Thumbnail to R2
		var thumbnailURL string
		go func() {
			defer wg.Done()

			_ = s.batchService.UpdateJob(bgCtx, batchID, "thumbnail_upload", "processing", "")
			r2Path := fmt.Sprintf("thumbnails/%s.%s", videoID, strings.Split(thumbContentType, "/")[1])
			url, err := s.uploadToR2(bgCtx, r2Path, thumbInputPath, thumbContentType)
			if err != nil {
				_ = s.batchService.UpdateJob(bgCtx, batchID, "thumbnail_upload", "failed", err.Error())
				return
			}
			_ = s.batchService.UpdateJob(bgCtx, batchID, "thumbnail_upload", "completed", "")
			thumbnailURL = url
		}()

		// Job B: Transcribe & Details
		var transcript GeneratedVideoContent
		var transcriptErr error
		go func() {
			defer wg.Done()

			_ = s.batchService.UpdateJob(bgCtx, batchID, "generate_transcripts", "processing", "")
			_ = s.batchService.UpdateJob(bgCtx, batchID, "generate_details", "processing", "")
			result, err := s.processTranscriptionAndDetails(bgCtx, videoID, batchID, videoInputPath, language)
			if err != nil {
				s.log.Error().Err(err).Msg("Transcription and detail generation failed or skipped.")
				transcriptErr = err
				return
			}
			transcript = result
		}()

		// Wait for all jobs to finish, then clean up temp files
		wg.Wait()
		defer os.Remove(videoInputPath)
		defer os.Remove(thumbInputPath)

		// Fetch the learning item for final update
		item, err := s.learningRepo.GetByID(bgCtx, videoID)
		if err != nil {
			s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to get learning item for final update")
			return
		}

		var currentMeta map[string]interface{}
		_ = json.Unmarshal(item.Metadata, &currentMeta)

		currentMeta["video_url"] = videoURL
		currentMeta["thumbnail_url"] = thumbnailURL

		// If transcription failed, mark item as failed and return
		if transcriptErr != nil {
			currentMeta["processing_status"] = "failed"
			currentMeta["error"] = transcriptErr.Error()
			newMetaJSON, _ := json.Marshal(currentMeta)
			item.Metadata = newMetaJSON
			item.IsActive = false
			_ = s.learningRepo.Update(bgCtx, item)
			return
		}

		item.Content = transcript.Topic
		item.Level = transcript.Level
		item.Language = transcript.Language

		// Build transcript text
		var sb strings.Builder
		for _, seg := range transcript.Segments {
			sb.WriteString(seg.Text)
			sb.WriteString(" ")
		}
		transcriptText := strings.TrimSpace(sb.String())

		currentMeta["processing_status"] = "completed"
		item.IsActive = true

		tagsB, _ := json.Marshal(transcript.Tags)
		item.Tags = tagsB
		detailsB, _ := json.Marshal(map[string]interface{}{
			"topic":        transcript.Topic,
			"language":     transcript.Language,
			"level":        transcript.Level,
			"description":  transcript.Description,
			"transcript":   transcriptText,
			"gist_quiz":    transcript.GistQuiz,
			"retell_story": transcript.RetellStory,
			"segments":     transcript.Segments,
		})
		item.Details = detailsB
		newMetaJSON, _ := json.Marshal(currentMeta)
		item.Metadata = newMetaJSON

		if err := s.learningRepo.Update(bgCtx, item); err != nil {
			s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to update learning item with video_url")
			return
		}

		_ = s.batchService.UpdateJob(bgCtx, batchID, "generate_details", "completed", "")
	}()

	return &VideoUploadResult{
		VideoID: videoID.String(),
		BatchID: batchID,
		Status:  "processing",
	}, nil
}

// processR2Upload handles uploading the video file to R2 and creating the MediaItem.
func (s *VideoService) processR2Upload(ctx context.Context, videoID uuid.UUID, batchID, inputPath, r2Path, mimeType string) (string, error) {
	videoURL, err := s.uploadToR2(ctx, r2Path, inputPath, mimeType)
	if err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to upload to R2")
		_ = s.batchService.UpdateJob(ctx, batchID, "video_upload", "failed", err.Error())
		// We should also fail the item in DB? For now, just mark the job.
		return "", err
	}

	return videoURL, nil
}

// processTranscriptionAndDetails handles audio extraction, transcription, and details generation.
func (s *VideoService) processTranscriptionAndDetails(ctx context.Context, videoID uuid.UUID, batchID, videoPath, language string) (GeneratedVideoContent, error) {
	audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_audio.wav", videoID))
	defer os.Remove(audioPath)

	if err := s.extractAudio(videoPath, audioPath); err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to extract audio")
		_ = s.batchService.UpdateJob(ctx, batchID, "generate_transcripts", "failed", err.Error())
		_ = s.batchService.UpdateJob(ctx, batchID, "generate_details", "failed", "skipped: generate details failed")
		return GeneratedVideoContent{}, err
	}

	transcript, err := s.whisperClient.TranscribeFile(ctx, audioPath, language)
	if err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Whisper transcription failed")
		_ = s.batchService.UpdateJob(ctx, batchID, "generate_transcripts", "failed", err.Error())
		_ = s.batchService.UpdateJob(ctx, batchID, "generate_details", "failed", "skipped: generate details failed")
		return GeneratedVideoContent{}, err
	}
	s.log.Info().Str("video_id", videoID.String()).Msg("Transcript generation completed")
	_ = s.batchService.UpdateJob(ctx, batchID, "generate_transcripts", "completed", "")

	var transcriptSegments []repository.TranscriptSegment
	for _, ws := range transcript.Segments {
		transcriptSegments = append(transcriptSegments, repository.TranscriptSegment{
			Text:     ws.Text,
			Start:    ws.Start,
			Duration: ws.End - ws.Start,
		})
	}

	generatedContent, err := s.generateContentInfo(ctx, videoID, batchID, transcriptSegments, transcript.Language)
	if err != nil {
		return GeneratedVideoContent{}, err
	}

	caser := cases.Title(lang.English)
	generatedContent.Language = caser.String(transcript.Language)
	generatedContent.Segments = transcriptSegments

	return generatedContent, nil
}

// generateContentInfo generates language, level, tags, gist_quiz, retell_story in one go.
func (s *VideoService) generateContentInfo(ctx context.Context, videoID uuid.UUID, batchID string, segments []repository.TranscriptSegment, detectedLang string) (GeneratedVideoContent, error) {
	// Build transcript text
	var sb strings.Builder
	for _, seg := range segments {
		sb.WriteString(seg.Text)
		sb.WriteString(" ")
	}
	transcriptText := strings.TrimSpace(sb.String())

	if transcriptText == "" {
		_ = s.batchService.UpdateJob(ctx, batchID, "generate_details", "failed", "empty transcript")
		return GeneratedVideoContent{}, errors.New(errors.ErrAIService, "empty transcript")
	}

	userMessage := fmt.Sprintf("Transcript:\n\"\"\"\n%s\n\"\"\"\n\nLanguage: %s", transcriptText, detectedLang)

	// Generate Video Details
	responseText, _, err := s.generateVideoDetails(ctx, videoID, userMessage, "gemini")
	if err != nil {
		_ = s.batchService.UpdateJob(ctx, batchID, "generate_details", "failed", err.Error())
		return GeneratedVideoContent{}, err
	}

	// Clean response
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var generatedContent GeneratedVideoContent
	if err := json.Unmarshal([]byte(responseText), &generatedContent); err != nil {
		_ = s.batchService.UpdateJob(ctx, batchID, "generate_details", "failed", "invalid JSON: "+err.Error())
		return generatedContent, err
	}

	return generatedContent, nil
}

// generateVideoDetails tries Azure Chat first, then falls back to Gemini.
// Returns the response text, the provider name used, and any error.
func (s *VideoService) generateVideoDetails(ctx context.Context, videoID uuid.UUID, userMessage, model string) (string, string, error) {
	// Gemini Chat
	if model == "gemini" && s.geminiClient != nil {
		// Gemini Chat takes a single message, so combine system prompt + user message
		fullPrompt := contentAnalysisSystemPrompt + "\n" + userMessage
		responseText, err := s.geminiClient.Chat(ctx, fullPrompt)
		if err == nil {
			return responseText, "gemini", nil
		}
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Gemini failed")
		return "", "", fmt.Errorf("all AI providers failed: gemini: %w", err)
	}

	// Azure Chat
	if model == "azure" && s.azureChat != nil {
		responseText, err := s.azureChat.ChatCompletion(ctx, contentAnalysisSystemPrompt, userMessage)
		if err == nil {
			return responseText, "azure", nil
		}
		s.log.Warn().Err(err).Str("video_id", videoID.String()).Msg("Azure Chat failed, falling back to Gemini")
	}

	return "", "", fmt.Errorf("no AI provider configured")
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
func (s *VideoService) uploadToR2(ctx context.Context, key, filePath, contentType string) (string, error) {
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
