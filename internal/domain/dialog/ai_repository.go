package dialog

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/pkg/errors"
)

const dialogGenerationPrompt = `You are an expert language-learning dialogue designer.

Generate a realistic dialogue guide as valid JSON only.
Do not include markdown, commentary, or code fences.

Requirements:
- Keep the language natural and useful for learners.
- Respect the requested topic, description, language, and level.
- Generate 3 to 5 relevant tags if none are provided.
- Keep the speech script concise and level-appropriate.
- Keep objectives actionable and easy to follow.

Output schema:
{
  "description": "string",
  "level": "string",
  "tags": ["string"],
  "image_prompt": "string",
  "speech_mode": {
    "situation": "string",
    "script": [
      {
        "speaker": "User or AI",
        "text": "string"
      }
    ]
  },
  "chat_mode": {
    "situation": "string",
    "objectives": {
      "requirements": ["string"],
      "persuasion": ["string"],
      "constraints": ["string"]
    }
  }
}`

// AIRepository generates dialog content from the LLM.
type AIRepository interface {
	GenerateDialog(ctx context.Context, payload GenerateDialogPayload) (*DialogDetails, *errors.AppError)
}

type dialogueGuideResponse struct {
	Description string          `json:"description"`
	Level       string          `json:"level"`
	Tags        []string        `json:"tags"`
	ImagePrompt string          `json:"image_prompt"`
	SpeechMode  json.RawMessage `json:"speech_mode"`
	ChatMode    json.RawMessage `json:"chat_mode"`
}

type aiRepository struct {
	chatGPT *client.AzureChatGPTClient
}

// NewAIRepository creates a new dialog AI repository.
func NewAIRepository(chatGPT *client.AzureChatGPTClient) AIRepository {
	return &aiRepository{chatGPT: chatGPT}
}

// GenerateDialog creates structured dialog content from the configured LLM.
func (r *aiRepository) GenerateDialog(ctx context.Context, payload GenerateDialogPayload) (*DialogDetails, *errors.AppError) {
	if r.chatGPT == nil {
		return nil, errors.Internal("dialog AI client not configured")
	}

	userMessage := buildDialogUserPrompt(payload)
	raw, err := r.chatGPT.ChatCompletion(ctx, dialogGenerationPrompt, userMessage)
	if err != nil {
		return nil, err
	}

	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var parsed dialogueGuideResponse
	if err := json.Unmarshal([]byte(clean), &parsed); err != nil {
		return nil, errors.InternalWrap("failed to parse generated dialog", err)
	}

	if parsed.Description == "" {
		parsed.Description = payload.Description
	}
	if parsed.Level == "" {
		parsed.Level = payload.Level
	}
	if len(parsed.Tags) == 0 {
		parsed.Tags = payload.Tags
	}

	return &DialogDetails{
		Topic:       payload.Topic,
		Description: parsed.Description,
		Language:    payload.Language,
		Level:       parsed.Level,
		Tags:        parsed.Tags,
		ImagePrompt: parsed.ImagePrompt,
		SpeechMode:  parsed.SpeechMode,
		ChatMode:    parsed.ChatMode,
	}, nil
}

func buildDialogUserPrompt(payload GenerateDialogPayload) string {
	var b strings.Builder

	b.WriteString("Topic: ")
	b.WriteString(payload.Topic)
	b.WriteString("\nDescription: ")
	b.WriteString(payload.Description)
	b.WriteString("\nLanguage: ")
	b.WriteString(payload.Language)
	b.WriteString("\nLevel: ")
	b.WriteString(payload.Level)
	b.WriteString("\nTags: ")

	if len(payload.Tags) == 0 {
		b.WriteString("generate relevant tags")
	} else {
		b.WriteString(strings.Join(payload.Tags, ", "))
	}

	return b.String()
}
