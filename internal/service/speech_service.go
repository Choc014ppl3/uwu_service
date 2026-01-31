package service

import (
	"context"

	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/errors"
)

// SpeechService provides speech analysis functionality.
type SpeechService struct {
	azureClient *client.AzureSpeechClient
}

// NewSpeechService creates a new Speech service.
func NewSpeechService(azureClient *client.AzureSpeechClient) *SpeechService {
	return &SpeechService{
		azureClient: azureClient,
	}
}

// AnalyzeVocabAudio orchestrates audio analysis for vocabulary.
func (s *SpeechService) AnalyzeVocabAudio(ctx context.Context, audioData []byte, referenceText string) (map[string]interface{}, error) {
	if s.azureClient == nil {
		return nil, errors.New(errors.ErrAIService, "Azure Speech client not configured")
	}

	return s.azureClient.AnalyzeVocabAudio(ctx, audioData, referenceText)
}

// AnalyzeShadowingAudio orchestrates audio analysis for shadowing.
func (s *SpeechService) AnalyzeShadowingAudio(ctx context.Context, audioData []byte, referenceText, language string) (map[string]interface{}, error) {
	if s.azureClient == nil {
		return nil, errors.New(errors.ErrAIService, "Azure Speech client not configured")
	}

	return s.azureClient.AnalyzeShadowingAudio(ctx, audioData, referenceText, language)
}
