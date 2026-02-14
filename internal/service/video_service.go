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

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/repository"
)

// VideoService handles video upload and processing.
type VideoService struct {
	repo        repository.VideoRepository
	r2Client    *client.CloudflareClient
	azureSpeech *client.AzureSpeechClient
	log         zerolog.Logger
}

// NewVideoService creates a new VideoService.
func NewVideoService(
	repo repository.VideoRepository,
	r2Client *client.CloudflareClient,
	azureSpeech *client.AzureSpeechClient,
	log zerolog.Logger,
) *VideoService {
	return &VideoService{
		repo:        repo,
		r2Client:    r2Client,
		azureSpeech: azureSpeech,
		log:         log,
	}
}

// VideoUploadResult is returned after a successful upload.
type VideoUploadResult struct {
	Video *repository.Video `json:"video"`
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
	inputPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_input.mp4", videoID))

	// NOTE: Do NOT defer os.Remove here — the background goroutine needs the file.
	// Cleanup is handled inside processSubtitles.

	// Step 1: Save uploaded file to temp
	if err := s.saveTempFile(inputPath, file); err != nil {
		os.Remove(inputPath) // Clean up on early failure
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
		return nil, errors.InternalWrap("failed to create video record", err)
	}

	// Step 3: Upload to R2
	r2Key := fmt.Sprintf("videos/%s.mp4", videoID)
	videoURL, err := s.uploadToR2(ctx, r2Key, inputPath)
	if err != nil {
		os.Remove(inputPath)
		_ = s.repo.UpdateStatus(ctx, video.ID, "failed", "")
		return nil, errors.InternalWrap("failed to upload video to storage", err)
	}

	// Step 4: Update DB record with URL and "ready" status
	if err := s.repo.UpdateStatus(ctx, video.ID, "ready", videoURL); err != nil {
		os.Remove(inputPath)
		return nil, errors.InternalWrap("failed to update video record", err)
	}

	video.VideoURL = videoURL
	video.Status = "ready"

	s.log.Info().
		Str("video_id", video.ID.String()).
		Str("user_id", userID).
		Str("video_url", videoURL).
		Msg("Video upload completed, starting subtitle processing")

	// Step 5: Spawn async subtitle processing goroutine
	// The goroutine owns the temp file from this point — it handles cleanup.
	go s.processSubtitles(video.ID, inputPath)

	return &VideoUploadResult{Video: video}, nil
}

// processSubtitles runs in a background goroutine:
// extract audio via FFmpeg → transcribe via Azure Speech → update DB.
func (s *VideoService) processSubtitles(videoID uuid.UUID, videoPath string) {
	// CRITICAL: cleanup temp files when done
	defer os.Remove(videoPath)

	audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_audio.wav", videoID))
	defer os.Remove(audioPath)

	ctx := context.Background()

	// Step 1: Extract audio with FFmpeg
	if err := s.extractAudio(videoPath, audioPath); err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to extract audio")
		_ = s.repo.UpdateTranscript(ctx, videoID, nil, nil, "", "failed")
		return
	}

	// Step 2: Try primary language (en-US)
	result, err := s.azureSpeech.RecognizeFromFile(ctx, audioPath, "en-US")
	if err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Azure STT failed for en-US")
		_ = s.repo.UpdateTranscript(ctx, videoID, nil, nil, "", "failed")
		return
	}

	if result != nil {
		segments := azureResponseToSegments(result)
		rawJSON, _ := json.Marshal(result)
		_ = s.repo.UpdateTranscript(ctx, videoID, segments, rawJSON, "en-US", "completed")
		s.log.Info().
			Str("video_id", videoID.String()).
			Str("language", "en-US").
			Str("display_text", result.DisplayText).
			Int("word_count", len(segments)).
			Msg("Subtitle generation completed")
		return
	}

	// Step 3: Fallback — try Thai (th-TH)
	result, err = s.azureSpeech.RecognizeFromFile(ctx, audioPath, "th-TH")
	if err != nil {
		s.log.Error().Err(err).Str("video_id", videoID.String()).Msg("Azure STT failed for th-TH")
		_ = s.repo.UpdateTranscript(ctx, videoID, nil, nil, "", "failed")
		return
	}

	if result != nil {
		segments := azureResponseToSegments(result)
		rawJSON, _ := json.Marshal(result)
		_ = s.repo.UpdateTranscript(ctx, videoID, segments, rawJSON, "th-TH", "completed")
		s.log.Info().
			Str("video_id", videoID.String()).
			Str("language", "th-TH").
			Str("display_text", result.DisplayText).
			Int("word_count", len(segments)).
			Msg("Subtitle generation completed")
		return
	}

	// Step 4: No language matched
	_ = s.repo.UpdateTranscript(ctx, videoID, nil, nil, "", "wrong_language")
	s.log.Warn().
		Str("video_id", videoID.String()).
		Msg("No speech recognized in any candidate language")
}

// azureResponseToSegments converts Azure STT word-level data to TranscriptSegments.
func azureResponseToSegments(resp *client.AzureResponse) []repository.TranscriptSegment {
	// 1. แปลงเวลาทั้งหมดจาก Ticks เป็น Seconds
	totalStart := float64(resp.Offset) / 10000000.0
	totalDuration := float64(resp.Duration) / 10000000.0

	// 2. กรณีที่ Azure ส่ง Words มาให้ (โชคดี)
	if len(resp.NBest) > 0 && len(resp.NBest[0].Words) > 0 {
		words := resp.NBest[0].Words
		segments := make([]repository.TranscriptSegment, len(words))
		for i, w := range words {
			segments[i] = repository.TranscriptSegment{
				Text:     w.Word,
				Start:    float64(w.Offset) / 10000000.0,
				Duration: float64(w.Duration) / 10000000.0,
			}
		}
		return segments
	}

	// 3. กรณีที่ Azure ไม่ส่ง Words มา (ซึ่งเจอประจำใน REST API) -> ใช้การเฉลี่ยเวลา (Linear Interpolation)
	displayText := resp.DisplayText
	if len(resp.NBest) > 0 {
		displayText = resp.NBest[0].Display // ใช้ Display text ที่สวยงามกว่า
	}

	// แยกคำด้วยช่องว่าง
	wordList := strings.Fields(displayText)
	if len(wordList) == 0 {
		return []repository.TranscriptSegment{}
	}

	segments := make([]repository.TranscriptSegment, len(wordList))

	// คำนวณเวลาต่อ 1 คำ (แบบเฉลี่ย)
	durationPerWord := totalDuration / float64(len(wordList))

	for i, word := range wordList {
		segments[i] = repository.TranscriptSegment{
			Text:     word,
			Start:    totalStart + (float64(i) * durationPerWord),
			Duration: durationPerWord,
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
