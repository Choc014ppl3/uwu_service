package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/errors"
)

// AIService provides AI-related functionality.
type AIService struct {
	geminiClient      *client.GeminiClient
	cloudflareClient  *client.CloudflareClient
	azureSpeechClient *client.AzureSpeechClient
}

// NewAIService creates a new AI service.
func NewAIService(
	geminiClient *client.GeminiClient,
	cloudflareClient *client.CloudflareClient,
	azureSpeechClient *client.AzureSpeechClient,
) *AIService {
	return &AIService{
		geminiClient:      geminiClient,
		cloudflareClient:  cloudflareClient,
		azureSpeechClient: azureSpeechClient,
	}
}

// Chat sends a chat message to the specified AI provider.
func (s *AIService) Chat(ctx context.Context, message, provider string) (string, error) {
	switch provider {
	case "gemini":
		if s.geminiClient == nil {
			return "", errors.New(errors.ErrAIService, "Gemini client not configured")
		}
		return s.geminiClient.Chat(ctx, message)

	default:
		// Default to Gemini if available
		if s.geminiClient != nil {
			return s.geminiClient.Chat(ctx, message)
		}
		return "", errors.New(errors.ErrAIService, "no AI provider configured")
	}
}

// Complete generates a completion for the given prompt.
func (s *AIService) Complete(ctx context.Context, prompt, provider string) (string, error) {
	switch provider {
	case "gemini":
		if s.geminiClient == nil {
			return "", errors.New(errors.ErrAIService, "Gemini client not configured")
		}
		return s.geminiClient.Complete(ctx, prompt)

	default:
		// Default to Gemini if available
		if s.geminiClient != nil {
			return s.geminiClient.Complete(ctx, prompt)
		}
		return "", errors.New(errors.ErrAIService, "no AI provider configured")
	}
}

// ChatStream streams chat responses from the specified AI provider.
func (s *AIService) ChatStream(ctx context.Context, message, provider string, onChunk func(string) error) error {
	switch provider {
	case "gemini":
		if s.geminiClient == nil {
			return errors.New(errors.ErrAIService, "Gemini client not configured")
		}
		return s.geminiClient.ChatStream(ctx, message, onChunk)

	default:
		return errors.New(errors.ErrAIService, "provider not specified for streaming")
	}
}

// GenerateScenarioReq defines the request for creating a scenario.
type GenerateScenarioReq struct {
	Topic      string `json:"topic"`       // e.g., "Ordering Coffee"
	NativeLang string `json:"native_lang"` // "th"
	TargetLang string `json:"target_lang"` // "zh-CN"
	AIGender   string `json:"ai_gender"`   // "female"
	UserGender string `json:"user_gender"` // "male"
}

// DialogueItem represents a single line in the conversation.
type DialogueItem struct {
	Speaker  string `json:"speaker"`        // "ai" or "user"
	Text     string `json:"text,omitempty"` // AI's dialogue line
	Task     string `json:"task,omitempty"` // User's specific task with blanks
	AudioURL string `json:"audio_url,omitempty"`
}

// ScenarioResponse defines the structure of the AI-generated scenario.
type ScenarioResponse struct {
	ScenarioID  string         `json:"scenario_id"`
	Topic       string         `json:"topic"`
	Description string         `json:"description"`
	ImagePrompt string         `json:"image_prompt"`
	ImageURL    string         `json:"image_url"`
	Script      []DialogueItem `json:"script"`
}

