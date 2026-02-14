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
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/repository"
)

// VideoService handles video upload and processing.
type VideoService struct {
	repo          repository.VideoRepository
	r2Client      *client.CloudflareClient
	azureSpeech   *client.AzureSpeechClient
	whisperClient *client.AzureWhisperClient
	batchService  *BatchService
	log           zerolog.Logger
}

// NewVideoService creates a new VideoService.
func NewVideoService(
	repo repository.VideoRepository,
	r2Client *client.CloudflareClient,
	azureSpeech *client.AzureSpeechClient,
	whisperClient *client.AzureWhisperClient,
	batchService *BatchService,
	log zerolog.Logger,
) *VideoService {
	return &VideoService{
		repo:          repo,
		r2Client:      r2Client,
		azureSpeech:   azureSpeech,
		whisperClient: whisperClient,
		batchService:  batchService,
		log:           log,
	}
}

// VideoUploadResult is returned after a successful upload.
type VideoUploadResult struct {
	Video   *repository.Video `json:"video"`
	BatchID string            `json:"batch_id"`
}

// GetVideo retrieves a video by its ID string.
func (s *VideoService) GetVideo(ctx context.Context, idStr string) (*repository.Video, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, errors.Validation("invalid video ID")
	}

	video, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errors.InternalWrap("failed to get video", err)
	}

	return video, nil
}

// ProcessUpload handles the full video upload pipeline:
// save to tmp → upload to R2 → save metadata to DB → spawn async subtitle job.
func (s *VideoService) ProcessUpload(ctx context.Context, userID string, file multipart.File) (*VideoUploadResult, error) {
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, errors.Validation("invalid user ID")
	}

	videoID := uuid.New()
	batchID := uuid.New().String()
	inputPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_input.mp4", videoID))

	// Create batch in Redis
	_ = s.batchService.CreateBatch(ctx, batchID, videoID.String(), userID)

	// Step 1: Save uploaded file to temp
	if err := s.saveTempFile(inputPath, file); err != nil {
		os.Remove(inputPath)
		_ = s.batchService.UpdateJob(ctx, batchID, "upload", "failed", err.Error())
		return nil, errors.InternalWrap("failed to save temp file", err)
	}

	// Step 2: Create initial DB record with "processing" status
	video := &repository.Video{
		UserID:   parsedUserID,
		VideoURL: "",
		Status:   "processing",
	}
	if err := s.repo.Create(ctx, video); err != nil {
		os.Remove(inputPath)
		_ = s.batchService.UpdateJob(ctx, batchID, "upload", "failed", err.Error())
		return nil, errors.InternalWrap("failed to create video record", err)
	}

	// Step 3: Upload to R2
	r2Key := fmt.Sprintf("videos/%s.mp4", videoID)
	videoURL, err := s.uploadToR2(ctx, r2Key, inputPath)
	if err != nil {
		os.Remove(inputPath)
		_ = s.repo.UpdateStatus(ctx, video.ID, "failed", "")
		_ = s.batchService.UpdateJob(ctx, batchID, "upload", "failed", err.Error())
		return nil, errors.InternalWrap("failed to upload video to storage", err)
	}

	// Step 4: Update DB record with URL and "ready" status
	if err := s.repo.UpdateStatus(ctx, video.ID, "ready", videoURL); err != nil {
		os.Remove(inputPath)
		_ = s.batchService.UpdateJob(ctx, batchID, "upload", "failed", err.Error())
		return nil, errors.InternalWrap("failed to update video record", err)
	}

	video.VideoURL = videoURL
	video.Status = "ready"

	// Mark upload job as completed
	_ = s.batchService.UpdateJob(ctx, batchID, "upload", "completed", "")

	s.log.Info().
		Str("video_id", video.ID.String()).
		Str("user_id", userID).
		Str("batch_id", batchID).
		Str("video_url", videoURL).
		Msg("Video upload completed, starting subtitle processing")

	// Step 5: Spawn async subtitle + quiz processing goroutine
	go s.processSubtitles(video.ID, inputPath, batchID)

	return &VideoUploadResult{Video: video, BatchID: batchID}, nil
}

// processSubtitles runs in a background goroutine:
// extract audio via FFmpeg → transcribe via Azure OpenAI Whisper → update DB → mock quiz.
func (s *VideoService) processSubtitles(videoID uuid.UUID, videoPath string, batchID string) {
	// CRITICAL: cleanup temp files when done
	defer os.Remove(videoPath)

	audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_audio.wav", videoID))
	defer os.Remove(audioPath)

	ctx := context.Background()

	// --- Transcript Job ---
	_ = s.batchService.UpdateJob(ctx, batchID, "transcript", "processing", "")

	// Step 1: Extract audio with FFmpeg
	if err := s.extractAudio(videoPath, audioPath); err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to extract audio")
		_ = s.repo.UpdateTranscript(ctx, videoID, nil, nil, "", "failed")
		_ = s.batchService.UpdateJob(ctx, batchID, "transcript", "failed", err.Error())
		return
	}

	// Step 2: Transcribe with Whisper (auto language detection)
	result, err := s.whisperClient.TranscribeFile(ctx, audioPath, "")
	if err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Whisper transcription failed")
		_ = s.repo.UpdateTranscript(ctx, videoID, nil, nil, "", "failed")
		_ = s.batchService.UpdateJob(ctx, batchID, "transcript", "failed", err.Error())
		return
	}

	// Step 3: Map response to segments and save
	segments := whisperResponseToSegments(result)
	rawJSON, _ := json.Marshal(result)
	detectedLang := result.Language
	if detectedLang == "" {
		detectedLang = "unknown"
	}

	_ = s.repo.UpdateTranscript(ctx, videoID, segments, rawJSON, detectedLang, "completed")
	_ = s.batchService.UpdateJob(ctx, batchID, "transcript", "completed", "")

	s.log.Info().
		Str("video_id", videoID.String()).
		Str("language", detectedLang).
		Int("word_count", len(segments)).
		Float64("duration_sec", result.Duration).
		Msg("Subtitle generation completed (Whisper)")

	// --- Quiz Job (mock) ---
	s.generateQuiz(ctx, videoID, batchID)
}

// generateQuiz is a mock quiz generation job.
// TODO: Replace with real quiz generation logic.
func (s *VideoService) generateQuiz(ctx context.Context, videoID uuid.UUID, batchID string) {
	_ = s.batchService.UpdateJob(ctx, batchID, "quiz", "processing", "")

	// Simulate quiz generation work
	time.Sleep(2 * time.Second)

	_ = s.batchService.UpdateJob(ctx, batchID, "quiz", "completed", "")

	s.log.Info().
		Str("video_id", videoID.String()).
		Str("batch_id", batchID).
		Msg("Quiz generation completed (mock)")
}

// whisperResponseToSegments converts Whisper word-level data to TranscriptSegments.
// Whisper returns start/end in seconds — no conversion needed.
func whisperResponseToSegments(resp *client.WhisperResponse) []repository.TranscriptSegment {
	if len(resp.Words) == 0 {
		// No word-level data — return full text as single segment
		if resp.Text != "" {
			return []repository.TranscriptSegment{{
				Text:     resp.Text,
				Start:    0,
				Duration: resp.Duration,
			}}
		}
		return []repository.TranscriptSegment{}
	}

	segments := make([]repository.TranscriptSegment, len(resp.Words))
	for i, w := range resp.Words {
		segments[i] = repository.TranscriptSegment{
			Text:     w.Word,
			Start:    w.Start,
			Duration: w.End - w.Start,
		}
	}
	return segments
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
