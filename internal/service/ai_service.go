package service

import (
	"context"
	"fmt"
	"strings"

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

// GenerateScenarioContentReq defines the request for generating scenario content.
type GenerateScenarioContentReq struct {
	Topic           string `json:"topic"`
	Description     string `json:"description"`
	InteractionType string `json:"interaction_type"` // "chat", "speech"
	EstimatedTurns  string `json:"estimate_turns"`
	TargetLang      string `json:"target_lang"`
}

// GenerateScenarioContent generates the text content for a scenario.
func (s *AIService) GenerateScenarioContent(ctx context.Context, req GenerateScenarioContentReq) (string, error) {
	if s.geminiClient == nil {
		return "", errors.New(errors.ErrAIService, "Gemini client not configured")
	}

	promptTemplate := `Role: You are a specialized Language Learning Content Generator.
Your task is to generate a valid JSON object for a language learning database based on the provided template type.

Input Parameters:
- Topic: "{{topic}}" (The content to analyze)
- Description: "{{description}}" (Context details)
- Interaction Type: "{{interaction_type}}" (chat, speech)
- Estimate Turns: "{{estimate_turns}}" (Integer or Range)
- Target Language: "{{target_lang}}" (The language being learned)

Strict Constraints:
1. Language: All generated text, including hints, objectives, and instructions, MUST be in "{{target_lang}}". Absolutely NO any other language allowed.
2. Data Integrity: If a field is not applicable, you must explicitly set it to null.
3. Format: Output ONLY a valid JSON object. Do not include the comments ( //) in the final output. No prose, no markdown code blocks.
4. Minimum Turn Count: The total number of objects in the script array must be at least 6, otherwise, you must generate more turns.
5. Maximum Turn Count: The total number of objects in the script array must be at most 24, otherwise, you must generate less turns.
6. Image Prompt: The "image_prompt" field MUST be in English. It should be a highly detailed visual description suitable for a text-to-image AI (like DALL-E 3 or Imagen). Describe the setting, characters, action, lighting, and mood. Specify "high quality, educational vector art style" and "no text in image".
---

### If Interaction Type is "speech"
Generate the JSON following these Flow Rules:
1. **Starting Turn:** You may start with either "ai" or "user", whichever fits the context best.
2. **Turn Sequence:** Do NOT force strict alternation (A-B-A-B). Allow natural pauses or multi-part thoughts.
3. **Consecutive Limit:** A single speaker can have consecutive turns, but NO MORE than 2 turns in a row.
4. **Total Length:** The total number of objects in the script array must match "{{estimate_turns}}".
5. **Script Object Structure:** The "script" array must contain all types of user turns (context_hint, partial_blank, predictive_blank) and ai turns.

{
    "interaction_type": "speech",
    "difficulty_level": 1, // Estimate the difficulty level (1-5) based on the vocabulary and grammar used.
    "image_prompt": "...", // Generate a detailed English description of the scene. e.g., 'Two people talking in a cozy coffee shop, warm lighting, vector art style, educational illustration.'
    "script": [
        {
            "speaker": "...", // "ai" or "user"
            
            // Logic for 'text':
            // - If speaker is "ai": Content in {{target_lang}}.
            // - If speaker is "user" AND type is "partial_blank": Content in {{target_lang}} with '_____' placeholders.
            // - Otherwise: null.
            "text": "...", 

            "user_turn_details": { 
                // Required if speaker is "user", set to null if "ai".
                
                "type": "...", 
                // CRITICAL: YOU MUST SELECT ONE TYPE. DO NOT LEAVE BLANK.
                // 1. "context_hint": User must formulate the full sentence themselves. (Used for: Roleplay, asking questions, expressing needs).
                // 2. "partial_blank": User fills in specific missing words. (Used for: Vocabulary checks, grammar focus).
                // 3. "predictive_blank": User gives a short, obvious response implied by context with just a few hints.

                "hint": "...", 
                // - Required for "context_hint": Instruction in {{target_lang}} (e.g., "Ask for the price").
                // - Optional for "partial_blank": Clue for the missing word.
                // - Set to null for "predictive_blank".

                "missing_words": ["..."] 
                // - Required for "partial_blank" and "predictive_blank": Array of words filling the '_____' in order.
                // - Set to null for others.
            }
        }
    ]
}

---

### If Interaction Type is "chat"
Generate the JSON using this logic.

{
    "interaction_type": "chat",
    "image_prompt": "...", // Generate a detailed English description of the chat context. e.g. 'A smartphone screen showing a chat app interface, with a background of a busy office, flat design, modern ui.'
    "objectives": {
        "requirements": [ 
            // Generate 3-5 items
            "...", 
            // Definition: Task-oriented goals.
            // Examples: "Use the word 'receipt'", "Ask for the warranty period", "Mention specifically that you are in a hurry".
        ],
        "persuasion": [ 
            // Generate 1-3 items
            "...", 
            // Definition: The "Winning Condition" or ultimate outcome.
            // Examples: "Convince the shopkeeper to give a 10% discount", "Get the manager to approve the refund".
        ],
        "constraints": [ 
            // Generate 1-3 items
            "...", 
            // Definition: Tone, Manner, or Restrictions.
            // Examples: "Must use formal language (Keigo)", "Do not use emojis", "Remain polite even if the AI is rude".
        ]
    }
}`

	prompt := strings.ReplaceAll(promptTemplate, "{{topic}}", req.Topic)
	prompt = strings.ReplaceAll(prompt, "{{description}}", req.Description)
	prompt = strings.ReplaceAll(prompt, "{{interaction_type}}", req.InteractionType)
	prompt = strings.ReplaceAll(prompt, "{{estimate_turns}}", req.EstimatedTurns)
	prompt = strings.ReplaceAll(prompt, "{{target_lang}}", req.TargetLang)

	return s.geminiClient.Chat(ctx, prompt)
}

// GenerateAndUploadImage generates an image and uploads it to Cloudflare R2.
func (s *AIService) GenerateAndUploadImage(ctx context.Context, id, prompt string) (string, error) {
	if s.geminiClient == nil {
		return "", errors.New(errors.ErrAIService, "Gemini client not configured")
	}
	if s.cloudflareClient == nil {
		return "", errors.New(errors.ErrAIService, "Cloudflare client not configured")
	}

	// 1. Generate Image
	imgData, err := s.geminiClient.GenerateImage(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("gemini generate image error: %w", err)
	}

	// 2. Upload to Cloudflare R2
	key := fmt.Sprintf("conversation-scenarios/%s-image.webp", id)
	url, err := s.cloudflareClient.UploadR2Object(ctx, key, imgData, "image/webp")
	if err != nil {
		return "", fmt.Errorf("cloudflare upload error: %w", err)
	}

	return url, nil
}

// GenerateAndUploadAudio generates audio and uploads it to Cloudflare R2.
func (s *AIService) GenerateAndUploadAudio(ctx context.Context, id string, index int, text, lang string) (string, error) {
	if s.azureSpeechClient == nil {
		return "", errors.New(errors.ErrAIService, "Azure Speech client not configured")
	}
	if s.cloudflareClient == nil {
		return "", errors.New(errors.ErrAIService, "Cloudflare client not configured")
	}

	// Dynamic Voice Selection
	voiceName := "en-US-AvaMultilingualNeural" // Default
	switch {
	case strings.HasPrefix(lang, "zh"):
		voiceName = "zh-CN-XiaoxiaoNeural"
	case strings.HasPrefix(lang, "th"):
		voiceName = "th-TH-PremwadeeNeural"
	case strings.HasPrefix(lang, "ja"):
		voiceName = "ja-JP-NanamiNeural"
	case strings.HasPrefix(lang, "ko"):
		voiceName = "ko-KR-SunHiNeural"
	}

	audioData, err := s.azureSpeechClient.Synthesize(ctx, text, voiceName)
	if err != nil {
		return "", fmt.Errorf("azure speech synthesize error: %w", err)
	}

	key := fmt.Sprintf("conversation-scenarios/%s-ai-script-%d.mp3", id, index)
	url, err := s.cloudflareClient.UploadR2Object(ctx, key, audioData, "audio/mpeg")
	if err != nil {
		return "", fmt.Errorf("cloudflare upload error: %w", err)
	}

	return url, nil
}

// GenerateLearningItemReq defines the request for generating a learning item.
type GenerateLearningItemReq struct {
	Context    string `json:"context"`     // e.g., "Food"
	LangCode   string `json:"lang_code"`   // "en-US", "zh-CN"
	NativeLang string `json:"native_lang"` // "th"
}

// GenerateLearningItem generates structured learning data using Gemini.
func (s *AIService) GenerateLearningItem(ctx context.Context, req GenerateLearningItemReq) (string, error) {
	if s.geminiClient == nil {
		return "", errors.New(errors.ErrAIService, "Gemini client not configured")
	}

	fullPrompt := s.buildLearningItemPrompt(req)
	return s.geminiClient.Chat(ctx, fullPrompt)
}

func (s *AIService) buildLearningItemPrompt(req GenerateLearningItemReq) string {
	promptTemplate := `
You are a strict Linguistic Data Generator API.
Your task is to generate a valid JSON object for a language learning database.

Input Parameters:
- Context: "{{context}}" (The content to analyze)
- Target Language: "{{lang_code}}" (The language being learned)
- Native Language: "{{native_lang}}" (The user's native language)

Strict Rules:
1. Output MUST be valid JSON only. No markdown formatting or conversational text.
2. Field "context_type": MUST be inferred from the Context. Choose ONE from: "character", "word", "phrase", "sentence".
   - "character": A single letter or CJK radical (e.g., "A", "水", "あ")
   - "word": A single word (e.g., "apple", "食べる", "漂亮")
   - "phrase": A short phrase (2-4 words, idiomatic expressions, e.g., "good morning", "对不起")
   - "sentence": A complete sentence (e.g., "How are you?", "今天天气很好。")
3. Field "meanings": Must contain ONLY the translation in the Native Language ("{{native_lang}}").
4. Field "media.image_prompt": Write in English. Must be DETAILED and DESCRIPTIVE. Include subject details, background, lighting, and style (e.g., "minimalist vector art").
5. All other fields (metadata, tags): MUST be in the Target Language ("{{lang_code}}").
6. If specific data is not applicable, use null.

Return a JSON object with this structure based on the inferred context_type:

{
  "context_type": "...", // REQUIRED. One of: "character", "word", "phrase", "sentence"
  "meanings": {
    "{{native_lang}}": "..." // Translation in Native Language
  },
  "reading": {
    "ipa": "...", // IPA format
    "standard": "..." // Pinyin (for ZH), Romaji (for JP), or standard phonetic
  },
  "tags": ["...", "..."], // In Target Language
  "media": {
    "image_prompt": "..." // English prompt for image generation
  },
  "metadata": {
    // Include relevant fields based on context_type:
    // For "character": case_pair, sound_type, strokes, radical, components
    // For "word": pos, definition, example_sentence, classifier, plural_form, inflections
    // For "phrase"/"sentence": structure_pattern, usage (formality, tone, context, situations), tokens
  }
}
`
	// Replace placeholders
	prompt := strings.ReplaceAll(promptTemplate, "{{context}}", req.Context)
	prompt = strings.ReplaceAll(prompt, "{{lang_code}}", req.LangCode)
	prompt = strings.ReplaceAll(prompt, "{{native_lang}}", req.NativeLang)

	return prompt
}
