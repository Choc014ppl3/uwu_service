package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
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

// GenerateScenarioReq defines the request for creating a scenario.
type GenerateScenarioReq struct {
	Topic      string `json:"topic"`       // e.g., "Ordering Coffee"
	Difficulty string `json:"difficulty"`  // "easy", "medium", "hard"
	TargetLang string `json:"target_lang"` // "zh-CN"
	NativeLang string `json:"native_lang"` // "th"
}

// DialogueItem represents a single line in the conversation.
type DialogueItem struct {
	Speaker string `json:"speaker"` // "ai" or "user"
	Text    string `json:"text"`    // Target Language (e.g., Chinese)
	Meaning string `json:"meaning"` // Native Language (e.g., Thai)
	Hint    string `json:"hint"`    // Hints for Medium mode
}

// ScenarioResponse defines the structure of the AI-generated scenario.
type ScenarioResponse struct {
	ScenarioID  string `json:"scenario_id"`
	Level       string `json:"level"`
	Description string `json:"description"`
	ImagePrompt string `json:"image_prompt"` // For generating background image later

	// For Hard Mode Only
	Objective   string `json:"objective,omitempty"`
	SuccessCond string `json:"success_condition,omitempty"`

	// For Easy & Medium Mode
	Script []DialogueItem `json:"script,omitempty"`
}

// GenerateScenario generates a roleplay scenario based on difficulty.
func (s *AIService) GenerateScenario(ctx context.Context, req GenerateScenarioReq) (*ScenarioResponse, error) {
	if s.geminiClient == nil {
		return nil, errors.New(errors.ErrAIService, "Gemini client not configured")
	}

	var systemPrompt string

	commonInstructions := fmt.Sprintf(`
You are a helpful language tutor. Create a roleplay scenario about "%s".
Target Language: %s
Native Language: %s
Output STRICTLY in raw JSON format (no markdown backticks).
Structure the JSON to match the following schema:
{
  "level": "%s",
  "description": "Brief scenario description in Native Language",
  "image_prompt": "A vivid prompt to generate a background image for this scene",
  "script": [ ...array of dialogue items... (only for easy/medium) ],
  "objective": "...", (only for hard)
  "success_condition": "..." (only for hard)
}
`, req.Topic, req.TargetLang, req.NativeLang, req.Difficulty)

	switch req.Difficulty {
	case "easy":
		systemPrompt = commonInstructions + `
For "easy" mode:
1. Generate a full conversation script (5-6 turns).
2. Each item in "script" must have:
   - "speaker": "ai" or "user"
   - "text": The line in Target Language.
   - "meaning": The translation in Native Language.
   - "hint": Empty string.
3. Do NOT include "objective" or "success_condition".
`
	case "medium":
		systemPrompt = commonInstructions + `
For "medium" mode:
1. Generate a full conversation script (5-6 turns).
2. Each item in "script" must have:
   - "speaker": "ai" or "user"
3. For "ai" speaker:
   - "text": Line in Target Language.
   - "meaning": Translation in Native Language.
4. For "user" speaker (The student):
   - "text": The CORRECT answer in Target Language (to be hidden).
   - "meaning": Translation in Native Language.
   - "hint": A helpful hint in Native Language (e.g. key vocab or grammar note) instead of the direct translation.
5. Do NOT include "objective" or "success_condition".
`
	case "hard":
		systemPrompt = commonInstructions + `
For "hard" mode:
1. Do NOT generate a "script". Return an empty list or omit it.
2. Provide a challenging "objective" in Native Language describing what the user needs to achieve.
3. Provide a "success_condition" in Native Language describing how to win the roleplay.
`
	default:
		return nil, errors.New(errors.ErrValidation, "invalid difficulty level")
	}

	// Call Gemini
	// Use Chat method from GeminiClient
	respStr, err := s.geminiClient.Chat(ctx, systemPrompt)
	if err != nil {
		return nil, err
	}

	// Strip potential Markdown backticks if Gemini ignores instructions
	cleanResp := strings.TrimSpace(respStr)
	cleanResp = strings.TrimPrefix(cleanResp, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```")

	var scenario ScenarioResponse
	if err := json.Unmarshal([]byte(cleanResp), &scenario); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w. Raw: %s", err, cleanResp)
	}

	// Post-processing
	scenario.ScenarioID = uuid.New().String()
	scenario.Level = req.Difficulty // Ensure consistency

	return &scenario, nil
}
