package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/repository"
)

// Workout type constants.
const (
	WorkoutMissing        = "missing"
	WorkoutSparringMode   = "sparring_mode"
	WorkoutStructureDrill = "structure_drill"
	WorkoutRhythmFlow     = "rhythm_flow"
	WorkoutVocabReps      = "vocab_reps"
	WorkoutPrecisionCheck = "precision_check"
)

// workoutJobNames defines the batch jobs for media generation.
var workoutJobNames = []string{
	WorkoutMissing, WorkoutSparringMode,
	WorkoutStructureDrill, WorkoutRhythmFlow,
	WorkoutVocabReps, WorkoutPrecisionCheck,
}

const workoutPromptTemplate = `You are a language learning content generator.
Generate a complete workout set for the topic: "%s"
Target language: %s

Additional instructions: %s

Generate exactly 6 items as a JSON object with the following structure:

{
  "missing": {
    "topic": "...",
    "description": "A chat conversation scenario about this topic",
    "image_prompt": "A vivid image prompt for this scenario (no text/words)",
    "difficulty_level": 1-5,
    "script": [
      {"speaker": "ai", "text": "..."},
      {"speaker": "user", "task": "..."},
      {"speaker": "ai", "text": "..."}
    ]
  },
  "sparring_mode": {
    "topic": "...",
    "description": "A speech conversation scenario about this topic",
    "image_prompt": "A vivid image prompt for this scenario (no text/words)",
    "difficulty_level": 1-5,
    "script": [
      {"speaker": "ai", "text": "..."},
      {"speaker": "user", "task": "..."},
      {"speaker": "ai", "text": "..."}
    ]
  },
  "structure_drill": {
    "content": "A sentence in target language",
    "context_type": "sentence",
    "meanings": {"en": "English meaning", "th": "Thai meaning"},
    "reading": {"ipa": "IPA pronunciation"},
    "tags": ["grammar", "structure"],
    "media": {"image_prompt": "vivid image for this sentence (no text/words)"},
    "metadata": {}
  },
  "rhythm_flow": {
    "content": "A phrase in target language",
    "context_type": "phrase",
    "meanings": {"en": "English meaning", "th": "Thai meaning"},
    "reading": {"ipa": "IPA pronunciation"},
    "tags": ["rhythm", "fluency"],
    "media": {"image_prompt": "vivid image for this phrase (no text/words)"},
    "metadata": {}
  },
  "vocab_reps": {
    "content": "A word in target language",
    "context_type": "word",
    "meanings": {"en": "English meaning", "th": "Thai meaning"},
    "reading": {"ipa": "IPA pronunciation"},
    "tags": ["vocabulary"],
    "media": {"image_prompt": "vivid image for this word (no text/words)"},
    "metadata": {}
  },
  "precision_check": {
    "content": "A word in target language",
    "context_type": "word",
    "meanings": {"en": "English meaning", "th": "Thai meaning"},
    "reading": {"ipa": "IPA pronunciation"},
    "tags": ["precision", "pronunciation"],
    "media": {"image_prompt": "vivid image for this word (no text/words)"},
    "metadata": {}
  }
}

IMPORTANT:
- All content must be in the target language (%s)
- Meanings should include both "en" and "th" translations
- Image prompts should be vivid and descriptive
- Scripts should have 6-10 turns
- Respond ONLY with valid JSON, no markdown
`

// WorkoutService handles workout generation.
type WorkoutService struct {
	aiService    *AIService
	scenarioRepo repository.ConversationScenarioRepository
	learningRepo repository.LearningItemRepository
	batchService *BatchService
	chatClient   *client.AzureChatClient
	log          zerolog.Logger
}

// NewWorkoutService creates a new WorkoutService.
func NewWorkoutService(
	aiService *AIService,
	scenarioRepo repository.ConversationScenarioRepository,
	learningRepo repository.LearningItemRepository,
	batchService *BatchService,
	chatClient *client.AzureChatClient,
	log zerolog.Logger,
) *WorkoutService {
	return &WorkoutService{
		aiService:    aiService,
		scenarioRepo: scenarioRepo,
		learningRepo: learningRepo,
		batchService: batchService,
		chatClient:   chatClient,
		log:          log,
	}
}

// PreBriefRequest is the request body for pre-brief generation.
type PreBriefRequest struct {
	WorkoutTopic string `json:"workout_topic"`
	Description  string `json:"description"`
}

// PreBriefResponse is the response from pre-brief generation.
type PreBriefResponse struct {
	PreBriefPrompt string `json:"pre_brief_prompt"`
}

