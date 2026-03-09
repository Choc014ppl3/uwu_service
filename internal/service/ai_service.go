package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
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
	userStatsRepo      repository.UserStatsRepository
	batchService       *BatchService
}

// NewAIService creates a new AI service.
func NewAIService(
	geminiClient *client.GeminiClient,
	cloudflareClient *client.CloudflareClient,
	azureSpeechClient *client.AzureSpeechClient,
	learningItemRepo repository.LearningItemRepository,
	learningSourceRepo repository.LearningSourceRepository,
	userStatsRepo repository.UserStatsRepository,
	batchService *BatchService,
) *AIService {
	return &AIService{
		geminiClient:       geminiClient,
		cloudflareClient:   cloudflareClient,
		azureSpeechClient:  azureSpeechClient,
		learningItemRepo:   learningItemRepo,
		learningSourceRepo: learningSourceRepo,
		userStatsRepo:      userStatsRepo,
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
	Language   string `json:"language"`    // "en-US", "zh-CN"
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
- Target Language: "{{language}}" (The language being learned)
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
5. All other fields (metadata, tags): MUST be in the Target Language ("{{language}}").
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
	prompt = strings.ReplaceAll(prompt, "{{language}}", req.Language)
	prompt = strings.ReplaceAll(prompt, "{{native_lang}}", req.NativeLang)

	return prompt
}

// GenerateDialogueGuideReq defines the request for generating a dialogue guide.
type GenerateDialogueGuideReq struct {
	Topic       string   `json:"topic"`
	Description string   `json:"description"`
	Language    string   `json:"language"`
	Level       string   `json:"level"`
	Tags        []string `json:"tags"`
}

// DialogueGuideResponse represents the expected parsed structure from Gemini API
type DialogueGuideResponse struct {
	ImagePrompt string          `json:"image_prompt"`
	Level       string          `json:"level"`
	Tags        []string        `json:"tags"`
	SpeechMode  json.RawMessage `json:"speech_mode"`
	ChatMode    json.RawMessage `json:"chat_mode"`
}

const (
	dialogueGuideJobName              = "generate_dialogue_guide"
	dialogueGuideImageJobName         = "generate_image"
	dialogueGuideUploadJobName        = "upload_image"
	dialogueGuideAudioJobName         = "generate_audio"
	dialogueGuideUploadAudioJobName   = "upload_audio"
	dialogueGuideAudioScriptsJobName  = "generate_audio_scripts"
	dialogueGuideUploadScriptsJobName = "upload_audio_scripts"
)

// GenerateDialogueGuide initiates speech and chat conversations generation for a dialogue guide using Gemini asynchronously.
// Returns a batch_id immediately which can be used to poll for the result.
func (s *AIService) GenerateDialogueGuide(ctx context.Context, userID, topic, description, language, level string, tags []string) (string, error) {
	if s.geminiClient == nil {
		return "", errors.New(errors.ErrAIService, "Gemini client not configured")
	}

	batchID := uuid.New().String()

	// Create batch with a single job
	if s.batchService != nil {
		_ = s.batchService.CreateBatchWithJobs(ctx, batchID, topic, []string{
			dialogueGuideJobName,
			dialogueGuideImageJobName,
			dialogueGuideUploadJobName,
			dialogueGuideAudioJobName,
			dialogueGuideUploadAudioJobName,
			dialogueGuideAudioScriptsJobName,
			dialogueGuideUploadScriptsJobName,
		})
	}

	// Run processing in background
	go s.processDialogueGuideAsync(batchID, userID, topic, description, language, level, tags)

	return batchID, nil
}

// processDialogueGuideAsync runs the AI call, parses, saves to DB, and updates the batch status.
func (s *AIService) processDialogueGuideAsync(batchID, userID, topic, description, language, level string, tags []string) {
	ctx := context.Background()

	if s.batchService != nil {
		_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideJobName, "processing", "")
	}

	/** แบบที่ 1: Still Life / Macro Object (เน้นสิ่งของ สวยคมชัด)
	  ## 1. Content Generation Logic
	  * **Image Prompt:** Create a prompt for a text-to-image model in English.
		* *Style:* **Vertical 9:16 aspect ratio, Macro photography, Close-up still life shot, Cinematic lighting, Highly detailed textures, Shallow depth of field (bokeh background), Wallpaper aesthetic, Strictly no people, No text/words in image.**
		* *Content:* Strictly depict a **close-up view of specific objects or details** central to the conversation topic and description (e.g., a steaming coffee cup on a wooden table, a hotel room key card on a marble counter, a pen signing a document). The background should be heavily blurred but subtly hint at the location's atmosphere.

	/** แบบที่ 2: Dynamic Mood (คุมโทนแสงตามอารมณ์บทสนทนา)
	  ## 1. Content Generation Logic
	  * **Image Prompt:** Create a prompt for a text-to-image model in English.
		* *Style:* **Vertical 9:16 aspect ratio, Macro photography, Close-up still life shot, Cinematic lighting, Highly detailed textures, Shallow depth of field (bokeh background), Wallpaper aesthetic, Strictly no people, No text/words in image.**
		* *Content:* Strictly depict a **close-up view of specific objects or details** central to the conversation topic and description (e.g., a steaming coffee cup, a hotel room key card, business documents). The background should be heavily blurred but subtly hint at the location's atmosphere. **Adjust the lighting and color grading to match the emotional tone and difficulty level of the situation (e.g., bright and cozy for a friendly chat, moody and tense for a dispute).**
	*/

	/** แบบที่ 3: First-Person POV (มุมมองสายตาเรา เบลอคนอื่น)
	  ## 1. Content Generation Logic
	  * **Image Prompt:** Create a prompt for a text-to-image model in English.
	    * *Style:* **Vertical 9:16 aspect ratio, First-person POV shot (from the user's perspective), Shallow depth of field, Extreme bokeh background, Cinematic lighting, Wallpaper aesthetic, No clear faces, Faceless composition, No text/words in image.**
	    * *Content:* Depict the scene as if the user is looking at it. User's hands might be partially visible in the foreground performing an action related to the topic (e.g., holding a menu, handing over a credit card). Any people in the background (like staff or other customers) must be completely blurred or obscured to avoid showing clear faces.
	*/

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
	* *Style:* **Vertical 9:16 aspect ratio, Macro photography, Close-up still life shot, Cinematic lighting, Highly detailed textures, Shallow depth of field (bokeh background), Wallpaper aesthetic, Strictly no people, No text/words in image.**
	* *Content:* Strictly depict a **close-up view of specific objects or details** central to the conversation topic and description (e.g., a steaming coffee cup on a wooden table, a hotel room key card on a marble counter, a pen signing a document). The background should be heavily blurred but subtly hint at the location's atmosphere.
* **Speech Mode (Script) - OPTIMIZED FOR LEARNING:**
    * **Length Constraint:** Generate **ONLY 6-10 turns for Beginner level, 10-16 turns for Intermediate level, and 16-24 turns for Advanced level**. Keep it concise.
    * **Cognitive Load Control:** Ensure each "user" turn is **1-3 sentences max**. Avoid long monologues (too hard to memorize) and avoid single words (too easy).
    * **Create a realistic dialogue where the User has a clear goal. The AI should guide the conversation naturally.**
* **Chat Mode (Objectives):**
    * Create a "Mission" based on the same scenario.
    * Ensure the objectives (requirements/persuasion) are smooth, logical, and match the difficulty level detected.

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
  }
}`

	tagsStr := ""
	if len(tags) > 0 {
		tagsStr = strings.Join(tags, ", ")
	}

	prompt := strings.ReplaceAll(promptTemplate, "{{TOPIC}}", topic)
	prompt = strings.ReplaceAll(prompt, "{{DESCRIPTION}}", description)
	prompt = strings.ReplaceAll(prompt, "{{LANGUAGE}}", language)
	prompt = strings.ReplaceAll(prompt, "{{LEVEL}}", level)
	prompt = strings.ReplaceAll(prompt, "{{TAGS}}", tagsStr)

	respText, err := s.geminiClient.Chat(ctx, prompt)
	if err != nil {
		if s.batchService != nil {
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideJobName, "failed", err.Error())
		}
		return
	}

	cleanResp := strings.TrimSpace(respText)
	cleanResp = strings.TrimPrefix(cleanResp, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```")

	var parsedResp DialogueGuideResponse
	if err := json.Unmarshal([]byte(cleanResp), &parsedResp); err != nil {
		// Log error but we still try to save the raw response or update batch
		fmt.Printf("Warning: failed to unmarshal DialogueGuideResponse: %v\n", err)
		if s.batchService != nil {
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideJobName, "failed", "failed to parse DialogueGuideResponse JSON: "+err.Error())
		}
		return
	}

	// Parse SpeechMode JSON to extract situation and scripts
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

	// Create Learning Item FIRST to get the database ID for R2 paths
	var learningItemID string
	var li *repository.LearningItem
	if s.learningItemRepo != nil {
		featureID := repository.DialogueGuide
		levelPtr := level
		if level == "" {
			levelPtr = parsedResp.Level
		}

		tagsBytes, _ := json.Marshal(parsedResp.Tags)

		// Initial details
		detailsMap := map[string]interface{}{
			"topic":        topic,
			"description":  description,
			"speech_mode":  parsedResp.SpeechMode, // Original, will be updated later
			"chat_mode":    parsedResp.ChatMode,
			"image_prompt": parsedResp.ImagePrompt,
			"tags":         parsedResp.Tags,
		}

		// Initial metadata
		metadataMap := map[string]interface{}{
			"batch_id":          batchID,
			"user_id":           userID,
			"processing_status": "processing",
		}

		metadataBytes, _ := json.Marshal(metadataMap)
		detailsBytes, _ := json.Marshal(detailsMap)

		li = &repository.LearningItem{
			FeatureID: &featureID,
			Content:   topic,
			Language:  language,
			Level:     levelPtr,
			Tags:      tagsBytes,
			Metadata:  metadataBytes,
			Details:   detailsBytes,
			IsActive:  true,
		}
		if err := s.learningItemRepo.Create(ctx, li); err != nil {
			fmt.Printf("Warning: failed to create learning item: %v\n", err)
		} else {
			learningItemID = li.ID.String()
		}
	}

	// 4. Generate All Media Files in Parallel
	var imageURL, audioURL string
	var mediaWg sync.WaitGroup
	var mediaMu sync.Mutex

	// 4a. Image Generation (goroutine)
	if parsedResp.ImagePrompt != "" && s.geminiClient != nil && learningItemID != "" {
		mediaWg.Add(1)
		go func() {
			defer mediaWg.Done()
			if s.batchService != nil {
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideImageJobName, "processing", "")
			}

			imageBytes, err := s.geminiClient.GenerateImage(ctx, parsedResp.ImagePrompt)
			if err != nil {
				fmt.Printf("Warning: failed to generate image: %v\n", err)
				if s.batchService != nil {
					_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideImageJobName, "failed", err.Error())
					_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadJobName, "failed", "Skipped due to generation failure")
				}
				return
			}

			if s.batchService != nil {
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideImageJobName, "completed", "")
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadJobName, "processing", "")
			}

			if s.cloudflareClient != nil {
				objectKey := fmt.Sprintf("images/%s.png", learningItemID)
				url, err := s.cloudflareClient.UploadR2Object(ctx, objectKey, imageBytes, "image/png")
				if err != nil {
					fmt.Printf("Warning: failed to upload image: %v\n", err)
					if s.batchService != nil {
						_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadJobName, "failed", err.Error())
					}
				} else {
					mediaMu.Lock()
					imageURL = url
					mediaMu.Unlock()
					if s.batchService != nil {
						_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadJobName, "completed", "")
					}
				}
			} else {
				if s.batchService != nil {
					_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadJobName, "failed", "Cloudflare client not configured")
				}
			}
		}()
	} else {
		// skip image jobs if no necessary components
		if s.batchService != nil {
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideImageJobName, "completed", "")
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadJobName, "completed", "")
		}
	}

	// 4b. Situation Audio Generation (goroutine)
	if situationText != "" && s.azureSpeechClient != nil && learningItemID != "" {
		mediaWg.Add(1)
		go func() {
			defer mediaWg.Done()
			if s.batchService != nil {
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideAudioJobName, "processing", "")
			}

			audioBytes, err := s.azureSpeechClient.Synthesize(ctx, situationText, "en-US-AvaMultilingualNeural")
			if err != nil {
				fmt.Printf("Warning: failed to generate audio: %v\n", err)
				if s.batchService != nil {
					_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideAudioJobName, "failed", err.Error())
					_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadAudioJobName, "failed", "Skipped due to generation failure")
				}
				return
			}

			if s.batchService != nil {
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideAudioJobName, "completed", "")
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadAudioJobName, "processing", "")
			}

			if s.cloudflareClient != nil {
				objectKey := fmt.Sprintf("speech-context/%s.mp3", learningItemID)
				url, err := s.cloudflareClient.UploadR2Object(ctx, objectKey, audioBytes, "audio/mpeg")
				if err != nil {
					fmt.Printf("Warning: failed to upload audio: %v\n", err)
					if s.batchService != nil {
						_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadAudioJobName, "failed", err.Error())
					}
				} else {
					mediaMu.Lock()
					audioURL = url
					mediaMu.Unlock()
					if s.batchService != nil {
						_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadAudioJobName, "completed", "")
					}
				}
			} else {
				if s.batchService != nil {
					_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadAudioJobName, "failed", "Cloudflare client not configured")
				}
			}
		}()
	} else {
		if s.batchService != nil {
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideAudioJobName, "completed", "")
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadAudioJobName, "completed", "")
		}
	}

	// 4c. Script Audio Generation (goroutine per AI script line)
	var scriptsHasError bool
	var scriptsLastErr error
	if len(speechModeMap) > 0 && s.azureSpeechClient != nil && s.cloudflareClient != nil && learningItemID != "" {
		if scriptObj, ok := speechModeMap["script"]; ok {
			if scripts, ok := scriptObj.([]interface{}); ok {
				if s.batchService != nil {
					_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideAudioScriptsJobName, "processing", "")
					_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadScriptsJobName, "processing", "")
				}

				for i := range scripts {
					if scriptLine, ok := scripts[i].(map[string]interface{}); ok {
						speaker, _ := scriptLine["speaker"].(string)
						text, _ := scriptLine["text"].(string)

						if speaker == "AI" && text != "" {
							mediaWg.Add(1)
							go func(idx int, scriptText string) {
								defer mediaWg.Done()
								audioBytes, err := s.azureSpeechClient.Synthesize(ctx, scriptText, "en-US-AvaMultilingualNeural")
								if err != nil {
									mediaMu.Lock()
									scriptsHasError = true
									scriptsLastErr = err
									mediaMu.Unlock()
									fmt.Printf("Warning: failed to generate script audio: %v\n", err)
									return
								}

								objectKey := fmt.Sprintf("speech-script/%s-%d.mp3", learningItemID, idx)
								url, err := s.cloudflareClient.UploadR2Object(ctx, objectKey, audioBytes, "audio/mpeg")
								if err != nil {
									mediaMu.Lock()
									scriptsHasError = true
									scriptsLastErr = err
									mediaMu.Unlock()
									fmt.Printf("Warning: failed to upload script audio: %v\n", err)
									return
								}

								mediaMu.Lock()
								scripts[idx].(map[string]interface{})["audio_url"] = url
								mediaMu.Unlock()
							}(i, text)
						}
					}
				}
			}
		}
	} else {
		if s.batchService != nil {
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideAudioScriptsJobName, "completed", "")
			_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadScriptsJobName, "completed", "")
		}
	}

	// Wait for ALL media generation goroutines to complete
	mediaWg.Wait()

	// Update script job statuses after wait
	if len(speechModeMap) > 0 {
		if scriptObj, ok := speechModeMap["script"]; ok {
			if scripts, ok := scriptObj.([]interface{}); ok {
				speechModeMap["script"] = scripts
			}
		}
		if s.batchService != nil {
			if scriptsHasError {
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideAudioScriptsJobName, "failed", scriptsLastErr.Error())
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadScriptsJobName, "failed", scriptsLastErr.Error())
			} else {
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideAudioScriptsJobName, "completed", "")
				_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideUploadScriptsJobName, "completed", "")
			}
		}
	}

	// 7. Update Learning Item with final audio URLs and repacked speech_mode
	updatedSpeechModeBytes, _ := json.Marshal(speechModeMap)

	if li != nil && learningItemID != "" {
		// Update details with repacked speech_mode (includes audio_url on AI script turns)
		// Update metadata with audio URLs
		var detailsMap map[string]interface{}
		if len(li.Details) > 0 {
			_ = json.Unmarshal(li.Details, &detailsMap)
		}
		detailsMap["speech_mode"] = json.RawMessage(updatedSpeechModeBytes)
		detailsMap["chat_mode"] = parsedResp.ChatMode

		// Update metadata with audio URLs
		var metadataMap map[string]interface{}
		if len(li.Metadata) > 0 {
			_ = json.Unmarshal(li.Metadata, &metadataMap)
		}
		if metadataMap == nil {
			metadataMap = map[string]interface{}{}
		}
		if imageURL != "" {
			metadataMap["image_url"] = imageURL
		}
		if audioURL != "" {
			metadataMap["audio_url"] = audioURL
		}
		metadataMap["processing_status"] = "completed"

		detailsBytes, _ := json.Marshal(detailsMap)
		metadataBytes, _ := json.Marshal(metadataMap)
		li.Metadata = metadataBytes
		li.Details = detailsBytes

		if err := s.learningItemRepo.Update(ctx, li); err != nil {
			fmt.Printf("Warning: failed to update learning item with audio URLs: %v\n", err)
		}
	}

	// 8. Build final client response JSON and set batch result
	var cleanRespMap map[string]interface{}
	if err := json.Unmarshal([]byte(cleanResp), &cleanRespMap); err == nil {
		if imageURL != "" {
			cleanRespMap["image_url"] = imageURL
		}
		if audioURL != "" {
			cleanRespMap["audio_url"] = audioURL
		}
		cleanRespMap["speech_mode"] = speechModeMap

		if updatedBytes, err := json.Marshal(cleanRespMap); err == nil {
			cleanResp = string(updatedBytes)
		}
	}

	if s.batchService != nil {
		_ = s.batchService.SetBatchResult(ctx, batchID, []byte(cleanResp))
		_ = s.batchService.UpdateJob(ctx, batchID, dialogueGuideJobName, "completed", "")
	}
}

