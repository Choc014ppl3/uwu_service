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
const systemPrompt = `# Role
You are an expert Linguistic and Educational Content Analyzer. Your task is to analyze the description and generate content details and a quiz in a strict JSON format.

# Instructions
You must analyze the description and determine:
1. topic: Identify the main topic of the video based solely on the transcript. The topic should be concise (1 short sentence or a short phrase).
2. description: Generate a clear and summarizing the video content. The description must be based only on the transcript. Do not invent information that is not present in the transcript. Keep it 2-3 sentences long.
3. level: The estimated language proficiency level required to understand the description. You must use the official or most widely recognized standard framework specific to the identified language. For example:
    * For English: Use the CEFR scale (A1, A2, B1, B2, C1, C2).
    * For Chinese: Use the HSK scale (HSK1, HSK2, HSK3, HSK4, HSK5, HSK6).
    * For Japanese: Use the JLPT scale (N5, N4, N3, N2, N1).
    * For Spanish: Use the DELE scale (A1, A2, B1, B2, C1, C2).
    * For French: Use the DELF/DALF scale (A1, A2, B1, B2, C1, C2).
	* For Russian: Use the TORFL scale (TORFL1, TORFL2, TORFL3, TORFL4, TORFL5, TORFL6).
	* For Portuguese: Use the CAPLE scale (A1, A2, B1, B2, C1, C2).
4. tags: A list of 3-5 relevant topic or thematic tags for the video (e.g., ["travel", "food", "daily life"]).

## CRITICAL STEP: THOUGHT PROCESS FOR QUIZ
Before generating the JSON quiz, you must identify the chronological order of events for the "Sequence" question to ensure accuracy.
1. Identify 4 key events.
2. Verify their order in the description.
3. Only then, map them to the JSON output.

## Part 1: Gist Quiz 3 Questions
1.  **Context/Tone (1 Question):**
    * category: "context"
    * type: "multiple_response"
    * Must have 1-2 correct options (set is_correct: true).
2.  **Main Idea (1 Question):**
    * category: "main_idea"
    * type: "single_choice"
    * Only 1 correct option.
3.  **Sequence (1 Question):**
    * category: "sequence"
    * type: "ordering"
    * Provide 4 events in options (shuffled/random order).
    * Provide the correct_order array containing the correct sequence of Option IDs (e.g., ["B", "A", "C", "D"]).

## Part 2: Retell Story
Generate a concise example of how the user could retell the story based on the transcript following elements:
1. "retell_example": Create a concise, natural-sounding summary of the story. This serves as a model answer or a good example for a student to follow. It should use clear chronological order and appropriate transition words.
2. "key_points": Extract 3 to 5 essential plot points, main events, or key takeaways that the student MUST include in their retelling like in "retell_example" to be considered complete and accurate.

2. "key_points": Extract 3 to 5 essential plot points, main events, or key takeaways that the student MUST include in their retelling like in "retell_example" to be considered complete and accurate.

# Output Format (STRICT JSON)
Do not output any markdown text, introductory phrases, or code blocks. Output ONLY the raw JSON object.
Use the structure below:

{
  "topic": "string",
  "description": "string",
  "level": "string",
  "tags": ["string"],
  "gist_quiz": [
    {
      "id": 1,
      "category": "string (context | objective | sequence)",
      "type": "string (multiple_response | single_choice | ordering)",
      "question": "string",
      "options": [
        { "id": "A", "text": "string", "is_correct": true } // is_correct is null for ordering type
      ],
      "correct_order": ["string"] // null for non-ordering types
    }
  ],
  "retell_story": {
    "retell_example": "string",
	"key_points": ["string"] // 3-5 
  }
}

* Ensure the JSON is valid and parsable.
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