// GeneratePreBrief uses GPT-5 Nano to generate a markdown pre-brief prompt.
func (s *WorkoutService) GeneratePreBrief(ctx context.Context, req PreBriefRequest) (*PreBriefResponse, error) {
	if s.chatClient == nil {
		return nil, fmt.Errorf("GPT-5 Nano client not configured")
	}

	systemPrompt := `You are a language learning curriculum designer.
Given a workout topic and description, generate a detailed pre-brief prompt in Markdown format.
The pre-brief should guide an AI content generator to create high-quality language learning exercises.

Include the following sections:
- **Learning Objectives**: What the learner should achieve
- **Key Vocabulary**: Important words/phrases to cover
- **Grammar Focus**: Sentence patterns or structures to practice
- **Conversation Context**: Real-world situations where this topic applies
- **Difficulty Guidelines**: Appropriate complexity level
- **Cultural Notes**: Any relevant cultural context

Write the output as clean Markdown text. Be specific and actionable.`

	userMessage := fmt.Sprintf("Workout Topic: %s\nDescription: %s", req.WorkoutTopic, req.Description)

	result, err := s.chatClient.ChatCompletion(ctx, systemPrompt, userMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to generate pre-brief: %w", err)
	}

	return &PreBriefResponse{
		PreBriefPrompt: strings.TrimSpace(result),
	}, nil
}

// ConversationGenerateRequest is the request body for conversation generation.
type ConversationGenerateRequest struct {
	Topic           string `json:"topic"`
	Description     string `json:"description"`
	DescriptionType string `json:"description_type"` // "explanation" or "transcription"
}

// ConversationGenerateResponse is the response from conversation generation.
type ConversationGenerateResponse struct {
	BatchID string `json:"batch_id"`
}

const conversationSystemPrompt = `# Role: AI Language Learning Content Generator (JSON API)

You are a strictly formatted backend JSON API driven by an expert linguist and native-speaking language teacher. Your task is to generate highly engaging, culturally accurate, and natural conversational content for Speech Practice and Chat Missions. Ensure the language used is conversational, not purely textbook.

# Input Parameters
* **Topic:** {{TOPIC}}
* **Description:** {{DESCRIPTION}}
* **Language:** {{LANGUAGE}}
* **Level:** {{LEVEL}}
* **Tags:** {{TAGS}} Generate 3-5 relevant keywords describing the conversation context if there are no tags provided.

# Processing Rules

## 1. Content Generation Logic
* **Image Prompt:** Create a prompt for a text-to-image model.
    * *Style:* **Photorealistic, Cinematic lighting, Highly detailed, Lightweight thumbnail image, No text/words in image, No clear picture of a person in image, Organic atmosphere, Natural textures emphasized.**
    * *Content:* **Strictly depict the setting and atmosphere described from the topic, description, and conversation context. Actively integrate natural elements (e.g., plants, wood, stone, natural light, water features, outdoor views) into the scene, even for indoor settings, to create a connection with nature.**
* **Speech Mode (Script) - OPTIMIZED FOR LEARNING:**
    * **Length Constraint:** Generate **ONLY 6-10 turns for Beginner level, 10-16 turns for Intermediate level, and 16-24 turns for Advanced level**. Keep it concise.
    * **Cognitive Load Control:** Ensure each "user" turn is **1-3 sentences max**. Avoid long monologues (too hard to memorize) and avoid single words (too easy).
    * **Create a realistic dialogue where the User has a clear goal. The AI should guide the conversation naturally.
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
  "tags": ["string"] // Generate 3-5 relevant keywords describing the conversation context if there are no tags provided.
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
    "situation": "string", // Brief context to explain the scenario
    "objectives": {
      "requirements": ["string"], // 2-3 Actionable tasks suited to the level
      "persuasion": ["string"], // 1-2 Goals to achieve in the conversation
      "constraints": ["string"] // 1-2 Behavioral/Tonal constraints
    }
  }
}`

const conversationJobName = "generate_conversation"

// GenerateConversation kicks off async conversation generation via GPT-5 Nano.
// Returns a batch_id immediately; poll GET /workouts/batches/{batchID} for result.
func (s *WorkoutService) GenerateConversation(ctx context.Context, req ConversationGenerateRequest) (*ConversationGenerateResponse, error) {
	if s.chatClient == nil {
		return nil, fmt.Errorf("GPT-5 Nano client not configured")
	}

	batchID := uuid.New().String()

	// Create batch with a single job
	_ = s.batchService.CreateBatchWithJobs(ctx, batchID, req.Topic, []string{conversationJobName})

	// Run in background
	go s.processConversationAsync(batchID, req)

	return &ConversationGenerateResponse{
		BatchID: batchID,
	}, nil
}

