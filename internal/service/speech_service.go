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

// AnalyzeAudio orchestrates audio analysis.
func (s *SpeechService) AnalyzeAudio(ctx context.Context, audioData []byte, rererenceText string) (map[string]interface{}, error) {
	if s.azureClient == nil {
		return nil, errors.New(errors.ErrAIService, "Azure Speech client not configured")
	}

	return s.azureClient.AnalyzeAudio(ctx, audioData, rererenceText)
}
