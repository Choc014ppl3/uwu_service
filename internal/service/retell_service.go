package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

const maxRetellAttempts = 3

const retellSystemPrompt = `You are evaluating a student's retelling of a lesson.
Compare their speech against specific mission points they need to cover.
For each mission point, determine if the student adequately covered it in their retelling.
A point is "covered" if the student mentions the key idea, even if not word-for-word.

Respond ONLY with valid JSON in this exact format:
{
  "found_point_ids": [1, 3],
  "feedback": "Your summary of how they did and what they missed."
}

found_point_ids should contain the IDs of mission points the student successfully covered.
feedback should be encouraging and specific about what was covered and what was missed.`

// RetellService handles retell check logic.
type RetellService struct {
	retellRepo    repository.RetellRepository
	r2Client      *client.CloudflareClient
	whisperClient *client.AzureWhisperClient
	geminiClient  *client.GeminiClient
	log           zerolog.Logger
}

// NewRetellService creates a new RetellService.
func NewRetellService(
	retellRepo repository.RetellRepository,
	r2Client *client.CloudflareClient,
	whisperClient *client.AzureWhisperClient,
	geminiClient *client.GeminiClient,
	log zerolog.Logger,
) *RetellService {
	return &RetellService{
		retellRepo:    retellRepo,
		r2Client:      r2Client,
		whisperClient: whisperClient,
		geminiClient:  geminiClient,
		log:           log,
	}
}

// --- Response types ---

// RetellAttemptResponse is returned after submitting a retell attempt.
type RetellAttemptResponse struct {
	SessionID       int     `json:"session_id"`
	AttemptNumber   int     `json:"attempt_number"`
	TotalPoints     int     `json:"total_points"`
	FoundPointIDs   []int   `json:"found_point_ids"`
	MissingPointIDs []int   `json:"missing_point_ids"`
	CollectedTotal  int     `json:"collected_total"`
	Score           float64 `json:"score"`
	Feedback        string  `json:"feedback"`
	Status          string  `json:"status"`
	AttemptsLeft    int     `json:"attempts_left"`
}

// RetellSessionStatus is returned for GET session status.
type RetellSessionStatus struct {
	SessionID       int                `json:"session_id"`
	LessonID        int                `json:"lesson_id"`
	Status          string             `json:"status"`
	AttemptCount    int                `json:"attempt_count"`
	AttemptsLeft    int                `json:"attempts_left"`
	CollectedPoints json.RawMessage    `json:"collected_point_ids"`
	TotalPoints     int                `json:"total_points"`
	Score           float64            `json:"score"`
	MissionPoints   []MissionPointInfo `json:"mission_points"`
}

// MissionPointInfo is a mission point with its collected status.
type MissionPointInfo struct {
	ID        int    `json:"id"`
	Content   string `json:"content"`
	Collected bool   `json:"collected"`
}