// GetDialogueGuideByBatchID reconstructs the batch response from the database if the metadata has been archived.
func (s *AIService) GetDialogueGuideByBatchID(ctx context.Context, batchID string) (*BatchStatus, error) {
	if s.learningItemRepo == nil || s.learningSourceRepo == nil {
		return nil, nil // Return nil to signal not found or not configured
	}

	items, err := s.learningItemRepo.GetByBatchID(ctx, batchID)
	if err != nil || len(items) == 0 {
		return nil, nil // Not found
	}
	masterItem := items[0]

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
		"speech_mode":  details["speech_mode"],
		"chat_mode":    details["chat_mode"],
		"level":        masterItem.Level,
		"tags":         tags,
	}

	// Assemble final BatchStatus structure matching Redis model
	resultJSON, _ := json.Marshal(resultMap)

	batch := &BatchStatus{
		BatchID:       batchID,
		ReferenceID:   masterItem.Content, // Use topic as reference ID
		Status:        "completed",        // If it's in the DB, it's considered completed
		TotalJobs:     7,
		CompletedJobs: 7,
		Jobs: []JobStatus{
			{
				Name:        dialogueGuideJobName,
				Status:      "completed",
				CompletedAt: masterItem.CreatedAt.Format(time.RFC3339),
			},
			{
				Name:        dialogueGuideImageJobName,
				Status:      "completed",
				CompletedAt: masterItem.CreatedAt.Format(time.RFC3339),
			},
			{
				Name:        dialogueGuideUploadJobName,
				Status:      "completed",
				CompletedAt: masterItem.CreatedAt.Format(time.RFC3339),
			},
			{
				Name:        dialogueGuideAudioJobName,
				Status:      "completed",
				CompletedAt: masterItem.CreatedAt.Format(time.RFC3339),
			},
			{
				Name:        dialogueGuideUploadAudioJobName,
				Status:      "completed",
				CompletedAt: masterItem.CreatedAt.Format(time.RFC3339),
			},
			{
				Name:        dialogueGuideAudioScriptsJobName,
				Status:      "completed",
				CompletedAt: masterItem.CreatedAt.Format(time.RFC3339),
			},
			{
				Name:        dialogueGuideUploadScriptsJobName,
				Status:      "completed",
				CompletedAt: masterItem.CreatedAt.Format(time.RFC3339),
			},
		},
		CreatedAt: masterItem.CreatedAt.Format(time.RFC3339),
		Result:    resultJSON,
	}

	return batch, nil
}

