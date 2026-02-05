package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/errors"
)

const (
	// Redis key prefix for speaking reply results
	speakingReplyKeyPrefix = "speaking:reply:"
	// TTL for reply results in Redis
	replyTTL = 60 * time.Second
	// Default timeout for BLPOP waiting
	defaultReplyTimeout = 10 * time.Second
)

// AiProcessingResult is the result struct stored in Redis and returned to client.
type AiProcessingResult struct {
	AiText   string `json:"ai_text"`
	AudioURL string `json:"audio_url"`
}

// AnalyzeResult is returned immediately from the Analyze endpoint.
type AnalyzeResult struct {
	RequestID  string  `json:"request_id"`
	Transcript string  `json:"transcript"`
	Score      float64 `json:"score"`
}

// SpeakingService handles the 2-step async voice chat flow.
type SpeakingService struct {
	azureClient  *client.AzureSpeechClient
	geminiClient *client.GeminiClient
	redisClient  *client.RedisClient
	log          zerolog.Logger
}

// NewSpeakingService creates a new Speaking service.
func NewSpeakingService(
	azureClient *client.AzureSpeechClient,
	geminiClient *client.GeminiClient,
	redisClient *client.RedisClient,
	log zerolog.Logger,
) *SpeakingService {
	return &SpeakingService{
		azureClient:  azureClient,
		geminiClient: geminiClient,
		redisClient:  redisClient,
		log:          log,
	}
}

// AnalyzeSpeaking processes audio, returns immediate transcript, and spawns AI processing.
// This is the PRODUCER side of the async pattern.
func (s *SpeakingService) AnalyzeSpeaking(ctx context.Context, audioData []byte) (*AnalyzeResult, error) {
	if s.azureClient == nil {
		return nil, errors.New(errors.ErrAIService, "Azure Speech client not configured")
	}
	if s.redisClient == nil {
		return nil, errors.New(errors.ErrAIService, "Redis client not configured")
	}

	// Generate unique request ID
	requestID := fmt.Sprintf("req_%s", uuid.New().String()[:8])

	// Step 1: Call Azure STT to get transcript and score immediately
	// Using empty reference text - we just want the transcript
	result, err := s.azureClient.AnalyzeVocabAudio(ctx, audioData, "", "en-US")
	if err != nil {
		return nil, errors.Wrap(errors.ErrAIService, "failed to analyze audio", err)
	}

	// Extract transcript and score from Azure response
	transcript := ""
	var score float64 = 0.0

	if displayText, ok := result["DisplayText"].(string); ok {
		transcript = displayText
	}

	// Try to get pronunciation score from NBest
	if nbest, ok := result["NBest"].([]interface{}); ok && len(nbest) > 0 {
		if first, ok := nbest[0].(map[string]interface{}); ok {
			if pronScore, ok := first["PronScore"].(float64); ok {
				score = pronScore
			}
		}
	}

	s.log.Info().
		Str("request_id", requestID).
		Str("transcript", transcript).
		Float64("score", score).
		Msg("Audio analyzed, spawning AI processing goroutine")

	// Step 2: Fire-and-Forget goroutine for AI processing
	// This runs in background while we return immediately to the client
	go s.processAiReply(requestID, transcript)

	return &AnalyzeResult{
		RequestID:  requestID,
		Transcript: transcript,
		Score:      score,
	}, nil
}

// processAiReply is the background goroutine that:
// 1. Calls Gemini for AI chat response
// 2. (Mock) Calls TTS to synthesize audio
// 3. Pushes result to Redis via RPUSH
func (s *SpeakingService) processAiReply(requestID, transcript string) {
	ctx := context.Background()
	redisKey := speakingReplyKeyPrefix + requestID

	s.log.Debug().
		Str("request_id", requestID).
		Str("transcript", transcript).
		Msg("Starting AI processing in background")

	// Step 1: Call Gemini for AI response
	var aiText string
	if s.geminiClient != nil && transcript != "" {
		prompt := fmt.Sprintf("You are a helpful language learning assistant. The user said: \"%s\". Respond naturally and helpfully in 1-2 sentences.", transcript)
		response, err := s.geminiClient.Chat(ctx, prompt)
		if err != nil {
			s.log.Error().Err(err).Str("request_id", requestID).Msg("Gemini chat failed")
			aiText = "I'm sorry, I couldn't process your message. Please try again."
		} else {
			aiText = response
		}
	} else {
		// Fallback mock response
		aiText = fmt.Sprintf("I heard you say: \"%s\". That's great!", transcript)
	}

	// Step 2: Mock TTS - in production, this would call Azure TTS
	// For now, return a placeholder URL
	audioURL := fmt.Sprintf("https://storage.example.com/tts/%s.mp3", requestID)

	// Step 3: Create result and push to Redis
	result := AiProcessingResult{
		AiText:   aiText,
		AudioURL: audioURL,
	}

	// RPUSH the result to Redis list
	// The consumer (GetReply) will BLPOP this key to get the result
	if err := s.redisClient.RPush(ctx, redisKey, result); err != nil {
		s.log.Error().Err(err).Str("request_id", requestID).Msg("Failed to push result to Redis")
		return
	}

	// Set TTL on the key so it expires after 60 seconds
	if err := s.redisClient.SetExpiry(ctx, redisKey, replyTTL); err != nil {
		s.log.Error().Err(err).Str("request_id", requestID).Msg("Failed to set Redis key expiry")
	}

	s.log.Info().
		Str("request_id", requestID).
		Str("ai_text", aiText).
		Msg("AI processing complete, result pushed to Redis")
}

// GetReply waits for AI processing result using BLPOP.
// This is the CONSUMER side of the async pattern.
// Returns ErrTimeout if no result within timeout duration.
func (s *SpeakingService) GetReply(ctx context.Context, requestID string) (*AiProcessingResult, error) {
	if s.redisClient == nil {
		return nil, errors.New(errors.ErrAIService, "Redis client not configured")
	}

	redisKey := speakingReplyKeyPrefix + requestID

	s.log.Debug().
		Str("request_id", requestID).
		Dur("timeout", defaultReplyTimeout).
		Msg("Waiting for AI reply via BLPOP")

	// BLPOP blocks until a value is available or timeout expires
	// This is the key mechanism for the async pattern - the consumer
	// waits here while the producer (background goroutine) processes
	data, err := s.redisClient.BLPop(ctx, defaultReplyTimeout, redisKey)
	if err != nil {
		if err == redis.Nil {
			// Timeout - no result within 10 seconds
			s.log.Warn().Str("request_id", requestID).Msg("BLPOP timeout - no reply available")
			return nil, errors.New(errors.ErrTimeout, "AI reply not ready, please try again")
		}
		return nil, errors.Wrap(errors.ErrDatabase, "failed to get reply from Redis", err)
	}

	// Parse the JSON result
	var result AiProcessingResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errors.Wrap(errors.ErrInternal, "failed to parse AI reply", err)
	}

	s.log.Info().
		Str("request_id", requestID).
		Str("ai_text", result.AiText).
		Msg("AI reply retrieved successfully")

	return &result, nil
}