// SubmitAttempt processes a retell attempt: transcribe audio, evaluate with Gemini, update session.
func (s *RetellService) SubmitAttempt(ctx context.Context, userID string, lessonID int, audioFile io.Reader) (*RetellAttemptResponse, error) {
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, errors.New(errors.ErrValidation, "invalid user ID")
	}

	// 1. Get or create session
	session, err := s.retellRepo.GetOrCreateSession(ctx, parsedUserID, lessonID)
	if err != nil {
		return nil, errors.New(errors.ErrInternal, "failed to get/create session")
	}

	// Check attempt limit
	if session.AttemptCount >= maxRetellAttempts {
		return nil, errors.New(errors.ErrValidation, "maximum attempts reached (3/3). Please reset to try again.")
	}

	// 2. Load mission points
	allPoints, err := s.retellRepo.GetMissionPoints(ctx, lessonID)
	if err != nil || len(allPoints) == 0 {
		return nil, errors.New(errors.ErrNotFound, "no mission points found for this lesson")
	}

	// 3. Determine already collected point IDs
	var collectedIDs []int
	_ = json.Unmarshal(session.CollectedPointIDs, &collectedIDs)
	collectedSet := make(map[int]bool)
	for _, id := range collectedIDs {
		collectedSet[id] = true
	}

	// Find missing points
	var missingPoints []repository.RetellMissionPoint
	for _, p := range allPoints {
		if !collectedSet[p.ID] {
			missingPoints = append(missingPoints, p)
		}
	}

	if len(missingPoints) == 0 {
		return nil, errors.New(errors.ErrValidation, "all points already collected!")
	}

	// 4. Save audio to temp file
	tempAudio := filepath.Join(os.TempDir(), fmt.Sprintf("retell_%d_%d.webm", session.ID, session.AttemptCount+1))
	tempWAV := filepath.Join(os.TempDir(), fmt.Sprintf("retell_%d_%d.wav", session.ID, session.AttemptCount+1))
	defer os.Remove(tempAudio)
	defer os.Remove(tempWAV)

	audioData, err := io.ReadAll(audioFile)
	if err != nil {
		return nil, errors.New(errors.ErrInternal, "failed to read audio")
	}

	if err := os.WriteFile(tempAudio, audioData, 0644); err != nil {
		return nil, errors.New(errors.ErrInternal, "failed to save temp audio")
	}

	// 5. Convert to WAV for Whisper
	if err := convertToWAV(tempAudio, tempWAV); err != nil {
		s.log.Error().Err(err).Msg("FFmpeg conversion failed, trying raw audio")
		// Fallback: try using original audio directly
		tempWAV = tempAudio
	}

	// 6. Upload audio to R2
	r2Key := fmt.Sprintf("retell/%d/%d.wav", session.ID, session.AttemptCount+1)
	audioURL, err := s.r2Client.UploadR2Object(ctx, r2Key, audioData, "audio/wav")
	if err != nil {
		s.log.Error().Err(err).Msg("Failed to upload retell audio to R2")
		audioURL = "" // Non-fatal, continue
	}

	// 7. Transcribe with Whisper
	userTranscript, err := s.whisperClient.TranscribeFile(ctx, tempWAV, "")
	if err != nil {
		return nil, errors.New(errors.ErrInternal, "failed to transcribe audio: "+err.Error())
	}

	userText := userTranscript.Text
	if strings.TrimSpace(userText) == "" {
		return nil, errors.New(errors.ErrValidation, "could not detect any speech in the audio")
	}

	// 8. Load original video transcript
	originalTranscript, err := s.retellRepo.GetVideoTranscriptByLessonID(ctx, lessonID)
	if err != nil {
		s.log.Warn().Err(err).Msg("Could not load original transcript")
		originalTranscript = "(Original transcript unavailable)"
	}

	// 9. Build Gemini prompt
	var promptBuilder strings.Builder
	promptBuilder.WriteString(retellSystemPrompt)
	promptBuilder.WriteString("\n\nOriginal transcript:\n\"\"\"")
	promptBuilder.WriteString(originalTranscript)
	promptBuilder.WriteString("\"\"\"\n\nMission points to check:\n")
	for _, p := range missingPoints {
		promptBuilder.WriteString(fmt.Sprintf("- ID %d: %s\n", p.ID, p.Content))
	}
	promptBuilder.WriteString("\nStudent's retelling:\n\"\"\"")
	promptBuilder.WriteString(userText)
	promptBuilder.WriteString("\"\"\"")

	// 10. Call Gemini
	geminiResp, err := s.geminiClient.Chat(ctx, promptBuilder.String())
	if err != nil {
		return nil, errors.New(errors.ErrInternal, "AI evaluation failed: "+err.Error())
	}

	// 11. Parse Gemini response
	geminiResp = strings.TrimSpace(geminiResp)
	geminiResp = strings.TrimPrefix(geminiResp, "```json")
	geminiResp = strings.TrimPrefix(geminiResp, "```")
	geminiResp = strings.TrimSuffix(geminiResp, "```")
	geminiResp = strings.TrimSpace(geminiResp)

	var evalResult struct {
		FoundPointIDs []int  `json:"found_point_ids"`
		Feedback      string `json:"feedback"`
	}
	if err := json.Unmarshal([]byte(geminiResp), &evalResult); err != nil {
		s.log.Error().Err(err).Str("raw_response", geminiResp).Msg("Failed to parse Gemini response")
		evalResult.Feedback = "AI evaluation completed but response parsing failed."
		evalResult.FoundPointIDs = []int{}
	}

	// 12. Merge found points into collected set
	for _, id := range evalResult.FoundPointIDs {
		collectedSet[id] = true
	}
	var newCollectedIDs []int
	for id := range collectedSet {
		newCollectedIDs = append(newCollectedIDs, id)
	}

	// Calculate score
	score := float64(len(newCollectedIDs)) / float64(len(allPoints)) * 100
	newAttemptCount := session.AttemptCount + 1

	// Determine session status
	status := "in_progress"
	if len(newCollectedIDs) == len(allPoints) {
		status = "completed"
	} else if newAttemptCount >= maxRetellAttempts {
		status = "failed"
	}

	// 13. Update session
	collectedJSON, _ := json.Marshal(newCollectedIDs)
	if err := s.retellRepo.UpdateSession(ctx, session.ID, collectedJSON, score, newAttemptCount, status); err != nil {
		s.log.Error().Err(err).Msg("Failed to update retell session")
	}

	// 14. Save audio log
	foundJSON, _ := json.Marshal(evalResult.FoundPointIDs)
	if err := s.retellRepo.SaveAudioLog(ctx, session.ID, audioURL, userText, foundJSON, evalResult.Feedback); err != nil {
		s.log.Error().Err(err).Msg("Failed to save audio log")
	}

	// Build missing IDs for response
	var stillMissing []int
	for _, p := range allPoints {
		if !collectedSet[p.ID] {
			stillMissing = append(stillMissing, p.ID)
		}
	}

	s.log.Info().
		Int("session_id", session.ID).
		Int("lesson_id", lessonID).
		Int("attempt", newAttemptCount).
		Int("found_this_attempt", len(evalResult.FoundPointIDs)).
		Int("total_collected", len(newCollectedIDs)).
		Float64("score", score).
		Str("status", status).
		Msg("Retell attempt completed")

	return &RetellAttemptResponse{
		SessionID:       session.ID,
		AttemptNumber:   newAttemptCount,
		TotalPoints:     len(allPoints),
		FoundPointIDs:   evalResult.FoundPointIDs,
		MissingPointIDs: stillMissing,
		CollectedTotal:  len(newCollectedIDs),
		Score:           score,
		Feedback:        evalResult.Feedback,
		Status:          status,
		AttemptsLeft:    maxRetellAttempts - newAttemptCount,
	}, nil
}

