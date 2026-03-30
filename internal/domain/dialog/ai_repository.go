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

Generate a realistic and coherent dialogue guide strictly following the provided requirements and output schema.

Return valid JSON only.
Do not include markdown, explanations, comments, or code fences.
Do not include any text before or after the JSON.
Ensure the output is properly formatted and fully parseable.

**Requirements:**
- Keep the language natural, clear, and useful for learners.  
- Follow the requested topic, description, target language, and proficiency level strictly.  

- Generate a new **description** that:
  - Is **1-2 sentences only**.
  - Uses a **friendly, natural, and slightly story-like tone** (engaging but not exaggerated).
  - Clearly summarizes the scenario, key interaction, and learning focus.
  - Is aligned with both *speech_mode* and *chat_mode*.
  - Does **not copy or directly paraphrase** the user-provided description.

- Enforce vocabulary and grammar appropriate to the specified level:
  - Use CEFR-aligned complexity (e.g., A1-A2: simple sentences, common words; B1-B2: more varied structures; C1+: nuanced and natural expressions).
  - Avoid vocabulary or sentence structures significantly above the target level.

- Generate 3-5 contextual tags only if none are provided:
  - Tags must reflect key themes, vocabulary, or real-life situations in the content.
  - Avoid generic labels such as language levels (e.g., A2, C1) or broad categories (e.g., English learning, Chinese learning).

- Keep the speech script concise, coherent, and appropriate for the specified level.  

- Ensure learning objectives are practical, actionable, and easy to follow.  

- Make sure the **chat_mode** context and objectives:
  - Directly and logically continue from the *speech_mode* conversation.
  - Feel like a natural next step (not a separate or unrelated scenario).

- Ensure all fields in the output schema are fully populated and consistent with each other:
  - No contradictions between description, tags, and scenarios.
  - Maintain a single coherent context across the entire output.

**Output schema:**
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
- **Pacing & Flow:** Ask ONLY ONE question at a time. Do not overwhelm the user with multiple questions or choices in a single message. Wait for the user's response before moving forward.
- **Natural Conversation:** Keep your responses short, natural, and conversational. Do not rush to complete the User Objectives all at once. Let the conversation flow naturally step-by-step.
%s
%s

## User Objectives (Progress Tracking)
The user needs to accomplish the following objectives during this conversation:
%s

## Task & Output Format
Analyze the user's latest message based on the conversation history. 
1. Generate an appropriate, natural reply following the constraints.
2. Evaluate if the user's message successfully fulfills any of the pending "User Objectives".
3. Formulate helpful feedback in the "suggestion" field based on their performance.

You MUST respond strictly in the following JSON format:
{
  "reply_message": "Your conversational response here.",
  "completed_objectives_indexes": [0, 2],
  "suggestion": "Helpful feedback. Provide a short grammar/vocabulary correction."
}`

// ReplyMessageResult is the parsed AI response for chat mode.
type ReplyMessageResult struct {
	ReplyMessage               string `json:"reply_message"`
	Suggestion                 string `json:"suggestion"`
	CompletedObjectivesIndexes []int  `json:"completed_objectives_indexes"`
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
	Situation string         `json:"situation"`
	Script    []SpeechScript `json:"script"`
}

// SpeechScript
type SpeechScript struct {
	Speaker    string      `json:"speaker"`
	Text       string      `json:"text"`
	AudioURL   *string     `json:"audio_url,omitempty"`
	Evaluation *Evaluation `json:"evaluation,omitempty"`
}

// Evaluation & EvaluationWord
type Evaluation struct {
	AccuracyScore     float64          `json:"accuracy_score"`
	FluencyScore      float64          `json:"fluency_score"`
	PronScore         float64          `json:"pron_score"`
	CompletenessScore float64          `json:"completeness_score"`
	DisplayText       string           `json:"display_text"`
	Duration          int              `json:"duration"`
	Words             []EvaluationWord `json:"words"`
}

type EvaluationWord struct {
	AccuracyScore float64 `json:"AccuracyScore"`
	Confidence    float64 `json:"Confidence"`
	Duration      int     `json:"Duration"`
	ErrorType     string  `json:"ErrorType"`
	Offset        int     `json:"Offset"`
	Word          string  `json:"Word"`
}

// Chat Mode & ChatObjective
type ChatMode struct {
	Situation  string        `json:"situation"`
	Objectives ChatObjective `json:"objectives"`
}

type ChatObjective struct {
	Requirements []string `json:"requirements"`
	Persuasion   []string `json:"persuasion"`
	Constraints  []string `json:"constraints"`
}

// AIRepository generates dialog content from the LLM.
type AIRepository interface {
	GenerateDialog(ctx context.Context, payload GenerateDialogPayload) (*DialogDetails, *errors.AppError)
	ReplyUserMessage(ctx context.Context, chatObjective ChatObjective, history []ChatMessage, situation, userMessage string) (*ReplyMessageResult, *errors.AppError)
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

// ReplyUserMessage sends a multi-turn chat request and parses the structured AI response.
func (r *aiRepository) ReplyUserMessage(ctx context.Context, chatObjective ChatObjective, history []ChatMessage, situation, userMessage string) (*ReplyMessageResult, *errors.AppError) {
	if r.chatGPT == nil {
		return nil, errors.Internal("dialog AI client not configured")
	}

	// Build system prompt
	systemPrompt := buildChatReplySystemPrompt(chatObjective, situation)

	// Build full message list: system + history + new user message
	messages := make([]client.ChatMessage, 0, len(history)+2)
	messages = append(messages, client.ChatMessage{Role: "system", Content: systemPrompt})
	for _, msg := range history {
		messages = append(messages, client.ChatMessage{Role: msg.Role, Content: msg.Content})
	}
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

	var result ReplyMessageResult
	if parseErr := json.Unmarshal([]byte(clean), &result); parseErr != nil {
		return nil, errors.InternalWrap("failed to parse chat reply", parseErr)
	}

	return &result, nil
}

func buildChatReplySystemPrompt(chatObjective ChatObjective, situation string) string {
	// Build constraints list
	var constraints strings.Builder
	for i, c := range chatObjective.Constraints {
		constraints.WriteString(fmt.Sprintf("%d. %s\n", i+1, c))
	}

	// Build persuasion list
	var persuasion strings.Builder
	for i, p := range chatObjective.Persuasion {
		persuasion.WriteString(fmt.Sprintf("%d. %s\n", i+1, p))
	}

	// Build requirements list
	var requirements strings.Builder
	for i, r := range chatObjective.Requirements {
		requirements.WriteString(fmt.Sprintf("%d. [Index %d] %s\n", i+1, i, r))
	}

	return fmt.Sprintf(
		submitChatPrompt,
		situation,
		constraints.String(),
		persuasion.String(),
		requirements.String(),
	)
}