// SubmitDialogueSpeechWithR2 handles audio cleanup, conversion, R2 upload, and Azure speech analysis.
func (s *AIService) SubmitDialogueSpeechWithR2(ctx context.Context, wavData []byte, referenceText, langCode, learningItemID, userID, speechIndex string) (map[string]interface{}, error) {
	if s.azureSpeechClient == nil {
		return nil, errors.New(errors.ErrAIService, "Azure Speech client not configured")
	}

	// 1. Write the incoming wav to a temp file
	tmpWav, err := os.CreateTemp("", "speech_*.wav")
	if err != nil {
		return nil, errors.Wrap(errors.ErrInternal, "failed to create temp wav file", err)
	}

	if _, err := tmpWav.Write(wavData); err != nil {
		tmpWav.Close()
		return nil, errors.Wrap(errors.ErrInternal, "failed to write temp wav data", err)
	}
	tmpWav.Close()

	// 2. Analyze with Azure Speech (using the original WAV data synchronously)
	result, err := s.azureSpeechClient.AnalyzeShadowingAudio(ctx, wavData, referenceText, langCode)
	if err != nil {
		// If analysis fails, we don't care about background process much.
		return nil, errors.Wrap(errors.ErrAIService, "failed to analyze shadowing audio", err)
	}

	// 3. Append deterministic audio URL to the result
	// We know where the background worker is putting it
	if s.cloudflareClient != nil {
		r2URL := fmt.Sprintf("%s/user-input/%s/%s/%s.m4a", s.cloudflareClient.PublicURL(), userID, learningItemID, speechIndex)
		result["user_audio_url"] = r2URL
	}

	// 4. Process and convert audio using ffmpeg in background
	go s.processSpeechAudioUpload(userID, learningItemID, speechIndex, tmpWav.Name())

	// 5. Fire background worker for learning sources and user stats
	go s.processSpeechAnalysisResults(userID, referenceText, langCode, result)

	return result, nil
}

