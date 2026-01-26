package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/errors"
)

// AIService provides AI-related functionality.
type AIService struct {
	geminiClient     *client.GeminiClient
	cloudflareClient *client.CloudflareClient
}

// NewAIService creates a new AI service.
func NewAIService(
	geminiClient *client.GeminiClient,
	cloudflareClient *client.CloudflareClient,
) *AIService {
	return &AIService{
		geminiClient:     geminiClient,
		cloudflareClient: cloudflareClient,
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
	Speaker   string   `json:"speaker"`         // "ai" or "user"
	Objective string   `json:"objective"`       // only for ai
	AudioURL  string   `json:"audio_url"`       // only for ai
	Context   string   `json:"context"`         // hint for user with must have word
	Vocabs    []string `json:"vocab,omitempty"` // [word1, word2] - mapped from mock vocabs
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

	// 1. Logic Mocks & Defaults
	duration := "20s-30s"
	userGender := req.UserGender
	if userGender == "" {
		userGender = "male"
	}
	aiGender := req.AIGender
	if aiGender == "" {
		aiGender = "female"
	}

	mockVocabs := s.generateMockVocabs()

	// 2. Construct Prompt
	systemPrompt := fmt.Sprintf(`
You are a creative scenario generator.
Create a roleplay scenario about "%s".
Native Language: %s
Target Language: %s
Duration: %s
User Gender: %s
AI Gender: %s

Must Have Vocabularies (User should use these): %v

Output STRICTLY in raw JSON format (no markdown backticks).
Structure the JSON to match the following schema:
{
  "description": "Brief scenario description",
  "image_prompt": "A vivid prompt to generate a background image for this scene",
  "script": [
    {
      "speaker": "ai" or "user",
      "objective": "The objective of this turn (empty if user)",
      "audio_url": "", (leave empty for now)
      "context": "Hint/Context for user, must include target vocab if applicable"
    }
  ]
}
Ensure the script makes sense and incorporates the vocabularies naturally in the user's context/hints.
`, req.Topic, req.NativeLang, req.TargetLang, duration, userGender, aiGender, mockVocabs)

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

	// 4. Trigger Image Generation (Async)
	scenarioID := uuid.New().String()
	imagePrompt := tempResp.ImagePrompt
	if imagePrompt == "" {
		imagePrompt = req.Topic
	}

	go func() {
		bgCtx := context.Background()
		if err := s.generateScenarioImage(bgCtx, scenarioID, imagePrompt); err != nil {
			fmt.Printf("Failed to generate/upload image async: %v\n", err)
		}
	}()

	// 5. Map Vocabularies
	for i := range tempResp.Script {
		item := &tempResp.Script[i]
		item.Vocabs = []string{}
		for _, vocab := range mockVocabs {
			lowerVocab := strings.ToLower(vocab)
			if strings.Contains(strings.ToLower(item.Context), lowerVocab) ||
				strings.Contains(strings.ToLower(item.Objective), lowerVocab) {
				item.Vocabs = append(item.Vocabs, vocab)
			}
		}
	}

	// Construct Image URL
	imageURL := ""
	if s.cloudflareClient != nil {
		imageURL = fmt.Sprintf("https://pub-d85099e9916143fcb172f661babc3497.r2.dev/image/scenario-%s.webp", scenarioID)
	} else {
		// Fallback for local
		imageURL = fmt.Sprintf("/images/scenario-%s.webp", scenarioID)
	}

	// 6. Construct Response
	response := &ScenarioResponse{
		ScenarioID:  scenarioID,
		Topic:       req.Topic,
		Description: tempResp.Description,
		ImagePrompt: tempResp.ImagePrompt,
		Script:      tempResp.Script,
		ImageURL:    imageURL,
	}

	return response, nil
}

// generateMockVocabs returns a 6-10 word mock list.
func (s *AIService) generateMockVocabs() []string {
	// For now, hardcoded list.
	return []string{
		"apple", "banana", "coffee", "please", "thank you",
		"where", "how much", "delicious", "check", "bill",
	}
}

// generateScenarioImage handles real image generation and saving.
func (s *AIService) generateScenarioImage(ctx context.Context, id, prompt string) error {
	// 1. Generate Image
	imgData, err := s.geminiClient.GenerateImage(ctx, prompt)
	if err != nil {
		return fmt.Errorf("gemini generate image error: %w", err)
	}

	// 2. Mock 'Convert' to webp (just use same data for now, assuming Gemini returned valid image bytes)
	// Upload to Cloudflare R2
	if s.cloudflareClient != nil {
		key := fmt.Sprintf("image/scenario-%s.webp", id)
		url, err := s.cloudflareClient.UploadImage(ctx, key, imgData, "image/webp")
		if err != nil {
			return fmt.Errorf("cloudflare upload error: %w", err)
		}
		fmt.Printf("Image uploaded to: %s\n", url)
	}

	// 3. Keep local save for debugging if needed (optional)
	dir := "images"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0755)
	}
	_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("scenario-%s.webp", id)), imgData, 0644)

	return nil
}