// GetSessionStatus returns the current retell session status.
func (s *RetellService) GetSessionStatus(ctx context.Context, userID string, lessonID int) (*RetellSessionStatus, error) {
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, errors.New(errors.ErrValidation, "invalid user ID")
	}

	session, err := s.retellRepo.GetOrCreateSession(ctx, parsedUserID, lessonID)
	if err != nil {
		return nil, errors.New(errors.ErrInternal, "failed to get session")
	}

	allPoints, err := s.retellRepo.GetMissionPoints(ctx, lessonID)
	if err != nil {
		return nil, errors.New(errors.ErrInternal, "failed to load mission points")
	}

	// Parse collected IDs
	var collectedIDs []int
	_ = json.Unmarshal(session.CollectedPointIDs, &collectedIDs)
	collectedSet := make(map[int]bool)
	for _, id := range collectedIDs {
		collectedSet[id] = true
	}

	// Build mission point info
	pointInfos := make([]MissionPointInfo, len(allPoints))
	for i, p := range allPoints {
		pointInfos[i] = MissionPointInfo{
			ID:        p.ID,
			Content:   p.Content,
			Collected: collectedSet[p.ID],
		}
	}

	return &RetellSessionStatus{
		SessionID:       session.ID,
		LessonID:        lessonID,
		Status:          session.Status,
		AttemptCount:    session.AttemptCount,
		AttemptsLeft:    maxRetellAttempts - session.AttemptCount,
		CollectedPoints: session.CollectedPointIDs,
		TotalPoints:     len(allPoints),
		Score:           session.CurrentScore,
		MissionPoints:   pointInfos,
	}, nil
}

// ResetSession resets the retell session so the user can start fresh.
func (s *RetellService) ResetSession(ctx context.Context, userID string, lessonID int) (*RetellSessionStatus, error) {
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, errors.New(errors.ErrValidation, "invalid user ID")
	}

	_, err = s.retellRepo.ResetSession(ctx, parsedUserID, lessonID)
	if err != nil {
		return nil, errors.New(errors.ErrInternal, "failed to reset session")
	}

	// Return fresh status
	return s.GetSessionStatus(ctx, userID, lessonID)
}

// convertToWAV uses FFmpeg to convert audio to WAV format for Whisper.
func convertToWAV(inputPath, outputPath string) error {
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-vn",
		"-acodec", "pcm_s16le",
		"-ar", "16000",
		"-ac", "1",
		"-y",
		outputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %s: %w", string(output), err)
	}
	return nil
}
