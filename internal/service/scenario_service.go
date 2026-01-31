package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/repository"
)

type ScenarioService struct {
	aiService    *AIService
	scenarioRepo repository.ConversationScenarioRepository
}

func NewScenarioService(aiService *AIService, repo repository.ConversationScenarioRepository) *ScenarioService {
	return &ScenarioService{
		aiService:    aiService,
		scenarioRepo: repo,
	}
}

type CreateScenarioReq struct {
	Topic           string `json:"topic"`
	Description     string `json:"description"`
	InteractionType string `json:"interaction_type"` // "chat" or "speech"
	EstimatedTurns  string `json:"estimate_turns"`
	TargetLang      string `json:"target_lang"`
	IsActive        bool   `json:"is_active"`
}

func (s *ScenarioService) CreateScenario(ctx context.Context, req CreateScenarioReq) (*repository.ConversationScenario, error) {
	// 1. Generate Metadata via AI
	aiReq := GenerateScenarioContentReq{
		Topic:           req.Topic,
		Description:     req.Description,
		InteractionType: req.InteractionType,
		EstimatedTurns:  req.EstimatedTurns,
		TargetLang:      req.TargetLang,
	}

	aiResp, err := s.aiService.GenerateScenarioContent(ctx, aiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to generate AI content: %w", err)
	}

	// 2. Parse AI Response
	cleanResp := strings.TrimSpace(aiResp)
	cleanResp = strings.TrimPrefix(cleanResp, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```")

	// We validate it's JSON and extract difficulty_level
	var tempMetadata struct {
		DifficultyLevel int `json:"difficulty_level"`
		// Capture other fields if necessary, or just unmarshal again to RawMessage
	}
	if err := json.Unmarshal([]byte(cleanResp), &tempMetadata); err != nil {
		fmt.Printf("Warning: failed to parse difficulty_level: %v\n", err)
	}

	var metadata json.RawMessage
	if err := json.Unmarshal([]byte(cleanResp), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse AI JSON: %w", err)
	}

	// Use DifficultyLevel from AI, default to 1 if missing/invalid
	difficulty := max(tempMetadata.DifficultyLevel, 1)

	// 3. Create DB Entry
	scenario := &repository.ConversationScenario{
		Topic:           req.Topic,
		Description:     req.Description,
		InteractionType: req.InteractionType,
		TargetLang:      req.TargetLang,
		EstimatedTurns:  req.EstimatedTurns,
		DifficultyLevel: difficulty,
		Metadata:        metadata,
		IsActive:        req.IsActive,
	}

	if err := s.scenarioRepo.Create(ctx, scenario); err != nil {
		return nil, fmt.Errorf("failed to create scenario in DB: %w", err)
	}

	// 4. Async Tasks (Image & Audio)
	if req.IsActive {
		go s.processAsyncScenarioTasks(scenario.ID.String(), req.Topic, req.TargetLang, cleanResp)
	}

	return scenario, nil
}

func (s *ScenarioService) processAsyncScenarioTasks(id, topic, targetLang, rawMetadata string) {
	ctx := context.Background()

	var metadataMap map[string]interface{}
	if err := json.Unmarshal([]byte(rawMetadata), &metadataMap); err != nil {
		fmt.Printf("Async Error: Failed to parse metadata for ID %s: %v\n", id, err)
		return
	}

	updated := false

	// A. Generate Image
	imagePrompt := topic // fallback
	if prompt, ok := metadataMap["image_prompt"].(string); ok && prompt != "" {
		imagePrompt = prompt
	}

	if imgURL, err := s.aiService.GenerateAndUploadImage(ctx, id, imagePrompt); err == nil {
		metadataMap["image_url"] = imgURL
		updated = true
	} else {
		fmt.Printf("Async Error: Image generation failed for ID %s: %v\n", id, err)
	}

	// B. Generate Audio (only for 'speech' type with script)
	if script, ok := metadataMap["script"].([]interface{}); ok {
		var newScript []interface{}
		for i, item := range script {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				newScript = append(newScript, item)
				continue
			}

			speaker, _ := itemMap["speaker"].(string)
			text, _ := itemMap["text"].(string)

			if speaker == "ai" && text != "" {
				if audioURL, err := s.aiService.GenerateAndUploadAudio(ctx, id, i, text, targetLang); err == nil {
					itemMap["audio_url"] = audioURL
					updated = true
				} else {
					fmt.Printf("Async Error: Audio generation failed for ID %s index %d: %v\n", id, i, err)
				}
			}
			newScript = append(newScript, itemMap)
		}
		metadataMap["script"] = newScript
	}

	// C. Update DB
	updatedData, _ := json.Marshal(metadataMap)
	if updated {
		if err := s.scenarioRepo.UpdateMetadata(ctx, uuid.MustParse(id), updatedData); err != nil {
			fmt.Printf("Async Error: Failed to update metadata in DB for ID %s: %v\n", id, err)
		} else {
			fmt.Printf("Async Success: Metadata updated for ID %s\n", id)
		}
	}
}

func (s *ScenarioService) GetScenario(ctx context.Context, id uuid.UUID) (*repository.ConversationScenario, error) {
	return s.scenarioRepo.GetByID(ctx, id)
}
