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

You are a backend API that processes language learning data. Your task is to generate a strictly formatted JSON object containing content for **Speech Practice** (Roleplay) and **Chat Mission** (Text-based).

# Input Parameters
* **Topic:** {{TOPIC}}
* **Description:** {{DESCRIPTION}}
* **Description Type:** {{DESCRIPTION_TYPE}}
  * *Values: "explanation" (Summary of context) OR "transcription" (Actual dialogue text)*

# Processing Rules

## 1. Language & Level Analysis
* **Target Language:** Detect the language from the input. Return the IETF BCP 47 code (e.g., ` + "`en-US`" + `, ` + "`zh-CN`" + `, ` + "`th-TH`" + `).
* **Level:** Evaluate the complexity of the input text/topic and assign a standard proficiency level code:
    * European languages: **CEFR** (A1, A2, B1, B2, C1, C2)
    * Chinese: **HSK** (HSK1 - HSK6/9)
    * Japanese: **JLPT** (N5 - N1)
* **Tags:** Generate 3-5 relevant keywords describing the topic (e.g., "business", "travel", "slang").

## 2. Content Generation Logic
* **Image Prompt:** Create a prompt for a text-to-image model.
    * *Style:* **Photorealistic, Cinematic lighting, 4k resolution, Highly detailed.**
    * *Content:* Strictly depict the setting and atmosphere described.
* **Speech Mode (Script) - OPTIMIZED FOR LEARNING:**
    * **Length Constraint:** Generate **ONLY 6-10 turns for Beginner level, 10-16 turns for Intermediate level, and 16-24 turns for Advanced level**. Keep it concise.
    * **Cognitive Load Control:** Ensure each "user" turn is **1-3 sentences max**. Avoid long monologues (too hard to memorize) and avoid single words (too easy).
    * **If Type is "explanation":** Create a realistic dialogue where the User has a clear goal. The AI should guide the conversation naturally.
    * **If Type is "transcription":**
        * **SEMANTIC GROUPING:** Do not split every sentence. Group the source text into logical "Thought Units."
        * **Example:** Combine [Observation + Feeling + Action] into one turn.
        * **Adaptation:** You may slightly condense the source text to fit the "setting turns" limit while keeping the key vocabulary and phrases.
        * **Role Play:** User speaks the core content. AI acts as an **Active Listener** (asking short follow-up questions or giving brief reactions) to bridge the User's turns naturally.
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
  "meta": {
    "target_lang": "string",
    "level": "string", // e.g. "HSK3" or "B1"
    "tags": ["string"] // e.g. ["shopping", "bargaining"]
  },
  "image_prompt": "string", // English, Photorealistic style
  "speech_mode": {
    "script": [
      {
        "speaker": "string", // "User" or "AI" (Try to avoid turn-by-turn dialogues)
        "text": "string" // Actual dialogue text
      }
	  // ... Generate enough turns to cover the content ...
    ]
  },
  "chat_mode": {
    "situation": "string", // Brief context setup
    "objectives": {
      "requirements": ["string"], // 3-5 Actionable tasks suited to the level
      "persuasion": ["string"], // 1-2 Goals to achieve in the conversation
      "constraints": ["string"] // 1-3 Behavioral/Tonal constraints
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

	item := &repository.LearningItem{
		Content:   wli.Content,
		LangCode:  langCode,
		Meanings:  wli.Meanings,
		Reading:   wli.Reading,
		Type:      wli.ContextType,
		Tags:      wli.Tags,
		Media:     mediaBytes,
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
	finalMedia, _ := json.Marshal(mediaMap)
	return s.learningRepo.UpdateMedia(ctx, uuid.MustParse(id), finalMedia)
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