// GenerateScenario generates a roleplay scenario.
func (s *AIService) GenerateScenario(ctx context.Context, req GenerateScenarioReq) (*ScenarioResponse, error) {
	if s.geminiClient == nil {
		return nil, errors.New(errors.ErrAIService, "Gemini client not configured")
	}

	// 1. Logic defaults
	duration := "20s-30s"
	userGender := req.UserGender
	if userGender == "" {
		userGender = "male"
	}
	aiGender := req.AIGender
	if aiGender == "" {
		aiGender = "female"
	}

	// 2. Construct Prompt
	systemPrompt := fmt.Sprintf(`
You are a creative scenario generator.
Create a roleplay scenario about "%s".
Native Language: %s
Target Language: %s
Duration: %s
User Gender: %s
AI Gender: %s

Output STRICTLY in raw JSON format (no markdown backticks).
Structure the JSON to match the following schema:
{
  "description": "Brief scenario description",
  "image_prompt": "A vivid prompt to generate a background image for this scene",
  "script": [
    {
      "speaker": "ai" or "user",
      "text": "The dialogue line (strictly for 'ai' speaker only)",
      "task": "Specific task for the user with blanks for target language practice (strictly for 'user' speaker only), e.g., 'Say: I would like a ____ please.'",
      "audio_url": "" 
    }
  ]
}
Ensure the script makes sense and creates a natural conversation flow.
`, req.Topic, req.NativeLang, req.TargetLang, duration, userGender, aiGender)

	// 3. Call Gemini
	respStr, err := s.geminiClient.Chat(ctx, systemPrompt)
	if err != nil {
		return nil, err
	}

	cleanResp := strings.TrimSpace(respStr)
	cleanResp = strings.TrimPrefix(cleanResp, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```")

	var tempResp struct {
		Description string         `json:"description"`
		ImagePrompt string         `json:"image_prompt"`
		Script      []DialogueItem `json:"script"`
	}
	if err := json.Unmarshal([]byte(cleanResp), &tempResp); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w. Raw: %s", err, cleanResp)
	}

	// WaitGroup for concurrent operations
	var wg sync.WaitGroup

	// 4. Trigger Image Generation (Async)
	scenarioID := uuid.New().String()
	imagePrompt := tempResp.ImagePrompt
	if imagePrompt == "" {
		imagePrompt = req.Topic
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Use a detached context for async operations
		bgCtx := context.Background()
		if err := s.generateScenarioImage(bgCtx, scenarioID, imagePrompt); err != nil {
			fmt.Printf("Failed to generate/upload image async: %v\n", err)
		}
	}()

	// 5. Generate Audio for AI lines (Concurrent)
	script := tempResp.Script

	for i := range script {
		item := &script[i]
		if item.Speaker == "ai" && item.Text != "" && s.azureSpeechClient != nil {
			wg.Add(1)
			go func(idx int, it *DialogueItem) {
				defer wg.Done()
				// Generate Audio
				voiceName := "en-US-AvaMultilingualNeural"                                // Default dynamic voice
				audioData, err := s.azureSpeechClient.Synthesize(ctx, it.Text, voiceName) // Use request ctx or bgCtx? Request ctx is safer for cancellation but might timeout if user disconnects.
				if err != nil {
					fmt.Printf("Failed to synthesize audio for %d: %v\n", idx, err)
					return
				}

				// Upload to Cloudflare
				if s.cloudflareClient != nil {
					key := fmt.Sprintf("audio/scenario-%s-%d.mp3", scenarioID, idx)
					url, err := s.cloudflareClient.UploadImage(ctx, key, audioData, "audio/mpeg")
					if err != nil {
						fmt.Printf("Failed to upload audio %d: %v\n", idx, err)
						return
					}
					it.AudioURL = url
				}
			}(i, item)
		}
	}

	wg.Wait()

	// Construct Image URL
	imageURL := ""
	if s.cloudflareClient != nil {
		imageURL = fmt.Sprintf("%s/image/scenario-%s.webp", s.cloudflareClient.PublicURL(), scenarioID)
	}

	// 6. Construct Response
	response := &ScenarioResponse{
		ScenarioID:  scenarioID,
		Topic:       req.Topic,
		Description: tempResp.Description,
		ImagePrompt: tempResp.ImagePrompt,
		Script:      script,
		ImageURL:    imageURL,
	}

	return response, nil
}

// generateScenarioImage handles real image generation and saving.
func (s *AIService) generateScenarioImage(ctx context.Context, id, prompt string) error {
	// 1. Generate Image
	imgData, err := s.geminiClient.GenerateImage(ctx, prompt)
	if err != nil {
		return fmt.Errorf("gemini generate image error: %w", err)
	}

	// 2. Upload to Cloudflare R2
	if s.cloudflareClient != nil {
		key := fmt.Sprintf("image/scenario-%s.webp", id)
		url, err := s.cloudflareClient.UploadImage(ctx, key, imgData, "image/webp")
		if err != nil {
			return fmt.Errorf("cloudflare upload error: %w", err)
		}
		fmt.Printf("Image uploaded to: %s\n", url)
	}

	return nil
}
