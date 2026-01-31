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

	// 4. Trigger Image Generation (Async)
	scenarioID := uuid.New().String()
	imagePrompt := tempResp.ImagePrompt
	if imagePrompt == "" {
		imagePrompt = req.Topic
	}

	go func() {
		// Use a detached context for async operations
		bgCtx := context.Background()
		if err := s.generateScenarioImage(bgCtx, scenarioID, imagePrompt); err != nil {
			fmt.Printf("Failed to generate/upload image async: %v\n", err)
		}
	}()

	// 5. Generate Audio for AI lines (Concurrent)
	script := tempResp.Script

	// WaitGroup for concurrent operations
	var wg sync.WaitGroup

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
					url, err := s.cloudflareClient.UploadR2Object(ctx, key, audioData, "audio/mpeg")
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
		url, err := s.cloudflareClient.UploadR2Object(ctx, key, imgData, "image/webp")
		if err != nil {
			return fmt.Errorf("cloudflare upload error: %w", err)
		}
		fmt.Printf("Image uploaded to: %s\n", url)
	}

	return nil
}

// GenerateLearningItemReq defines the request for generating a learning item.
type GenerateLearningItemReq struct {
	Context     string `json:"context"`      // e.g., "Food"
	ContextType string `json:"context_type"` // "word", "character", "phrase", "sentence"
	LangCode    string `json:"lang_code"`    // "en-US", "zh-CN"
	NativeLang  string `json:"native_lang"`  // "th"
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
Your task is to generate a valid JSON object for a language learning database based on the provided template type.

Input Parameters:
- Context: "{{context}}" (The content to analyze)
- Context Type: "{{context_type}}" (The type which is "character", "word", "phrase", "sentence")
- Target Language: "{{lang_code}}" (The language being learned)
- Native Language: "{{native_lang}}" (The user's native language)

Strict Rules:
1. Output MUST be valid JSON only. No markdown formatting or conversational text.
2. Field "meanings": Must contain ONLY the translation in the Native Language ("{{native_lang}}").
3. Field "media_prompts": Write the image prompt in English (as it is the standard for Image Gen AI).
4. All other fields (metadata, tags, examples, synonyms): MUST be in the Target Language ("{{lang_code}}"). DO NOT translate these into Native Language.
5. If specific data is not applicable, use null.

Analyze the "character" provided in the Context Type.
Return this JSON structure: {
  "meanings": {
    "{{native_lang}}": "..." // Meaning/Name of the character in Native Language
  },
  "reading": {
    "ipa": "...", // IPA format
    "standard": "..." // Pinyin (for ZH), Romaji (for JP), or Name (for EN)
  },
  "tags": [
    "...",
    "..."
  ], // e.g., ["vowel", "consonant"] or ["radical", "hsk1"] in Target Lang
  "media": {
    "image_prompt": "A minimalist, high-contrast educational illustration of the character '{{context}}', vector style, white background."
  },
  "metadata": {
    // For English/European Languages:
    "case_pair": "...", // The opposite case (e.g., if input is 'A', output 'a')
    "sound_type": "...", // "vowel" or "consonant" (in Target Lang)
    // For Chinese/Japanese:
    "strokes": 0, // Integer number of strokes
    "radical": "...", // Radical character
    "components": [
      "..."
    ], // List of component parts
  }
}

Analyze the "word" provided in the Context Type.
Return this JSON structure: {
  "meanings": {
    "{{native_lang}}": "..." // Exact meaning in Native Language
  },
  "reading": {
    "ipa": "...", // IPA format
    "standard": "..." // Pinyin, Romaji, or standard phonetic spelling
  },
  "tags": [
    "...",
    "..."
  ], // Parts of speech, Category, Level (e.g., "noun", "food", "A1") in Target Lang
  "media": {
    "image_prompt": "A clear, iconic illustration representing '{{context}}', isolated on white background, cartoon style suitable for learning."
  },
  "metadata": {
    "pos": "...", // Part of Speech in Target Lang (e.g. "noun", "verb")
    "definition": "...", // Definition in Target Lang (Monolingual dictionary style)
    "example_sentence": "...", // Example sentence using the word in Target Lang
    // Optional specific fields (use null if not applicable):
    "classifier": "...", // For CN/TH/JP (in Target Lang)
    "plural_form": "...", // For EN/European (in Target Lang)
    "inflections": { // Verb conjugations (in Target Lang)
      "past": "...",
      "continuous": "..."
    }
  }
}

Analyze the "sentence" or "phrase" provided in the Context Type.
Return this JSON structure: {
  "meanings": {
    "{{native_lang}}": "..." // Natural translation in Native Language
  },
  "reading": {
    "ipa": "...", // Full sentence IPA
    "standard": "..." // Full sentence Pinyin/Romaji
  },
  "tags": [
    "...",
    "..."
  ], // Grammar topic, Situation (e.g., "greeting", "past_tense") in Target Lang
  "media": {
    "image_prompt": "A scene depicting the situation: '{{context}}', expressive characters, vector art style."
  },
  "metadata": {
    "structure_pattern": "...", // e.g., "S + V + O" or Grammar pattern
    // STRICT SELECTION ONLY: Choose values from the lists below based on the nuance of the sentence.
    "usage": {
      // 1. Formality Level (Choose ONE)
      // - literary: Poetic, archaic, or very high-level written style.
      // - formal: Business, official, academic, or polite interaction with strangers.
      // - neutral: Standard language, suitable for most situations.
      // - casual: Relaxed, spoken language with friends or family.
      // - slang: Very informal, street language, or group-specific jargon.
      // - vulgar: Offensive, taboo, or swearing.
      "formality": "...",
      // 2. Emotional Tone (Choose ANY that apply)
      // Positive: polite, respectful, friendly, playful, humorous, affectionate, romantic, gentle, encouraging, enthusiastic, grateful
      // Neutral/Functional: neutral, serious, urgent, authoritative, professional, direct, cautious, hesitant, curious, confused
      // Negative/Complex: sarcastic, ironic, rude, aggressive, angry, cold, insulting, defensive, complaining, sad
      "tone": ["..."],
      // 3. Context: Choose ONE strictly from this list:
      // - daily_life (Family, Friends, Home, Routine)
      // - professional_academic (Work, Office, School, Interview)
      // - services (Shopping, Dining, Bank, Medical, Gov)
      // - travel (Transport, Hotel, Directions, Tourism)
      // - social_leisure (Party, Dating, Hobby, Online, Media)
      // - specialized (Tech, Legal, Emergency, Religious)
      "context": "...",
      // 4. Situations: 1-2 short descriptive strings (Freestyle)
      // Be specific about the situation. e.g., ["bargaining price", "street market"] or ["opening bank account"]
      "situations": ["...", "..."]
    },
    "tokens": [ // Break down the sentence for click-to-translate
      {
        "text": "...", // The word in the sentence
        "pos": "...", // Part of speech in Target Lang
        "lemma": "...", // Root form (e.g., "running" -> "run")
      }
    ]
  }
}
`
	// Replace placeholders
	prompt := strings.ReplaceAll(promptTemplate, "{{context}}", req.Context)
	prompt = strings.ReplaceAll(prompt, "{{context_type}}", req.ContextType)
	prompt = strings.ReplaceAll(prompt, "{{lang_code}}", req.LangCode)
	prompt = strings.ReplaceAll(prompt, "{{native_lang}}", req.NativeLang)

	return prompt
}