// processConversationAsync runs the AI call, parses, saves to DB, and updates batch.
func (s *WorkoutService) processConversationAsync(batchID string, req ConversationGenerateRequest) {
	ctx := context.Background()

	_ = s.batchService.UpdateJob(ctx, batchID, conversationJobName, "processing", "")

	userMessage := fmt.Sprintf("Topic: %s\nDescription: %s\nDescription Type: %s", req.Topic, req.Description, req.DescriptionType)

	aiResp, err := s.chatClient.ChatCompletion(ctx, conversationSystemPrompt, userMessage)
	if err != nil {
		s.log.Error().Err(err).Str("batch_id", batchID).Msg("Conversation AI generation failed")
		_ = s.batchService.UpdateJob(ctx, batchID, conversationJobName, "failed", err.Error())
		return
	}

	// Clean response
	cleanResp := strings.TrimSpace(aiResp)
	cleanResp = strings.TrimPrefix(cleanResp, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```")
	cleanResp = strings.TrimSpace(cleanResp)

	// Parse AI response
	var parsed struct {
		Meta struct {
			TargetLang string   `json:"target_lang"`
			Level      string   `json:"level"`
			Tags       []string `json:"tags"`
		} `json:"meta"`
		ImagePrompt string `json:"image_prompt"`
		SpeechMode  struct {
			Script json.RawMessage `json:"script"`
		} `json:"speech_mode"`
		ChatMode json.RawMessage `json:"chat_mode"`
	}
	if err := json.Unmarshal([]byte(cleanResp), &parsed); err != nil {
		s.log.Error().Err(err).Str("raw", cleanResp).Msg("Failed to parse conversation AI response")
		_ = s.batchService.UpdateJob(ctx, batchID, conversationJobName, "failed", "failed to parse AI response: "+err.Error())
		return
	}

	// Save speech scenario
	speechMetadata, _ := json.Marshal(map[string]interface{}{
		"batch_id":     batchID,
		"image_prompt": parsed.ImagePrompt,
		"script":       json.RawMessage(parsed.SpeechMode.Script),
		"level":        parsed.Meta.Level,
		"tags":         parsed.Meta.Tags,
	})
	speechScenario := &repository.ConversationScenario{
		Topic:           req.Topic,
		Description:     req.Description,
		InteractionType: "speech",
		TargetLang:      parsed.Meta.TargetLang,
		EstimatedTurns:  "6-10",
		DifficultyLevel: 1,
		Metadata:        speechMetadata,
		IsActive:        true,
	}
	if err := s.scenarioRepo.Create(ctx, speechScenario); err != nil {
		s.log.Error().Err(err).Msg("Failed to save speech scenario")
		_ = s.batchService.UpdateJob(ctx, batchID, conversationJobName, "failed", "failed to save speech scenario: "+err.Error())
		return
	}

	// Save chat scenario
	chatMetadata, _ := json.Marshal(map[string]interface{}{
		"batch_id":     batchID,
		"image_prompt": parsed.ImagePrompt,
		"chat_mode":    json.RawMessage(parsed.ChatMode),
		"level":        parsed.Meta.Level,
		"tags":         parsed.Meta.Tags,
	})
	chatScenario := &repository.ConversationScenario{
		Topic:           req.Topic,
		Description:     req.Description,
		InteractionType: "chat",
		TargetLang:      parsed.Meta.TargetLang,
		EstimatedTurns:  "6-10",
		DifficultyLevel: 1,
		Metadata:        chatMetadata,
		IsActive:        true,
	}
	if err := s.scenarioRepo.Create(ctx, chatScenario); err != nil {
		s.log.Error().Err(err).Msg("Failed to save chat scenario")
		_ = s.batchService.UpdateJob(ctx, batchID, conversationJobName, "failed", "failed to save chat scenario: "+err.Error())
		return
	}

	// Store result data in batch metadata so client can retrieve it
	resultData, _ := json.Marshal(map[string]interface{}{
		"speech_scenario_id": speechScenario.ID.String(),
		"chat_scenario_id":   chatScenario.ID.String(),
		"data":               json.RawMessage(cleanResp),
	})
	_ = s.batchService.SetBatchResult(ctx, batchID, resultData)

	_ = s.batchService.UpdateJob(ctx, batchID, conversationJobName, "completed", "")
	s.log.Info().Str("batch_id", batchID).Msg("Conversation generation completed")
}

// ConversationBatchResult is the DB-fallback response for expired batches.
type ConversationBatchResult struct {
	BatchID          string                           `json:"batch_id"`
	Status           string                           `json:"status"`
	SpeechScenarioID string                           `json:"speech_scenario_id,omitempty"`
	ChatScenarioID   string                           `json:"chat_scenario_id,omitempty"`
	SpeechScenario   *repository.ConversationScenario `json:"speech_scenario,omitempty"`
	ChatScenario     *repository.ConversationScenario `json:"chat_scenario,omitempty"`
}

// GetScenariosByBatchID retrieves conversation scenarios from DB by batch_id.
func (s *WorkoutService) GetScenariosByBatchID(ctx context.Context, batchID string) (*ConversationBatchResult, error) {
	scenarios, err := s.scenarioRepo.GetByBatchID(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if len(scenarios) == 0 {
		return nil, nil
	}

	result := &ConversationBatchResult{
		BatchID: batchID,
		Status:  "completed",
	}

	for _, sc := range scenarios {
		switch sc.InteractionType {
		case "speech":
			result.SpeechScenarioID = sc.ID.String()
			result.SpeechScenario = sc
		case "chat":
			result.ChatScenarioID = sc.ID.String()
			result.ChatScenario = sc
		}
	}

	return result, nil
}

// LearningItemsGenerateRequest is the request body for learning items generation from scenario.
type LearningItemsGenerateRequest struct {
	ScenarioID string `json:"scenario_id"`
}

// LearningItemsGenerateResponse returns batch_id for async polling.
type LearningItemsGenerateResponse struct {
	BatchID string `json:"batch_id"`
}

const learningItemsJobName = "generate_learning_items"

const learningItemsSystemPrompt = `# Role: Adaptive Learning Content Generator

You are a strict Linguistic Data Extraction API. 
Your task is to analyze the provided **Conversation Script** and **Scenario Context** to generate a JSON Array of learning items.

**Input Parameters:**
- **Context:** The conversation script and situation details provided by the user.
- **Target Level:** "{{level}}" (e.g., B1, B2)
- **Target Topics:** "{{tags}}" (e.g., survival, fishing, outdoor)

---

## **Strict Rules:**

1. **Output Format:** You MUST return a valid **JSON Array** containing multiple learning item objects. No markdown formatting.
2. **Extraction Logic:** You must identify and extract items that fit specific **Learning Categories** (detailed below). Do not generate generic trivia; focus on linguistic utility.
3. **Language:** - ` + "`instruction`" + ` and ` + "`explanations`" + ` must be in **English**.
   - ` + "`meanings`" + ` or translations should be in **English** definitions.

---

## **Learning Categories & Schema Requirements:**

### **1. Category: "rhythm_flow" (For Intonation & Linking)**
- **type**: "rhythm_flow"
- **source_text**: The full sentence from the script.
- **audio_guide**:
    - **stress_marked**: The sentence with CAPS for stressed syllables.
    - **intonation**: Description of the pitch.
- **drill_focus**: "sentence_stress", "linking", or "emotional_inflection".

### **2. Category: "structure_drill" (For Grammar Patterns)**
- **type**: "structure_drill"
- **source_text**: The sentence containing the pattern.
- **pattern_name**: The grammatical concept.
- **structure_formula**: The abstract formula.
- **cloze_test**: The sentence with key element replaced by ` + "`[___]`" + `.

### **3. Category: "vocab_reps" (For Vocabulary Acquisition)**
- **type**: "vocab_reps"
- **word**: The target word (lemma form).
- **pos**: Part of speech.
- **ipa**: IPA pronunciation.
- **definition**: A concise definition suitable for the Target Level.
- **media**:
    - **image_prompt**: A detailed description for generating an image (photorealistic style).

### **4. Category: "precision_check" (For Nuance & Collocation)**
- **type**: "precision_check"
- **phrase**: The specific collocation or phrase.
- **usage_note**: Why this specific wording is used.
- **collocation_partners**: Other words that commonly go with this keyword.

---

## **JSON Output Structure:**

[
  {
    "category": "vocab_reps",
    "item_id": "vocab_001",
    "data": {
      "word": "string",
      "pos": "string",
      "ipa": "string",
      "definition": "string",
      "context_sentence": "string",
      "media": {
        "image_prompt": "string"
      }
    }
  },
  {
    "category": "structure_drill",
    "item_id": "struct_001",
    "data": {
      "source_text": "string",
      "pattern_name": "string",
      "structure_formula": "string",
      "cloze_test": "string"
    }
  },
  {
    "category": "rhythm_flow",
    "item_id": "flow_001",
    "data": {
      "source_text": "string",
      "audio_guide": {
        "stress_marked": "string",
        "intonation": "string"
      },
      "drill_focus": "string"
    }
  },
  {
    "category": "precision_check",
    "item_id": "check_001",
    "data": {
      "phrase": "string",
      "usage_note": "string",
      "collocation_partners": ["string"]
    }
  }
]`

// GenerateLearningItems kicks off async learning item generation from a speech scenario.
func (s *WorkoutService) GenerateLearningItems(ctx context.Context, req LearningItemsGenerateRequest) (*LearningItemsGenerateResponse, error) {
	if s.chatClient == nil {
		return nil, fmt.Errorf("GPT-5 Nano client not configured")
	}

	// Fetch the scenario
	scenarioID, err := uuid.Parse(req.ScenarioID)
	if err != nil {
		return nil, fmt.Errorf("invalid scenario_id: %w", err)
	}

	scenario, err := s.scenarioRepo.GetByID(ctx, scenarioID)
	if err != nil {
		return nil, fmt.Errorf("failed to get scenario: %w", err)
	}

	batchID := uuid.New().String()
	_ = s.batchService.CreateBatchWithJobs(ctx, batchID, req.ScenarioID, []string{learningItemsJobName})

	go s.processLearningItemsAsync(batchID, scenario)

	return &LearningItemsGenerateResponse{
		BatchID: batchID,
	}, nil
}

// processLearningItemsAsync calls GPT-5 Nano, parses the learning items array, saves to DB.
func (s *WorkoutService) processLearningItemsAsync(batchID string, scenario *repository.ConversationScenario) {
	ctx := context.Background()
	_ = s.batchService.UpdateJob(ctx, batchID, learningItemsJobName, "processing", "")

	// Extract level and tags from scenario metadata
	var meta struct {
		Level string   `json:"level"`
		Tags  []string `json:"tags"`
	}
	_ = json.Unmarshal(scenario.Metadata, &meta)

	level := meta.Level
	tags := strings.Join(meta.Tags, ", ")

	// Replace template vars in system prompt
	systemPrompt := strings.ReplaceAll(learningItemsSystemPrompt, "{{level}}", level)
	systemPrompt = strings.ReplaceAll(systemPrompt, "{{tags}}", tags)

	userMessage := fmt.Sprintf("Scenario: %s\nTarget Language: %s\n\nConversation Script:\n%s",
		scenario.Topic, scenario.TargetLang, scenario.Description)

	aiResp, err := s.chatClient.ChatCompletion(ctx, systemPrompt, userMessage)
	if err != nil {
		s.log.Error().Err(err).Str("batch_id", batchID).Msg("Learning items AI generation failed")
		_ = s.batchService.UpdateJob(ctx, batchID, learningItemsJobName, "failed", err.Error())
		return
	}

	// Clean response
	cleanResp := strings.TrimSpace(aiResp)
	cleanResp = strings.TrimPrefix(cleanResp, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```")
	cleanResp = strings.TrimSpace(cleanResp)

	// Parse as array of learning items
	var items []struct {
		Category string          `json:"category"`
		ItemID   string          `json:"item_id"`
		Data     json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(cleanResp), &items); err != nil {
		s.log.Error().Err(err).Str("raw", cleanResp).Msg("Failed to parse learning items AI response")
		_ = s.batchService.UpdateJob(ctx, batchID, learningItemsJobName, "failed", "failed to parse AI response: "+err.Error())
		return
	}

	// Save each learning item to DB
	var savedIDs []string
	for _, item := range items {
		metadata, _ := json.Marshal(map[string]interface{}{
			"batch_id":    batchID,
			"scenario_id": scenario.ID.String(),
			"category":    item.Category,
			"item_id":     item.ItemID,
		})

		detailsMap := map[string]interface{}{
			"meanings": item.Data, // Store full data as meanings/details
			"type":     item.Category,
			"media":    map[string]interface{}{},
		}
		detailsJSON, _ := json.Marshal(detailsMap)
		tagsJSON, _ := json.Marshal(meta.Tags)

		dbItem := &repository.LearningItem{
			Content:  fmt.Sprintf("[%s] %s", item.Category, item.ItemID),
			LangCode: scenario.TargetLang,
			Details:  detailsJSON,
			Tags:     tagsJSON,
			Metadata: metadata,
			IsActive: true,
		}

		if err := s.learningRepo.Create(ctx, dbItem); err != nil {
			s.log.Error().Err(err).Str("item_id", item.ItemID).Msg("Failed to save learning item")
			continue
		}
		savedIDs = append(savedIDs, dbItem.ID.String())
	}

	// Store result in batch
	resultData, _ := json.Marshal(map[string]interface{}{
		"scenario_id":    scenario.ID.String(),
		"total_items":    len(items),
		"saved_item_ids": savedIDs,
		"data":           json.RawMessage(cleanResp),
	})
	_ = s.batchService.SetBatchResult(ctx, batchID, resultData)

	_ = s.batchService.UpdateJob(ctx, batchID, learningItemsJobName, "completed", "")
	s.log.Info().Str("batch_id", batchID).Int("items", len(savedIDs)).Msg("Learning items generation completed")
}

// LearningItemsBatchResult is the DB-fallback response for expired learning item batches.
type LearningItemsBatchResult struct {
	BatchID string                     `json:"batch_id"`
	Status  string                     `json:"status"`
	Items   []*repository.LearningItem `json:"items"`
}

// GetLearningItemsByBatchID retrieves learning items from DB by batch_id.
func (s *WorkoutService) GetLearningItemsByBatchID(ctx context.Context, batchID string) (*LearningItemsBatchResult, error) {
	items, err := s.learningRepo.GetByBatchID(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}

	return &LearningItemsBatchResult{
		BatchID: batchID,
		Status:  "completed",
		Items:   items,
	}, nil
}

// WorkoutGenerateRequest is the POST body.
type WorkoutGenerateRequest struct {
	WorkoutTopic   string `json:"workout_topic"`
	PreBriefPrompt string `json:"pre_brief_prompt"`
	TargetLang     string `json:"target_lang"`
	IsComplete     bool   `json:"is_complete"`
}

// WorkoutItem references a generated item.
type WorkoutItem struct {
	Type     string `json:"type"`
	Category string `json:"category"` // "conversation" or "learning_item"
	ID       string `json:"id"`
}

// WorkoutGenerateResponse is the API response.
type WorkoutGenerateResponse struct {
	BatchID string        `json:"batch_id"`
	Items   []WorkoutItem `json:"items"`
}

// AI response structure for parsing.
type workoutAIResponse struct {
	Missing        workoutScenario     `json:"missing"`
	SparringMode   workoutScenario     `json:"sparring_mode"`
	StructureDrill workoutLearningItem `json:"structure_drill"`
	RhythmFlow     workoutLearningItem `json:"rhythm_flow"`
	VocabReps      workoutLearningItem `json:"vocab_reps"`
	PrecisionCheck workoutLearningItem `json:"precision_check"`
}

type workoutScenario struct {
	Topic           string          `json:"topic"`
	Description     string          `json:"description"`
	ImagePrompt     string          `json:"image_prompt"`
	DifficultyLevel int             `json:"difficulty_level"`
	Script          json.RawMessage `json:"script"`
}

type workoutLearningItem struct {
	Content     string          `json:"content"`
	ContextType string          `json:"context_type"`
	Meanings    json.RawMessage `json:"meanings"`
	Reading     json.RawMessage `json:"reading"`
	Tags        []string        `json:"tags"`
	Media       json.RawMessage `json:"media"`
	Metadata    json.RawMessage `json:"metadata"`
}

// GenerateWorkout generates a complete workout set.
func (s *WorkoutService) GenerateWorkout(ctx context.Context, req WorkoutGenerateRequest) (*WorkoutGenerateResponse, error) {
	// 1. Build prompt and call Gemini
	prompt := fmt.Sprintf(workoutPromptTemplate, req.WorkoutTopic, req.TargetLang, req.PreBriefPrompt, req.TargetLang)

	aiResp, err := s.aiService.Chat(ctx, prompt, "gemini")
	if err != nil {
		return nil, fmt.Errorf("failed to generate workout content: %w", err)
	}

	// 2. Clean and parse AI response
	cleanResp := strings.TrimSpace(aiResp)
	cleanResp = strings.TrimPrefix(cleanResp, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```")
	cleanResp = strings.TrimSpace(cleanResp)

	var workout workoutAIResponse
	if err := json.Unmarshal([]byte(cleanResp), &workout); err != nil {
		s.log.Error().Err(err).Str("raw", cleanResp).Msg("Failed to parse workout AI response")
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	// 3. Save items to DB
	items := make([]WorkoutItem, 0, 6)
	batchID := uuid.New().String()

	// Save conversation scenarios
	missingID, err := s.saveScenario(ctx, workout.Missing, "chat", req.TargetLang)
	if err != nil {
		s.log.Error().Err(err).Msg("Failed to save missing scenario")
	} else {
		items = append(items, WorkoutItem{Type: WorkoutMissing, Category: "conversation", ID: missingID})
	}

	sparringID, err := s.saveScenario(ctx, workout.SparringMode, "speech", req.TargetLang)
	if err != nil {
		s.log.Error().Err(err).Msg("Failed to save sparring scenario")
	} else {
		items = append(items, WorkoutItem{Type: WorkoutSparringMode, Category: "conversation", ID: sparringID})
	}

	// Save learning items
	drillID, err := s.saveLearningItem(ctx, workout.StructureDrill, req.TargetLang)
	if err != nil {
		s.log.Error().Err(err).Msg("Failed to save structure drill")
	} else {
		items = append(items, WorkoutItem{Type: WorkoutStructureDrill, Category: "learning_item", ID: drillID})
	}

	rhythmID, err := s.saveLearningItem(ctx, workout.RhythmFlow, req.TargetLang)
	if err != nil {
		s.log.Error().Err(err).Msg("Failed to save rhythm & flow")
	} else {
		items = append(items, WorkoutItem{Type: WorkoutRhythmFlow, Category: "learning_item", ID: rhythmID})
	}

	vocabID, err := s.saveLearningItem(ctx, workout.VocabReps, req.TargetLang)
	if err != nil {
		s.log.Error().Err(err).Msg("Failed to save vocab reps")
	} else {
		items = append(items, WorkoutItem{Type: WorkoutVocabReps, Category: "learning_item", ID: vocabID})
	}

	precisionID, err := s.saveLearningItem(ctx, workout.PrecisionCheck, req.TargetLang)
	if err != nil {
		s.log.Error().Err(err).Msg("Failed to save precision check")
	} else {
		items = append(items, WorkoutItem{Type: WorkoutPrecisionCheck, Category: "learning_item", ID: precisionID})
	}

	// 4. If is_complete, create batch and generate media async
	if req.IsComplete && len(items) > 0 {
		var mediaJobNames []string
		for _, item := range items {
			mediaJobNames = append(mediaJobNames, item.Type)
		}

		_ = s.batchService.CreateBatchWithJobs(ctx, batchID, req.WorkoutTopic, mediaJobNames)

		go s.generateMediaBatch(batchID, req.TargetLang, workout, items)
	}

	return &WorkoutGenerateResponse{
		BatchID: batchID,
		Items:   items,
	}, nil
}

func (s *WorkoutService) saveScenario(ctx context.Context, ws workoutScenario, interactionType, targetLang string) (string, error) {
	// Build metadata as the full scenario data
	metadata, _ := json.Marshal(map[string]interface{}{
		"image_prompt": ws.ImagePrompt,
		"script":       json.RawMessage(ws.Script),
	})

	difficulty := max(ws.DifficultyLevel, 1)

	scenario := &repository.ConversationScenario{
		Topic:           ws.Topic,
		Description:     ws.Description,
		InteractionType: interactionType,
		TargetLang:      targetLang,
		EstimatedTurns:  "6-10",
		DifficultyLevel: difficulty,
		Metadata:        metadata,
		IsActive:        true,
	}

	if err := s.scenarioRepo.Create(ctx, scenario); err != nil {
		return "", err
	}

	return scenario.ID.String(), nil
}

func (s *WorkoutService) saveLearningItem(ctx context.Context, wli workoutLearningItem, langCode string) (string, error) {
	mediaBytes := wli.Media
	if mediaBytes == nil {
		mediaBytes = json.RawMessage(`{}`)
	}
	metaBytes := wli.Metadata
	if metaBytes == nil {
		metaBytes = json.RawMessage(`{}`)
	}

	detailsMap := map[string]interface{}{
		"meanings": wli.Meanings,
		"reading":  wli.Reading,
		"type":     wli.ContextType,
		"media":    wli.Media,
	}
	detailsJSON, _ := json.Marshal(detailsMap)
	tagsJSON, _ := json.Marshal(wli.Tags)

	item := &repository.LearningItem{
		Content:   wli.Content,
		LangCode:  langCode,
		Details:   detailsJSON,
		Tags:      tagsJSON,
		Metadata:  metaBytes,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.learningRepo.Create(ctx, item); err != nil {
		return "", err
	}

	return item.ID.String(), nil
}

// generateMediaBatch runs media generation for each item in the background.
func (s *WorkoutService) generateMediaBatch(batchID, targetLang string, workout workoutAIResponse, items []WorkoutItem) {
	ctx := context.Background()
	var wg sync.WaitGroup

	for _, item := range items {
		wg.Add(1)
		go func(wi WorkoutItem) {
			defer wg.Done()

			_ = s.batchService.UpdateJob(ctx, batchID, wi.Type, "processing", "")

			var genErr error
			switch wi.Type {
			case WorkoutMissing:
				genErr = s.generateScenarioMedia(ctx, wi.ID, targetLang, workout.Missing)
			case WorkoutSparringMode:
				genErr = s.generateScenarioMedia(ctx, wi.ID, targetLang, workout.SparringMode)
			case WorkoutStructureDrill:
				genErr = s.generateLearningMedia(ctx, wi.ID, targetLang, workout.StructureDrill)
			case WorkoutRhythmFlow:
				genErr = s.generateLearningMedia(ctx, wi.ID, targetLang, workout.RhythmFlow)
			case WorkoutVocabReps:
				genErr = s.generateLearningMedia(ctx, wi.ID, targetLang, workout.VocabReps)
			case WorkoutPrecisionCheck:
				genErr = s.generateLearningMedia(ctx, wi.ID, targetLang, workout.PrecisionCheck)
			}

			if genErr != nil {
				s.log.Error().Err(genErr).Str("type", wi.Type).Str("id", wi.ID).Msg("Media generation failed")
				_ = s.batchService.UpdateJob(ctx, batchID, wi.Type, "failed", genErr.Error())
			} else {
				_ = s.batchService.UpdateJob(ctx, batchID, wi.Type, "completed", "")
			}
		}(item)
	}

	wg.Wait()
	s.log.Info().Str("batch_id", batchID).Msg("Workout media generation batch completed")
}

// generateScenarioMedia generates image + audio for a conversation scenario.
func (s *WorkoutService) generateScenarioMedia(ctx context.Context, id, targetLang string, ws workoutScenario) error {
	var mu sync.Mutex
	var wg sync.WaitGroup
	var metadataMap map[string]interface{}

	// Parse existing metadata
	scenario, err := s.scenarioRepo.GetByID(ctx, uuid.MustParse(id))
	if err != nil {
		return err
	}
	_ = json.Unmarshal(scenario.Metadata, &metadataMap)
	if metadataMap == nil {
		metadataMap = make(map[string]interface{})
	}

	updated := false

	// Image generation
	if ws.ImagePrompt != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if imgURL, err := s.aiService.GenerateAndUploadImage(ctx, id, ws.ImagePrompt); err == nil {
				mu.Lock()
				metadataMap["image_url"] = imgURL
				updated = true
				mu.Unlock()
			} else {
				s.log.Error().Err(err).Str("id", id).Msg("Scenario image generation failed")
			}
		}()
	}

	// Audio generation for AI script lines
	if script, ok := metadataMap["script"].([]interface{}); ok {
		for i, item := range script {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			speaker, _ := itemMap["speaker"].(string)
			text, _ := itemMap["text"].(string)
			if speaker == "ai" && text != "" {
				wg.Add(1)
				go func(idx int, im map[string]interface{}, txt string) {
					defer wg.Done()
					if audioURL, err := s.aiService.GenerateAndUploadAudio(ctx, id, idx, txt, targetLang); err == nil {
						mu.Lock()
						im["audio_url"] = audioURL
						updated = true
						mu.Unlock()
					} else {
						s.log.Error().Err(err).Str("id", id).Int("idx", idx).Msg("Scenario audio generation failed")
					}
				}(i, itemMap, text)
			}
		}
	}

	wg.Wait()

	if updated {
		updatedData, _ := json.Marshal(metadataMap)
		return s.scenarioRepo.UpdateMetadata(ctx, uuid.MustParse(id), updatedData)
	}
	return nil
}

// generateLearningMedia generates image + audio for a learning item.
func (s *WorkoutService) generateLearningMedia(ctx context.Context, id, targetLang string, wli workoutLearningItem) error {
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Parse existing media
	var mediaMap struct {
		ImagePrompt     string `json:"image_prompt"`
		ImageURL        string `json:"image_url,omitempty"`
		AudioURL        string `json:"audio_url,omitempty"`
		MeaningAudioURL string `json:"meaning_audio_url,omitempty"`
	}
	_ = json.Unmarshal(wli.Media, &mediaMap)

	// Image
	if mediaMap.ImagePrompt != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if imgURL, err := s.aiService.GenerateAndUploadImage(ctx, id, mediaMap.ImagePrompt); err == nil {
				mu.Lock()
				mediaMap.ImageURL = imgURL
				mu.Unlock()
			} else {
				s.log.Error().Err(err).Str("id", id).Msg("Learning item image generation failed")
			}
		}()
	}

	// Content audio (target lang)
	if wli.Content != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			voice := selectVoice(targetLang)
			if s.aiService.azureSpeechClient != nil {
				audioData, err := s.aiService.azureSpeechClient.Synthesize(ctx, wli.Content, voice)
				if err != nil {
					s.log.Error().Err(err).Str("id", id).Msg("Content audio gen failed")
					return
				}
				key := fmt.Sprintf("learning-items/%s-context.mp3", id)
				if url, err := s.aiService.cloudflareClient.UploadR2Object(ctx, key, audioData, "audio/mpeg"); err == nil {
					mu.Lock()
					mediaMap.AudioURL = url
					mu.Unlock()
				}
			}
		}()
	}

	// Meaning audio (native lang — Thai)
	var meaningsMap map[string]string
	_ = json.Unmarshal(wli.Meanings, &meaningsMap)
	meaningText := meaningsMap["th"]
	if meaningText != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.aiService.azureSpeechClient != nil {
				audioData, err := s.aiService.azureSpeechClient.Synthesize(ctx, meaningText, "th-TH-PremwadeeNeural")
				if err != nil {
					s.log.Error().Err(err).Str("id", id).Msg("Meaning audio gen failed")
					return
				}
				key := fmt.Sprintf("learning-items/%s-meaning.mp3", id)
				if url, err := s.aiService.cloudflareClient.UploadR2Object(ctx, key, audioData, "audio/mpeg"); err == nil {
					mu.Lock()
					mediaMap.MeaningAudioURL = url
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	// Update media in DB
	// Update media in DB via Details column
	item, err := s.learningRepo.GetByID(ctx, uuid.MustParse(id))
	if err != nil {
		return err
	}

	var detailsMap map[string]interface{}
	if len(item.Details) > 0 {
		_ = json.Unmarshal(item.Details, &detailsMap)
	} else {
		detailsMap = make(map[string]interface{})
	}

	// Update media field in details
	detailsMap["media"] = mediaMap
	newDetails, _ := json.Marshal(detailsMap)
	item.Details = newDetails

	return s.learningRepo.Update(ctx, item)
}

func selectVoice(langCode string) string {
	switch langCode {
	case "zh-CN":
		return "zh-CN-XiaoxiaoNeural"
	case "en-US":
		return "en-US-AvaMultilingualNeural"
	default:
		return "en-US-AvaMultilingualNeural"
	}
}
