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
