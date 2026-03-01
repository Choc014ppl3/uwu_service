package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/repository"
)

// AIService provides AI-related functionality.
type AIService struct {
	geminiClient       *client.GeminiClient
	cloudflareClient   *client.CloudflareClient
	azureSpeechClient  *client.AzureSpeechClient
	learningItemRepo   repository.LearningItemRepository
	learningSourceRepo repository.LearningSourceRepository
	batchService       *BatchService
}

// NewAIService creates a new AI service.
func NewAIService(
	geminiClient *client.GeminiClient,
	cloudflareClient *client.CloudflareClient,
	azureSpeechClient *client.AzureSpeechClient,
	learningItemRepo repository.LearningItemRepository,
	learningSourceRepo repository.LearningSourceRepository,
	batchService *BatchService,
) *AIService {
	return &AIService{
		geminiClient:       geminiClient,
		cloudflareClient:   cloudflareClient,
		azureSpeechClient:  azureSpeechClient,
		learningItemRepo:   learningItemRepo,
		learningSourceRepo: learningSourceRepo,
		batchService:       batchService,
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

// GenerateDialogueGuildReq defines the request for generating a dialogue guild.
type GenerateDialogueGuildReq struct {
	Topic       string   `json:"topic"`
	Description string   `json:"description"`
	Language    string   `json:"language"`
	Level       string   `json:"level"`
	Tags        []string `json:"tags"`
}

// DialogueGuildResponse represents the expected parsed structure from Gemini API
type DialogueGuildResponse struct {
	ImagePrompt string          `json:"image_prompt"`
	Level       string          `json:"level"`
	Tags        []string        `json:"tags"`
	SpeechMode  json.RawMessage `json:"speech_mode"`
	ChatMode    json.RawMessage `json:"chat_mode"`
	Words       []struct {
		Text            string   `json:"text"`
		Level           string   `json:"level"`
		Tags            []string `json:"tags"`
		ReadingStandard string   `json:"reading_standard"`
		ReadingStress   string   `json:"reading_stress"`
		POS             string   `json:"pos"`
		Definition      string   `json:"definition"`
		ExSentence      string   `json:"ex_sentence"`
	} `json:"words"`
	Sentences []struct {
		Text            string          `json:"text"`
		Level           string          `json:"level"`
		Tags            []string        `json:"tags"`
		ReadingStandard string          `json:"reading_standard"`
		ReadingStress   string          `json:"reading_stress"`
		StructureFormat string          `json:"structure_format"`
		Usage           json.RawMessage `json:"usage"`
	} `json:"sentences"`
}

const (
	dialogueGuildJobName            = "generate_dialogue_guild"
	dialogueGuildImageJobName       = "generate_image"
	dialogueGuildUploadJobName      = "upload_image"
	dialogueGuildAudioJobName       = "generate_audio"
	dialogueGuildUploadAudioJobName = "upload_audio"
)

// GenerateDialogueGuild initiates speech and chat conversations generation for a dialogue guild using Gemini asynchronously.
// Returns a batch_id immediately which can be used to poll for the result.
func (s *AIService) GenerateDialogueGuild(ctx context.Context, req GenerateDialogueGuildReq) (string, error) {
	if s.geminiClient == nil {
		return "", errors.New(errors.ErrAIService, "Gemini client not configured")
	}

	batchID := uuid.New().String()

	// Create batch with a single job
	if s.batchService != nil {
		_ = s.batchService.CreateBatchWithJobs(ctx, batchID, req.Topic, []string{
			dialogueGuildJobName,
			dialogueGuildImageJobName,
			dialogueGuildUploadJobName,
			dialogueGuildAudioJobName,
			dialogueGuildUploadAudioJobName,
		})
	}

	// Run processing in background
	go s.processDialogueGuildAsync(batchID, req)

	return batchID, nil
}

// processDialogueGuildAsync runs the AI call, parses, saves to DB, and updates the batch status.
func (s *AIService) processDialogueGuildAsync(batchID string, req GenerateDialogueGuildReq) {
	ctx := context.Background()

	if s.batchService != nil {
		_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildJobName, "processing", "")
	}

	promptTemplate := `Role: AI Language Learning Content Generator (JSON API)

You are a strictly formatted backend JSON API driven by an expert linguist and native-speaking language teacher. Your task is to generate highly engaging, culturally accurate, and natural conversational content for Speech Practice and Chat Missions. Ensure the language used is conversational, not purely textbook.

# Input Parameters
* **Topic:** {{TOPIC}}
* **Description:** {{DESCRIPTION}}
* **Language:** {{LANGUAGE}}
* **Level:** {{LEVEL}}
* **Tags:** {{TAGS}} Generate 3-5 relevant keywords describing the conversation context if there are no tags provided.

# Processing Rules

## 1. Content Generation Logic
* **Image Prompt:** Create a prompt for a text-to-image model in English.
    * *Style:* **Photorealistic, Cinematic lighting, Lightweight thumbnail image, No text/words in image.**
    * *Content:* Strictly depict the setting and atmosphere described from the topic, description, and conversation context.
* **Speech Mode (Script) - OPTIMIZED FOR LEARNING:**
    * **Length Constraint:** Generate **ONLY 6-10 turns for Beginner level, 10-16 turns for Intermediate level, and 16-24 turns for Advanced level**. Keep it concise.
    * **Cognitive Load Control:** Ensure each "user" turn is **1-3 sentences max**. Avoid long monologues (too hard to memorize) and avoid single words (too easy).
    * **Create a realistic dialogue where the User has a clear goal. The AI should guide the conversation naturally.**
* **Chat Mode (Objectives):**
    * Create a "Mission" based on the same scenario.
    * Ensure the objectives (requirements/persuasion) are smooth, logical, and match the difficulty level detected.

## 2. Vocabulary & Sentence Extraction Logic
* **Words Extraction:** Extract 5-10 key vocabulary words directly from the generated "speech_mode" script. Provide accurate IPA ("reading_standard"), show syllable stress ("reading_stress"), provide the definition ("definition"), and ensure the "ex_sentence" matches the word's specific meaning in this context.
* **Sentences Extraction:** Extract 3-5 high-value or structurally important sentences directly from the script. Provide sentence-level intonation/stress markers, breakdown the grammatical structure ("structure_format"), and detail its specific usage context.

## 3. Strict Output Constraints
* **Output ONLY valid JSON.**
* **DO NOT** use markdown code blocks.
* **DO NOT** include any conversational text, explanations, or comments.
* Ensure all strings are properly escaped.

# JSON Output Schema

{
  "image_prompt": "string", // English, Photorealistic style
  "level": "string", // Re-estimate level based on generated content
  "tags": ["string"], // Generate 3-5 relevant keywords describing the conversation context if there are no tags provided.
  "speech_mode": {
    "situation": "string", // Brief context to explain conversation context
    "script": [
      {
        "speaker": "string", // "User" or "AI"
        "text": "string" // Actual dialogue text
      }
	  // ... Generate enough turns to cover the content ...
    ]
  },
  "chat_mode": {
    "situation": "string", // Brief context to explain the scenario and the user's goal
    "objectives": {
      "requirements": ["string"], // 2-3 Actionable tasks suited to the level
      "persuasion": ["string"], // 1-2 Goals to achieve in the conversation
      "constraints": ["string"] // 1-2 Behavioral/Tonal constraints
    }
  },
  "words": [
    {
      "text": "string", // The target word extracted from the script
      "level": "string", // CEFR level of the word (e.g., A1, B2)
      "tags": ["string"], // Relevant categories (e.g., "noun", "business", "travel")
      "reading_standard": "string", // IPA transcription (e.g., /ˈkæm.rə/)
      "reading_stress": "string", // Syllable stress representation (e.g., CA-me-ra)
      "pos": "string", // Part of Speech (e.g., Noun, Verb, Adjective)
	  "definition": "string", // Definition of the word
      "ex_sentence": "string" // An example sentence using the word
    }
  ],
  "sentences": [
    {
      "text": "string", // The target sentence extracted from the script
      "level": "string", // CEFR level of the sentence structure
      "tags": ["string"], // Grammatical or topical tags (e.g., "request", "present perfect")
      "reading_standard": "string", // Broad phonetic transcription or pronunciation guide
      "reading_stress": "string", // Highlight stressed words/intonation in the sentence
      "structure_format": "string", // Grammatical structure explanation (e.g., "Subject + modal verb (would) + like + infinitive")
      "usage": {
        "formality": "string", // e.g., "Formal", "Informal", "Neutral"
        "tone": "string", // e.g., "Polite", "Direct", "Friendly"
        "context": "string", // Explanation of when to use this sentence
        "situations": ["string"] // Specific situations where this is applicable
      }
    }
  ]
}`

	tagsStr := ""
	if len(req.Tags) > 0 {
		tagsStr = strings.Join(req.Tags, ", ")
	}

	prompt := strings.ReplaceAll(promptTemplate, "{{TOPIC}}", req.Topic)
	prompt = strings.ReplaceAll(prompt, "{{DESCRIPTION}}", req.Description)
	prompt = strings.ReplaceAll(prompt, "{{LANGUAGE}}", req.Language)
	prompt = strings.ReplaceAll(prompt, "{{LEVEL}}", req.Level)
	prompt = strings.ReplaceAll(prompt, "{{TAGS}}", tagsStr)

	respText, err := s.geminiClient.Chat(ctx, prompt)
	if err != nil {
		if s.batchService != nil {
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildJobName, "failed", err.Error())
		}
		return
	}

	cleanResp := strings.TrimSpace(respText)
	cleanResp = strings.TrimPrefix(cleanResp, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```")

	var parsedResp DialogueGuildResponse
	if err := json.Unmarshal([]byte(cleanResp), &parsedResp); err != nil {
		// Log error but we still try to save the raw response or update batch
		fmt.Printf("Warning: failed to unmarshal DialogueGuildResponse: %v\n", err)
		if s.batchService != nil {
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildJobName, "failed", "failed to parse DialogueGuildResponse JSON: "+err.Error())
		}
		return
	}

	// 1. Save Learning Sources (Words & Sentences)
	if s.learningSourceRepo != nil {
		for _, w := range parsedResp.Words {
			levelPtr := &w.Level
			if w.Level == "" {
				levelPtr = &parsedResp.Level
			}

			tagsBytes, _ := json.Marshal(w.Tags)

			metadataMap := map[string]interface{}{
				"reading_standard": w.ReadingStandard,
				"reading_stress":   w.ReadingStress,
				"ex_sentence":      w.ExSentence,
				"definition":       w.Definition,
				"pos":              w.POS,
				"batch_id":         batchID,
			}
			metadataBytes, _ := json.Marshal(metadataMap)

			ls := &repository.LearningSource{
				Content:  w.Text,
				Language: req.Language,
				Type:     repository.LearningSourceTypeWord,
				Level:    levelPtr,
				Tags:     tagsBytes,
				Metadata: metadataBytes,
			}
			var err error
			if s.learningSourceRepo != nil {
				err = s.learningSourceRepo.Create(ctx, ls)
				if err != nil {
					fmt.Printf("Warning: failed to trace save word: %v\n", err)
				}
			}
		}

		for _, st := range parsedResp.Sentences {
			levelPtr := &st.Level
			if st.Level == "" {
				levelPtr = nil
			}

			tagsBytes, _ := json.Marshal(st.Tags)

			metadataMap := map[string]interface{}{
				"reading_standard": st.ReadingStandard,
				"reading_stress":   st.ReadingStress,
				"structure_format": st.StructureFormat,
				"usage":            st.Usage,
				"batch_id":         batchID,
			}
			metadataBytes, _ := json.Marshal(metadataMap)

			ls := &repository.LearningSource{
				Content:  st.Text,
				Language: req.Language,
				Type:     repository.LearningSourceTypeSentence,
				Level:    levelPtr,
				Tags:     tagsBytes,
				Metadata: metadataBytes,
			}
			if s.learningSourceRepo != nil {
				_ = s.learningSourceRepo.Create(ctx, ls)
			}
		}
	}

	// 2. Generate Image
	var imageURL string
	if parsedResp.ImagePrompt != "" && s.geminiClient != nil {
		if s.batchService != nil {
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildImageJobName, "processing", "")
		}

		imageBytes, err := s.geminiClient.GenerateImage(ctx, parsedResp.ImagePrompt)
		if err != nil {
			fmt.Printf("Warning: failed to generate image: %v\n", err)
			if s.batchService != nil {
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildImageJobName, "failed", err.Error())
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildUploadJobName, "failed", "Skipped due to generation failure")
			}
		} else {
			if s.batchService != nil {
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildImageJobName, "completed", "")
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildUploadJobName, "processing", "")
			}

			// Upload Image
			if s.cloudflareClient != nil {
				objectKey := fmt.Sprintf("dialogue_guilds/%s/%s.png", time.Now().UTC().Format("2006/01/02"), uuid.New().String())
				url, err := s.cloudflareClient.UploadR2Object(ctx, objectKey, imageBytes, "image/png")
				if err != nil {
					fmt.Printf("Warning: failed to upload image: %v\n", err)
					if s.batchService != nil {
						_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildUploadJobName, "failed", err.Error())
					}
				} else {
					imageURL = url
					if s.batchService != nil {
						_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildUploadJobName, "completed", "")
					}
				}
			} else {
				if s.batchService != nil {
					_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildUploadJobName, "failed", "Cloudflare client not configured")
				}
			}
		}
	} else {
		// skip image jobs if no prompt or client
		if s.batchService != nil {
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildImageJobName, "completed", "")
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildUploadJobName, "completed", "")
		}
	}

	// 3. Generate Audio for Situation
	var audioURL string

	// SpeechMode is json.RawMessage, so we need to decode it to extract "situation"
	var speechModeMap map[string]interface{}
	var situationText string
	if len(parsedResp.SpeechMode) > 0 {
		if err := json.Unmarshal(parsedResp.SpeechMode, &speechModeMap); err == nil {
			if situationObj, ok := speechModeMap["situation"]; ok {
				if situationStr, ok := situationObj.(string); ok {
					situationText = situationStr
				}
			}
		}
	}

	if situationText != "" && s.azureSpeechClient != nil {
		if s.batchService != nil {
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildAudioJobName, "processing", "")
		}

		audioBytes, err := s.azureSpeechClient.Synthesize(ctx, situationText, "en-US-AvaMultilingualNeural")
		if err != nil {
			fmt.Printf("Warning: failed to generate audio: %v\n", err)
			if s.batchService != nil {
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildAudioJobName, "failed", err.Error())
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildUploadAudioJobName, "failed", "Skipped due to generation failure")
			}
		} else {
			if s.batchService != nil {
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildAudioJobName, "completed", "")
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildUploadAudioJobName, "processing", "")
			}

			// Upload Audio
			if s.cloudflareClient != nil {
				objectKey := fmt.Sprintf("dialogue_guilds/%s/%s.mp3", time.Now().UTC().Format("2006/01/02"), uuid.New().String())
				url, err := s.cloudflareClient.UploadR2Object(ctx, objectKey, audioBytes, "audio/mpeg")
				if err != nil {
					fmt.Printf("Warning: failed to upload audio: %v\n", err)
					if s.batchService != nil {
						_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildUploadAudioJobName, "failed", err.Error())
					}
				} else {
					audioURL = url
					if s.batchService != nil {
						_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildUploadAudioJobName, "completed", "")
					}
				}
			} else {
				if s.batchService != nil {
					_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildUploadAudioJobName, "failed", "Cloudflare client not configured")
				}
			}
		}
	} else {
		if s.batchService != nil {
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildAudioJobName, "completed", "")
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildUploadAudioJobName, "completed", "")
		}
	}

	// 4. Save Learning Item (Scenario Context)
	if s.learningItemRepo != nil {
		// Assuming PocketMission (4) or SparringMode (9) as best fit. Going with PocketMission as default here.
		featureID := repository.PocketMission
		levelPtr := &req.Level
		if req.Level == "" {
			levelPtr = &parsedResp.Level
		}

		tagsBytes, _ := json.Marshal(parsedResp.Tags)

		metadataMap := map[string]interface{}{
			"speech_mode": parsedResp.SpeechMode,
			"chat_mode":   parsedResp.ChatMode,
		}

		// Save batch variables safely to Details JSONb
		detailsMap := map[string]interface{}{
			"image_prompt": parsedResp.ImagePrompt,
			"topic":        req.Topic,
			"description":  req.Description,
			"batch_id":     batchID,
			"req_body":     req, // Store original request payload
		}

		if imageURL != "" {
			detailsMap["image_url"] = imageURL
		}
		if audioURL != "" {
			detailsMap["audio_url"] = audioURL
		}

		// Modify the cleanResp JSON to include the media urls dynamically for the client's result cache
		if imageURL != "" || audioURL != "" {
			var cleanRespMap map[string]interface{}
			if err := json.Unmarshal([]byte(cleanResp), &cleanRespMap); err == nil {
				if imageURL != "" {
					cleanRespMap["image_url"] = imageURL
				}
				if audioURL != "" {
					cleanRespMap["audio_url"] = audioURL
				}

				if updatedBytes, err := json.Marshal(cleanRespMap); err == nil {
					cleanResp = string(updatedBytes)
				}
			}
		}

		metadataBytes, _ := json.Marshal(metadataMap)
		detailsBytes, _ := json.Marshal(detailsMap)

		li := &repository.LearningItem{
			FeatureID:      &featureID,
			Content:        req.Topic,
			LangCode:       req.Language,
			EstimatedLevel: levelPtr,
			Tags:           tagsBytes,
			Metadata:       metadataBytes,
			Details:        detailsBytes,
			IsActive:       true,
		}
		_ = s.learningItemRepo.Create(ctx, li)
	}

	if s.batchService != nil {
		// Store pure clean JSON directly to batch result so the client can fetch it
		_ = s.batchService.SetBatchResult(ctx, batchID, []byte(cleanResp))
		_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuildJobName, "completed", "")
	}
}

// GetDialogueGuildByBatchID reconstructs the batch response from the database if the metadata has been archived.
func (s *AIService) GetDialogueGuildByBatchID(ctx context.Context, batchID string) (*BatchStatus, error) {
	if s.learningItemRepo == nil || s.learningSourceRepo == nil {
		return nil, nil // Return nil to signal not found or not configured
	}

	items, err := s.learningItemRepo.GetByBatchID(ctx, batchID)
	if err != nil || len(items) == 0 {
		return nil, nil // Not found
	}
	masterItem := items[0]

	sources, err := s.learningSourceRepo.GetByBatchID(ctx, batchID)
	if err != nil {
		return nil, nil
	}

	// Reconstruct the response data
	var meta map[string]interface{}
	if len(masterItem.Metadata) > 0 {
		_ = json.Unmarshal(masterItem.Metadata, &meta)
	}

	var details map[string]interface{}
	if len(masterItem.Details) > 0 {
		_ = json.Unmarshal(masterItem.Details, &details)
	}

	var tags []string
	if len(masterItem.Tags) > 0 {
		_ = json.Unmarshal(masterItem.Tags, &tags)
	}

	resultMap := map[string]interface{}{
		"image_prompt": details["image_prompt"],
		"image_url":    details["image_url"],
		"audio_url":    details["audio_url"],
		"speech_mode":  meta["speech_mode"],
		"chat_mode":    meta["chat_mode"],
		"level":        masterItem.EstimatedLevel,
		"tags":         tags,
	}

	// Extract words and sentences
	var words []map[string]interface{}
	var sentences []map[string]interface{}

	for _, src := range sources {
		var srcMeta map[string]interface{}
		var srcTags []string
		_ = json.Unmarshal(src.Metadata, &srcMeta)
		_ = json.Unmarshal(src.Tags, &srcTags)

		if src.Type == repository.LearningSourceTypeWord {
			word := map[string]interface{}{
				"text":             src.Content,
				"level":            src.Level,
				"tags":             srcTags,
				"reading_standard": srcMeta["reading_standard"],
				"reading_stress":   srcMeta["reading_stress"],
				"pos":              srcMeta["pos"],
				"definition":       srcMeta["definition"],
				"ex_sentence":      srcMeta["ex_sentence"],
			}
			words = append(words, word)
		} else if src.Type == repository.LearningSourceTypeSentence {
			sentence := map[string]interface{}{
				"text":             src.Content,
				"level":            src.Level,
				"tags":             srcTags,
				"reading_standard": srcMeta["reading_standard"],
				"reading_stress":   srcMeta["reading_stress"],
				"structure_format": srcMeta["structure_format"],
				"usage":            srcMeta["usage"],
			}
			sentences = append(sentences, sentence)
		}
	}

	resultMap["words"] = words
	resultMap["sentences"] = sentences

	// Assemble final BatchStatus structure matching Redis model
	resultJSON, _ := json.Marshal(resultMap)

	batch := &BatchStatus{
		BatchID:       batchID,
		ReferenceID:   masterItem.Content, // Use topic as reference ID
		Status:        "completed",        // If it's in the DB, it's considered completed
		TotalJobs:     5,
		CompletedJobs: 5,
		Jobs: []JobStatus{
			{
				Name:        dialogueGuildJobName,
				Status:      "completed",
				CompletedAt: masterItem.CreatedAt.Format(time.RFC3339),
			},
			{
				Name:        dialogueGuildImageJobName,
				Status:      "completed",
				CompletedAt: masterItem.CreatedAt.Format(time.RFC3339),
			},
			{
				Name:        dialogueGuildUploadJobName,
				Status:      "completed",
				CompletedAt: masterItem.CreatedAt.Format(time.RFC3339),
			},
			{
				Name:        dialogueGuildAudioJobName,
				Status:      "completed",
				CompletedAt: masterItem.CreatedAt.Format(time.RFC3339),
			},
			{
				Name:        dialogueGuildUploadAudioJobName,
				Status:      "completed",
				CompletedAt: masterItem.CreatedAt.Format(time.RFC3339),
			},
		},
		CreatedAt: masterItem.CreatedAt.Format(time.RFC3339),
		Result:    resultJSON,
	}

	return batch, nil
}
