package service

import (
	"context"

	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/errors"
)

// AIService provides AI-related functionality.
type AIService struct {
	openaiClient *client.OpenAIClient
	geminiClient *client.GeminiClient
}

// NewAIService creates a new AI service.
func NewAIService(
	geminiClient *client.GeminiClient,
) *AIService {
	return &AIService{
		geminiClient: geminiClient,
	}
}

// Chat sends a chat message to the specified AI provider.
func (s *AIService) Chat(ctx context.Context, message, provider string) (string, error) {
	switch provider {
	case "openai":
		if s.openaiClient == nil {
			return "", errors.New(errors.ErrAIService, "OpenAI client not configured")
		}
		return s.openaiClient.Chat(ctx, message)

	case "gemini":
		if s.geminiClient == nil {
			return "", errors.New(errors.ErrAIService, "Gemini client not configured")
		}
		return s.geminiClient.Chat(ctx, message)

	default:
		// Default to OpenAI if available, otherwise Gemini
		if s.openaiClient != nil {
			return s.openaiClient.Chat(ctx, message)
		}
		if s.geminiClient != nil {
			return s.geminiClient.Chat(ctx, message)
		}
		return "", errors.New(errors.ErrAIService, "no AI provider configured")
	}
}

// Complete generates a completion for the given prompt.
func (s *AIService) Complete(ctx context.Context, prompt, provider string) (string, error) {
	switch provider {
	case "openai":
		if s.openaiClient == nil {
			return "", errors.New(errors.ErrAIService, "OpenAI client not configured")
		}
		return s.openaiClient.Complete(ctx, prompt)

	case "gemini":
		if s.geminiClient == nil {
			return "", errors.New(errors.ErrAIService, "Gemini client not configured")
		}
		return s.geminiClient.Complete(ctx, prompt)

	default:
		// Default to OpenAI if available, otherwise Gemini
		if s.openaiClient != nil {
			return s.openaiClient.Complete(ctx, prompt)
		}
		if s.geminiClient != nil {
			return s.geminiClient.Complete(ctx, prompt)
		}
		return "", errors.New(errors.ErrAIService, "no AI provider configured")
	}
}

// ChatStream streams chat responses from the specified AI provider.
func (s *AIService) ChatStream(ctx context.Context, message, provider string, onChunk func(string) error) error {
	switch provider {
	case "openai":
		if s.openaiClient == nil {
			return errors.New(errors.ErrAIService, "OpenAI client not configured")
		}
		return s.openaiClient.ChatStream(ctx, message, onChunk)

	case "gemini":
		if s.geminiClient == nil {
			return errors.New(errors.ErrAIService, "Gemini client not configured")
		}
		return s.geminiClient.ChatStream(ctx, message, onChunk)

	default:
		return errors.New(errors.ErrAIService, "provider not specified for streaming")
	}
}
