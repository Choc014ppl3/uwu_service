package dialog

import (
	"context"
	"encoding/json"
	"fmt"
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
- Ensure the "chat_mode" context and objectives directly follow up on the conversation generated in the "speech_mode" script.

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

// submitChatPrompt builds the system prompt for the chat reply.
const submitChatPrompt = `You are an AI language learning conversational partner. Your role is to roleplay with the user in a specific situation to help them practice their language skills.

## Context & Persona
- Situation: %s
- You must stay in character at all times and respond naturally to the user's messages.

## Communication Constraints & Guidelines
Follow these rules strictly when formulating your response:
%s
%s

## User Objectives (Progress Tracking)
The user needs to accomplish the following objectives during this conversation:
%s

## Task & Output Format
Analyze the user's latest message based on the conversation history. 
1. Generate an appropriate, natural reply following the constraints.
2. Evaluate if the user's message successfully fulfills any of the pending "User Objectives".

You MUST respond strictly in the following JSON format:
{
  "reply_message": "Your conversational response here.",
  "completed_objectives_indexes": [0, 2],
  "feedback": "Optional short feedback or hint for the user (internal use, keep it empty if the user is doing well)."
}`

// AIRepository generates dialog content from the LLM.
type AIRepository interface {
	GenerateDialog(ctx context.Context, payload GenerateDialogPayload) (*DialogDetails, *errors.AppError)
	ChatReply(ctx context.Context, chatMode ChatMode, history []client.ChatMessage, userMessage string) (*ChatReplyResult, *errors.AppError)
}

// ChatReplyResult is the parsed AI response for chat mode.
type ChatReplyResult struct {
	ReplyMessage               string `json:"reply_message"`
	CompletedObjectivesIndexes []int  `json:"completed_objectives_indexes"`
	Feedback                   string `json:"feedback"`
}

type dialogueGuideResponse struct {
	Description string     `json:"description"`
	Level       string     `json:"level"`
	Tags        []string   `json:"tags"`
	ImagePrompt string     `json:"image_prompt"`
	SpeechMode  SpeechMode `json:"speech_mode"`
	ChatMode    ChatMode   `json:"chat_mode"`
}

// Speech Mode
type SpeechMode struct {
	Situation string `json:"situation"`
	Script    []struct {
		AudioURL *string `json:"audio_url"`
		Speaker  string  `json:"speaker"`
		Text     string  `json:"text"`
	} `json:"script"`
}

// Chat Mode
type ChatMode struct {
	Situation  string `json:"situation"`
	Objectives struct {
		Requirements []string `json:"requirements"`
		Persuasion   []string `json:"persuasion"`
		Constraints  []string `json:"constraints"`
	} `json:"objectives"`
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

// ChatReply sends a multi-turn chat request and parses the structured AI response.
func (r *aiRepository) ChatReply(ctx context.Context, chatMode ChatMode, history []client.ChatMessage, userMessage string) (*ChatReplyResult, *errors.AppError) {
	if r.chatGPT == nil {
		return nil, errors.Internal("dialog AI client not configured")
	}

	// Build system prompt
	systemPrompt := buildChatReplySystemPrompt(chatMode)

	// Build full message list: system + history + new user message
	messages := make([]client.ChatMessage, 0, len(history)+2)
	messages = append(messages, client.ChatMessage{Role: "system", Content: systemPrompt})
	messages = append(messages, history...)
	messages = append(messages, client.ChatMessage{Role: "user", Content: userMessage})

	raw, err := r.chatGPT.ChatCompletionMultiTurn(ctx, messages)
	if err != nil {
		return nil, err
	}

	// Clean and parse JSON response
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var result ChatReplyResult
	if parseErr := json.Unmarshal([]byte(clean), &result); parseErr != nil {
		return nil, errors.InternalWrap("failed to parse chat reply", parseErr)
	}

	return &result, nil
}

func buildChatReplySystemPrompt(chatMode ChatMode) string {
	// Build constraints list
	var constraints strings.Builder
	for i, c := range chatMode.Objectives.Constraints {
		constraints.WriteString(fmt.Sprintf("%d. %s\n", i+1, c))
	}

	// Build persuasion list
	var persuasion strings.Builder
	for i, p := range chatMode.Objectives.Persuasion {
		persuasion.WriteString(fmt.Sprintf("%d. %s\n", i+1, p))
	}

	// Build requirements list
	var requirements strings.Builder
	for i, r := range chatMode.Objectives.Requirements {
		requirements.WriteString(fmt.Sprintf("%d. [Index %d] %s\n", i+1, i, r))
	}

	return fmt.Sprintf(
		submitChatPrompt,
		chatMode.Situation,
		constraints.String(),
		persuasion.String(),
		requirements.String(),
	)
}