// processSpeechAudioUpload handles uploading user speech to Cloudflare R2.
func (s *AIService) processSpeechAudioUpload(userID, learningItemID, speechIndex, tmpName string) {
	// ใช้ Context แยก เพื่อไม่ให้ผูกกับ Request context เดิมที่อาจจะถูกยกเลิกไปแล้ว
	bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	m4aPath := tmpName + ".m4a"
	defer func() {
		os.Remove(tmpName)
		os.Remove(m4aPath)
	}()

	cmd := exec.CommandContext(bgCtx, "ffmpeg", "-y", "-i", tmpName,
		"-af", "afftdn,loudnorm=I=-16:TP=-1.5:LRA=11",
		"-c:a", "aac", "-b:a", "64k", "-movflags", "faststart",
		m4aPath,
	)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Warning: background audio conversion failed: %v\n", err)
		return
	}

	m4aData, err := os.ReadFile(m4aPath)
	if err != nil {
		fmt.Printf("Warning: failed to read converted m4a in background: %v\n", err)
		return
	}

	// Upload to Cloudflare R2
	if s.cloudflareClient != nil {
		r2Path := fmt.Sprintf("user-input/%s/%s/%s.m4a", userID, learningItemID, speechIndex)
		_, err = s.cloudflareClient.UploadR2Object(bgCtx, r2Path, m4aData, "audio/m4a")
		if err != nil {
			fmt.Printf("Warning: failed to upload user speech to R2 in background: %v\n", err)
		}
	} else {
		fmt.Printf("Warning: cloudflare client not configured for user speech upload\n")
	}
}

