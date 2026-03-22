package video

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/pkg/errors"
)

// The unified system prompt used to generate details and quiz from a transcript.
const systemPrompt = `Role
You are an expert Linguistic and Educational Content Analyzer. Your task is to analyze the transcript and generate structured learning content in a strict JSON format.

# Instructions
You must analyze the transcript and determine:

1. topic:
Identify the main topic based ONLY on the transcript.
Keep it concise (1 short sentence or phrase).

2. description:
Generate a clear summary of the transcript.
- Must be based ONLY on the transcript.
- Do NOT invent or infer missing information.
- Keep it 1-2 sentences long.
- Use a **neutral, content-focused narrative style**.
- Do NOT use first-person (e.g., "I") or reporting phrases (e.g., "the speaker explains", "the video shows").
- Write as if presenting the content directly, similar to how the transcript itself would state it.

3. level:
Estimate the language proficiency level required to understand the content using the appropriate standard:
- English: CEFR (A1-C2)
- Chinese: HSK (HSK1-HSK6)
- Japanese: JLPT (N5-N1)
- Spanish/French/Portuguese: CEFR (A1-C2)
- Russian: TORFL (TORFL1-TORFL6)
- Arabic: ACTFL (Novice, Intermediate, Advanced, Superior)

4. tags:
Generate 3-5 contextual tags.
- Tags must reflect specific situations, actions, or themes in the transcript.
- Avoid generic labels (e.g., "English learning", "A2 level").

## CRITICAL STEP (INTERNAL)
Before generating the sequence question:
- Identify 4 key events from the transcript
- Determine their correct chronological order
- Do NOT include this reasoning in the output

## Part 1: Gist Quiz (EXACTLY 3 Questions)
You MUST generate exactly:
- 1 context question (multiple_response, 1-2 correct answers)
- 1 main_idea question (single_choice, 1 correct answer)
- 1 sequence question (ordering)

### Rules:
- Do NOT use external knowledge
- Do NOT fabricate missing details

### Sequence Question Rules:
- Provide exactly 4 events
- Shuffle the options
- Use "correct_order" to define the answer
- Do NOT include "is_correct" for ordering options

## Part 2: Retell Story

1. retell_example:
- Use a **neutral, content-focused narrative style** (same style as description).
- Do NOT use first-person (e.g., "I") or reporting phrases (e.g., "the transcript explains", "this video shows").
- Present the content directly as a natural summary or short narration.

- Enforce tone and style:
  - Use a **natural storytelling flow**, not a step-by-step list.
  - Avoid rigid transitions like "First", "Then", "After that".
  - Use natural connectors where appropriate (e.g., "so", "because", "while", "later").

- Adjust tone and complexity based on level:
  - A1-A2: very simple sentences, basic vocabulary, short and clear ideas.
  - B1-B2: more natural flow, some connectors, slightly longer sentences.
  - C1-C2: fluent, expressive, nuanced phrasing.

- Keep it concise and aligned with the language level.

2. key_points:
- Extract 3-5 essential events or takeaways.
- Must align with retell_example and cover the full content.

# Output Format (STRICT JSON)
- Output ONLY valid JSON
- Do NOT include markdown, comments, or extra text
- Ensure the JSON is fully parsable

{
  "topic": "string",
  "description": "string",
  "level": "string",
  "tags": ["string"],
  "gist_quiz": [
    {
      "id": 1,
      "category": "context | main_idea | sequence",
      "type": "multiple_response | single_choice | ordering",
      "question": "string",
      "options": [
        { "id": "A", "text": "string", "is_correct": true }
      ],
      "correct_order": ["string"]
    }
  ],
  "retell_story": {
    "retell_example": "string",
    "key_points": ["string"]
  }
}
`

// AIRepository interface
type AIRepository interface {
	GenerateVideoTranscript(ctx context.Context, audioPath, language string) (*client.WhisperResponse, *errors.AppError)
	GenerateVideoDetails(ctx context.Context, transcript *client.WhisperResponse) (*VideoDetails, *errors.AppError)
}

type TranscriptSegment struct {
	Text     string  `json:"text"`
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
}

// aiRepository is the implementation of the AIRepository interface
type aiRepository struct {
	chatGPT *client.AzureChatGPTClient
	whisper *client.AzureWhisperClient
	log     *slog.Logger
}

// NewAIRepository creates a new aiRepository
func NewAIRepository(whisper *client.AzureWhisperClient, chatGPT *client.AzureChatGPTClient, log *slog.Logger) *aiRepository {
	return &aiRepository{chatGPT: chatGPT, whisper: whisper, log: log}
}

// GenerateVideoTranscript generates video transcript
func (r *aiRepository) GenerateVideoTranscript(ctx context.Context, audioPath, language string) (*client.WhisperResponse, *errors.AppError) {
	// Convert language to ISO 639-1 code
	switch language {
	case "Chinese":
		language = "zh"
	case "Japanese":
		language = "ja"
	default:
		language = "en"
	}

	transcript, err := r.whisper.TranscribeFile(ctx, audioPath, language)
	if err != nil {
		r.log.Error("Whisper transcription failed", "error", err.Error())
		return nil, err
	}
	return transcript, nil
}

// GenerateVideoDetails generates video details
func (r *aiRepository) GenerateVideoDetails(ctx context.Context, transcript *client.WhisperResponse) (*VideoDetails, *errors.AppError) {
	// Convert transcript segments
	segments := []TranscriptSegment{}
	for _, ws := range transcript.Segments {
		segments = append(segments, TranscriptSegment{
			Text:     ws.Text,
			Start:    ws.Start,
			Duration: ws.End - ws.Start,
		})
	}

	// Build transcript text
	var sb strings.Builder
	for _, seg := range segments {
		sb.WriteString(seg.Text)
		sb.WriteString(" ")
	}
	transcriptText := strings.TrimSpace(sb.String())
	if transcriptText == "" {
		return nil, errors.Internal("Empty transcript")
	}

	// Build LLM prompt
	detectedLanguage := transcript.Language
	userMessage := fmt.Sprintf("Transcript:\n\"\"\"\n%s\n\"\"\"\n\nLanguage: %s", transcriptText, detectedLanguage)

	responseText, err := r.chatGPT.ChatCompletion(ctx, systemPrompt, userMessage)
	if err != nil {
		return nil, err
	}

	// Clean up responseText
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	// Parse responseText
	var videoDetails VideoDetails
	if err := json.Unmarshal([]byte(responseText), &videoDetails); err != nil {
		return nil, errors.InternalWrap("failed to parse video details", err)
	}

	// Update video details
	videoDetails.Language = strings.ToLower(detectedLanguage)
	videoDetails.Segments = segments
	videoDetails.Transcript = transcriptText

	return &videoDetails, nil
}