// processSpeechAnalysisResults handles checking/creating learning sources and updating user stats based on audio evaluation.
func (s *AIService) processSpeechAnalysisResults(userIDStr, referenceText, langCode string, result map[string]interface{}) {
	if s.learningSourceRepo == nil || s.userStatsRepo == nil {
		return
	}

	ctx := context.Background()

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		fmt.Printf("Background worker error: invalid userID: %s\n", err)
		return
	}

	normalizedLang := "english" // default
	if langCode == "zh-CN" {
		normalizedLang = "chinese"
	}

	// Helper to calculate increments
	getWordIncrements := func(score float64, errType string) (listen, speak, read, write float64) {
		listen = 0.0
		read = 0.5
		write = 0.0
		speak = 0.0

		if score >= 85 && errType == "None" {
			speak = 1.0
		} else if score >= 60 || errType == "Insertion" {
			speak = 0.5
		} else if errType == "Mispronunciation" {
			speak = 0.25
		}

		return listen, speak, read, write
	}

	// We'll collect missing words/sentences here to generate their details in one prompt
	type missingItem struct {
		Content   string
		Type      repository.LearningSourceType
		Score     float64
		ErrorType string
	}
	var missingItems []missingItem

	// 1. Process Reference Sentence
	// The sentence doesn't have a specific score in STT word breakdown, so we'll use a neutral or combined metric, or just +0.5 for reading.
	// For this requirement, we'll try to use the overall pronunciation score if available, else just base values
	var overallScore float64
	if pronScoreObj, ok := result["pronunciation_score"]; ok {
		if val, err := parseToFloat(pronScoreObj); err == nil {
			overallScore = val
		}
	}
	listen, speak, read, write := getWordIncrements(overallScore, "None")

	lsArr, err := s.learningSourceRepo.GetByContentsAndLanguage(ctx, []string{referenceText}, normalizedLang)
	if err == nil && len(lsArr) > 0 {
		ls := lsArr[0]
		// Exists, update stats directly
		err = s.userStatsRepo.UpsertUserStat(ctx, ls.ID, userID, referenceText, normalizedLang, "sentence", listen, speak, read, write)
		if err != nil {
			fmt.Printf("Warning: failed to upsert user stat for existing sentence: %v\n", err)
		}
	} else {
		missingItems = append(missingItems, missingItem{
			Content:   referenceText,
			Type:      repository.LearningSourceTypeSentence,
			Score:     overallScore,
			ErrorType: "None",
		})
	}

	// 2. Process Extracted Words
	if nbestObj, ok := result["NBest"].([]interface{}); ok && len(nbestObj) > 0 {
		if firstNBest, ok := nbestObj[0].(map[string]interface{}); ok {
			if wordsArr, ok := firstNBest["Words"].([]interface{}); ok {
				// collect all words
				var words []string
				for _, wObj := range wordsArr {
					if wMap, ok := wObj.(map[string]interface{}); ok {
						if word, ok := wMap["Word"].(string); ok {
							words = append(words, word)
						}
					}
				}

				// get existing sources
				existingSources, _ := s.learningSourceRepo.GetByContentsAndLanguage(ctx, words, normalizedLang)
				sourceMap := make(map[string]*repository.LearningSource)
				for _, src := range existingSources {
					sourceMap[strings.ToLower(src.Content)] = src
				}

				// now iterate again to apply math and map missing items
				for _, wObj := range wordsArr {
					if wMap, ok := wObj.(map[string]interface{}); ok {
						wordText, _ := wMap["Word"].(string)
						errType, _ := wMap["ErrorType"].(string)

						var accScore float64
						if pronObj, ok := wMap["PronunciationAssessment"].(map[string]interface{}); ok {
							if val, err := parseToFloat(pronObj["AccuracyScore"]); err == nil {
								accScore = val
							}
						}

						wListen, wSpeak, wRead, wWrite := getWordIncrements(accScore, errType)

						if src, exists := sourceMap[strings.ToLower(wordText)]; exists {
							err = s.userStatsRepo.UpsertUserStat(ctx, src.ID, userID, wordText, normalizedLang, "word", wListen, wSpeak, wRead, wWrite)
							if err != nil {
								fmt.Printf("Warning: failed to upsert user stat for existing word: %v\n", err)
							}
						} else {
							missingItems = append(missingItems, missingItem{
								Content:   wordText,
								Type:      repository.LearningSourceTypeWord,
								Score:     accScore,
								ErrorType: errType,
							})
						}
					}
				}
			}
		}
	}

	// 3. Generate missing using Gemini AI
	if len(missingItems) > 0 {
		var contentList []string
		for _, mi := range missingItems {
			contentList = append(contentList, fmt.Sprintf("- [%s] %s", mi.Type, mi.Content))
		}

		prompt := fmt.Sprintf(`Generate learning details for the exact list of %s vocabulary and sentences provided below.
Do not add or remove any items; only generate details for the provided items.

STRICT RULES:
1. Language Interpretation: If the requested language is a locale code (e.g., "en-US", "zh-CN", "ja-JP"), you MUST treat it as the primary language name (e.g., "English", "Chinese", "Japanese") when generating content.
2. Proficiency Level Format: For the "level" field, you MUST strictly use official proficiency frameworks appropriate for the language:
   - CEFR (A1, A2, B1, B2, C1, C2) for European languages (English, Spanish, French, etc.).
   - HSK (HSK1, HSK2, HSK3, HSK4, HSK5, HSK6) for Mandarin Chinese.
   - JLPT (N5, N4, N3, N2, N1) for Japanese.
   - TOPIK (1, 2, 3, 4, 5, 6) for Korean.
   DO NOT use generic terms like "Beginner", "Intermediate", or "Advanced".
3. Output Format: You MUST format your output EXACTLY as the following JSON structure, with no extra markdown or conversational text.

{
  "words": [
    {
      "text": "string", // The exact word requested from the list. Do not modify it.
      "level": "string", // STRICTLY standard level (e.g., "A1", "B2", "HSK3"). No generic terms.
      "tags": ["string"], // Array of 2-3 relevant categories or topics (e.g., ["travel", "food", "verbs"]).
      "reading_standard": "string", // Standard pronunciation guide (e.g., Pinyin, Romaji, or IPA).
      "reading_stress": "string", // Phonetic breakdown showing syllable stress, rhythm, or tones.
      "pos": "string", // Part of speech (e.g., "noun", "verb", "adjective", "conjunction").
      "definition": "string", // Clear and concise dictionary definition or translation.
      "ex_sentence": "string" // A natural, contextual example sentence using the exact word.
    }
  ],
  "sentences": [
    {
      "text": "string", // The exact sentence requested from the list. Do not modify it.
      "level": "string", // STRICTLY standard level (e.g., "A1", "B2", "HSK3"). No generic terms.
      "tags": ["string"], // Array of 2-3 relevant categories or grammar points (e.g., ["greeting", "past tense"]).
      "reading_standard": "string", // Standard pronunciation guide (e.g., Pinyin, Romaji, or IPA).
      "reading_stress": "string", // Phonetic breakdown showing rhythm, intonation, or tones.
      "structure_format": "string", // Grammar structure or pattern breakdown (e.g., "Subject + Verb + Object").
      "usage": {
        "formality": "string", // Level of formality (e.g., "formal", "casual", "polite", "slang").
        "tone": "string", // Emotional tone or intent (e.g., "friendly", "urgent", "polite request").
        "context": "string", // Brief explanation of the context where this sentence is naturally used.
        "situations": ["string"] // Array of specific, practical scenarios (e.g., ["ordering food", "meeting a friend"]).
      }
    }
  ]
}

Items to generate:
%s`, normalizedLang, contentList)

		aiResponse, err := s.geminiClient.Chat(ctx, prompt)
		if err == nil {
			// clean JSON markdown
			cleanResp := strings.TrimPrefix(aiResponse, "```json")
			cleanResp = strings.TrimPrefix(cleanResp, "```")
			cleanResp = strings.TrimSuffix(cleanResp, "```\n")
			cleanResp = strings.TrimSuffix(cleanResp, "```")
			cleanResp = strings.TrimSpace(cleanResp)

			var parsedResp struct {
				Words []struct {
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

			if jsonErr := json.Unmarshal([]byte(cleanResp), &parsedResp); jsonErr == nil {
				// Insert newly generated words
				for _, w := range parsedResp.Words {
					// find original missing item to get its scores
					var originalItem missingItem
					for _, mi := range missingItems {
						if strings.EqualFold(mi.Content, w.Text) {
							originalItem = mi
							break
						}
					}

					tagsBytes, _ := json.Marshal(w.Tags)
					metadataMap := map[string]interface{}{
						"reading_standard": w.ReadingStandard,
						"reading_stress":   w.ReadingStress,
						"ex_sentence":      w.ExSentence,
						"definition":       w.Definition,
						"pos":              w.POS,
					}
					metadataBytes, _ := json.Marshal(metadataMap)

					newID := uuid.New()
					ls := &repository.LearningSource{
						ID:       newID,
						Content:  w.Text,
						Language: normalizedLang,
						Type:     repository.LearningSourceTypeWord,
						Level:    w.Level,
						Tags:     tagsBytes,
						Metadata: metadataBytes,
					}

					// Attempt to insert
					if err := s.learningSourceRepo.Create(ctx, ls); err == nil {
						lListen, lSpeak, lRead, lWrite := getWordIncrements(originalItem.Score, originalItem.ErrorType)
						statErr := s.userStatsRepo.UpsertUserStat(ctx, newID, userID, w.Text, normalizedLang, "word", lListen, lSpeak, lRead, lWrite)
						if statErr != nil {
							fmt.Printf("Warning: failed to upsert user stat for NEW word: %v\n", statErr)
						}
					} else {
						fmt.Printf("Warning: failed to create new learning source word: %v\n", err)
					}
				}

				// Insert newly generated sentences
				for _, st := range parsedResp.Sentences {
					var originalItem missingItem
					for _, mi := range missingItems {
						if strings.EqualFold(mi.Content, st.Text) {
							originalItem = mi
							break
						}
					}

					tagsBytes, _ := json.Marshal(st.Tags)
					metadataMap := map[string]interface{}{
						"reading_standard": st.ReadingStandard,
						"reading_stress":   st.ReadingStress,
						"structure_format": st.StructureFormat,
						"usage":            st.Usage,
					}
					metadataBytes, _ := json.Marshal(metadataMap)

					newID := uuid.New()
					ls := &repository.LearningSource{
						ID:       newID,
						Content:  st.Text,
						Language: normalizedLang,
						Type:     repository.LearningSourceTypeSentence,
						Level:    st.Level,
						Tags:     tagsBytes,
						Metadata: metadataBytes,
					}

					if err := s.learningSourceRepo.Create(ctx, ls); err == nil {
						lListen, lSpeak, lRead, lWrite := getWordIncrements(originalItem.Score, originalItem.ErrorType)
						statErr := s.userStatsRepo.UpsertUserStat(ctx, newID, userID, st.Text, normalizedLang, "sentence", lListen, lSpeak, lRead, lWrite)
						if statErr != nil {
							fmt.Printf("Warning: failed to upsert user stat for NEW sentence: %v\n", statErr)
						}
					} else {
						fmt.Printf("Warning: failed to create new learning source sentence: %v\n", err)
					}
				}
			} else {
				fmt.Printf("Warning: failed to unmarshal AI generated learning_sources mapping: %v\n", jsonErr)
			}
		} else {
			fmt.Printf("Warning: failing Gemini generation for missing sources: %v\n", err)
		}
	}
}

// Helper to safely parse float64 out of various JSON types
func parseToFloat(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("could not parse to float")
	}
}

// ChatEntry represents a single turn in the chat history.
type ChatEntry struct {
	Role string `json:"role"` // "user" or "model"
	Text string `json:"text"`
}

// SubmitDialogueChatResponse represents the API response for chat mode.
type SubmitDialogueChatResponse struct {
	AIResponse          string                 `json:"ai_response"`
	TurnCount           int                    `json:"turn_count"`
	IsCompleted         bool                   `json:"is_completed"`
	ObjectivesCompleted map[string]interface{} `json:"objectives_completed"`
}

// SubmitDialogueChat handles stateless chat iterations for dialogue guides.
func (s *AIService) SubmitDialogueChat(ctx context.Context, userIDStr, learningItemID, currentInput string, history []ChatEntry, maxTurns int) (*SubmitDialogueChatResponse, error) {
	if s.geminiClient == nil {
		return nil, errors.New(errors.ErrAIService, "Gemini client not configured")
	}

	itemID, err := uuid.Parse(learningItemID)
	if err != nil {
		return nil, errors.Wrap(errors.ErrValidation, "invalid learning_item_id", err)
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, errors.Wrap(errors.ErrValidation, "invalid user_id", err)
	}

	// 1. Fetch Learning Item for context (situation & objectives)
	item, err := s.learningItemRepo.GetByID(ctx, itemID)
	if err != nil {
		return nil, errors.Wrap(errors.ErrNotFound, "failed to fetch learning item", err)
	}

	var detailsMap map[string]interface{}
	if len(item.Details) > 0 {
		_ = json.Unmarshal(item.Details, &detailsMap)
	}

	chatModeData, err := json.Marshal(detailsMap["chat_mode"])
	if err != nil {
		return nil, errors.Wrap(errors.ErrInternal, "failed to read chat_mode", err)
	}

	// 2. Format Chat History
	var historyStrBuilder strings.Builder
	for _, entry := range history {
		historyStrBuilder.WriteString(fmt.Sprintf("%s: %s\n", entry.Role, entry.Text))
	}
	historyStrBuilder.WriteString(fmt.Sprintf("user: %s\n", currentInput))

	// 3. Construct Gemini Prompt
	prompt := fmt.Sprintf(`You are an AI roleplaying a conversational partner based on the following context.
Respond to the user naturally based on the situation and evaluate their objectives.

CHAT CONTEXT:
%s

CONVERSATION HISTORY:
%s

Analyze the conversation history, including the user's latest input, to determine if they've met the objectives listed in the chat context.

OUTPUT INSTRUCTIONS:
You MUST format your output EXACTLY as the following JSON structure, with no extra markdown or conversational text.

{
  "reply": "Your natural conversational response here as the other person in the scenario.",
  "evaluation": {
    "persuasion": [false, false], // Array of booleans corresponding exactly to the "persuasion" array in chat context. True if met.
    "constraints": [true, true], // Array of booleans corresponding exactly to the "constraints" array. True if adhered to.
    "requirements": [true, false, false] // Array of booleans corresponding exactly to the "requirements" array. True if accomplished.
  }
}
`, string(chatModeData), historyStrBuilder.String())

	// 4. Send to Gemini
	aiResponse, err := s.geminiClient.Chat(ctx, prompt)
	if err != nil {
		return nil, errors.Wrap(errors.ErrAIService, "failed to get AI chat response", err)
	}

	// 5. Parse AI Response
	cleanResp := strings.TrimPrefix(aiResponse, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```\n")
	cleanResp = strings.TrimSuffix(cleanResp, "```")
	cleanResp = strings.TrimSpace(cleanResp)

	var parsedResp struct {
		Reply      string                 `json:"reply"`
		Evaluation map[string]interface{} `json:"evaluation"`
	}

	if err := json.Unmarshal([]byte(cleanResp), &parsedResp); err != nil {
		// Fallback if AI fails to output strict JSON
		fmt.Printf("Warning: failed to unmarshal AI chat response: %v\n", err)
		parsedResp.Reply = aiResponse // Send raw text as reply fallback
		parsedResp.Evaluation = make(map[string]interface{})
	}

	// 6. Calculate Turn Count and Completion
	turnCount := (len(history) / 2) + 1
	isCompleted := false

	if maxTurns > 0 && turnCount >= maxTurns {
		isCompleted = true
	}

	// Check if all objectives are met
	if !isCompleted {
		allMet := true
		for _, v := range parsedResp.Evaluation {
			if boolArray, ok := v.([]interface{}); ok {
				for _, b := range boolArray {
					if val, isBool := b.(bool); isBool && !val {
						allMet = false
						break
					}
				}
			}
			if !allMet {
				break
			}
		}
		if allMet && len(parsedResp.Evaluation) > 0 {
			isCompleted = true
		}
	}

	// Log chat_attempted to user_actions if completed
	if isCompleted && s.learningItemRepo != nil {
		evaluationJSON, err := json.Marshal(parsedResp.Evaluation)
		if err == nil {
			logErr := s.learningItemRepo.AddUserActionWithMetadata(ctx, itemID, userID, "chat_attempted", evaluationJSON)
			if logErr != nil {
				fmt.Printf("Warning: failed to add chat_attempted user action: %v\n", logErr)
			}
		}
	}

	return &SubmitDialogueChatResponse{
		AIResponse:          parsedResp.Reply,
		TurnCount:           turnCount,
		IsCompleted:         isCompleted,
		ObjectivesCompleted: parsedResp.Evaluation,
	}, nil
}
