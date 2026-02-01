package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/repository"
)

type LearningService struct {
	ai   *AIService
	repo repository.LearningItemRepository
}

func NewLearningService(ai *AIService, repo repository.LearningItemRepository) *LearningService {
	return &LearningService{
		ai:   ai,
		repo: repo,
	}
}

type CreateLearningItemReq struct {
	Context    string `json:"context"`
	LangCode   string `json:"lang_code"`
	NativeLang string `json:"native_lang"`
	IsActive   bool   `json:"is_active"`
}

func (s *LearningService) CreateLearningItem(ctx context.Context, req CreateLearningItemReq) (*repository.LearningItem, error) {
	// 1. Generate Content via AI
	aiResp, err := s.ai.GenerateLearningItem(ctx, GenerateLearningItemReq{
		Context:    req.Context,
		LangCode:   req.LangCode,
		NativeLang: req.NativeLang,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate AI content: %w", err)
	}

	// 2. Parse AI Response
	// Expecting JSON from AI. Clean it up first.
	cleanResp := strings.TrimSpace(aiResp)
	cleanResp = strings.TrimPrefix(cleanResp, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```")

	var itemData struct {
		ContextType string          `json:"context_type"`
		Meanings    json.RawMessage `json:"meanings"`
		Reading     json.RawMessage `json:"reading"`
		Tags        []string        `json:"tags"`
		Media       struct {
			ImagePrompt     string `json:"image_prompt"`
			ImageURL        string `json:"image_url,omitempty"`
			AudioURL        string `json:"audio_url,omitempty"`         // Content Audio
			MeaningAudioURL string `json:"meaning_audio_url,omitempty"` // Meaning Audio
		} `json:"media"`
		Metadata json.RawMessage `json:"metadata"`
	}

	if err := json.Unmarshal([]byte(cleanResp), &itemData); err != nil {
		return nil, fmt.Errorf("failed to parse AI JSON: %w", err)
	}

	// 3. Prepare DB Item
	mediaBytes, _ := json.Marshal(itemData.Media)
	newItem := &repository.LearningItem{
		Content:   req.Context,
		LangCode:  req.LangCode,
		Meanings:  itemData.Meanings,
		Reading:   itemData.Reading,
		Type:      itemData.ContextType, // Use AI-inferred context_type
		Tags:      itemData.Tags,
		Media:     mediaBytes,
		Metadata:  itemData.Metadata,
		IsActive:  req.IsActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 4. Save to DB
	if err := s.repo.Create(ctx, newItem); err != nil {
		return nil, fmt.Errorf("failed to save learning item: %w", err)
	}

	// 5. Async Media Generation (if active)
	if req.IsActive {
		go s.generateMediaAsync(newItem.ID, itemData.Media.ImagePrompt, req.Context, req.LangCode, itemData.Meanings, req.NativeLang, itemData.Media)
	}

	return newItem, nil
}

func (s *LearningService) ListLearningItems(ctx context.Context, page, limit int) ([]*repository.LearningItem, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit
	return s.repo.List(ctx, limit, offset)
}

type UpdateLearningItemReq struct {
	Content  string          `json:"content"`
	LangCode string          `json:"lang_code"`
	Meanings json.RawMessage `json:"meanings"`
	Reading  json.RawMessage `json:"reading"`
	Type     string          `json:"type"`
	Tags     []string        `json:"tags"`
	Media    json.RawMessage `json:"media"`
	Metadata json.RawMessage `json:"metadata"`
	IsActive bool            `json:"is_active"`
}

func (s *LearningService) UpdateLearningItem(ctx context.Context, id uuid.UUID, req UpdateLearningItemReq) (*repository.LearningItem, error) {
	item := &repository.LearningItem{
		ID:       id,
		Content:  req.Content,
		LangCode: req.LangCode,
		Meanings: req.Meanings,
		Reading:  req.Reading,
		Type:     req.Type,
		Tags:     req.Tags,
		Media:    req.Media,
		Metadata: req.Metadata,
		IsActive: req.IsActive,
	}

	if err := s.repo.Update(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *LearningService) DeleteLearningItem(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *LearningService) GetLearningItem(ctx context.Context, id uuid.UUID) (*repository.LearningItem, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *LearningService) generateMediaAsync(
	id uuid.UUID,
	imagePrompt, content, langCode string,
	meaningsRaw json.RawMessage,
	nativeLang string,
	currentMedia struct {
		ImagePrompt     string `json:"image_prompt"`
		ImageURL        string `json:"image_url,omitempty"`
		AudioURL        string `json:"audio_url,omitempty"`
		MeaningAudioURL string `json:"meaning_audio_url,omitempty"`
	},
) {
	ctx := context.Background()
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Update currentMedia object safely
	updateMedia := func(infoType, url string) {
		mu.Lock()
		defer mu.Unlock()
		switch infoType {
		case "image":
			currentMedia.ImageURL = url
		case "audio":
			currentMedia.AudioURL = url
		case "meaning_audio":
			currentMedia.MeaningAudioURL = url
		}
	}

	// 1. Image Generation
	if imagePrompt != "" && s.ai.geminiClient != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Generate Image
			imgData, err := s.ai.geminiClient.GenerateImage(ctx, imagePrompt)
			if err != nil {
				fmt.Printf("Async Image Gen Error: %v\n", err)
				return
			}
			// Upload
			if s.ai.cloudflareClient != nil {
				key := fmt.Sprintf("learning-items/%s-image.webp", id)
				url, err := s.ai.cloudflareClient.UploadR2Object(ctx, key, imgData, "image/webp")
				if err != nil {
					fmt.Printf("Async Image Upload Error: %v\n", err)
					return
				}
				updateMedia("image", url)
			}
		}()
	}

	// 2. Audio Generation (Content - Target Lang)
	if content != "" && s.ai.azureSpeechClient != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Select voice based on langCode
			voice := "en-US-AvaMultilingualNeural" // Default
			switch langCode {
			case "zh-CN":
				voice = "zh-CN-XiaoxiaoNeural"
			case "en-US":
				voice = "en-US-AvaMultilingualNeural"
			}
			audioData, err := s.ai.azureSpeechClient.Synthesize(ctx, content, voice)
			if err != nil {
				fmt.Printf("Async Content Audio error: %v\n", err)
				return
			}
			// Upload
			if s.ai.cloudflareClient != nil {
				key := fmt.Sprintf("learning-items/%s-context.mp3", id)
				url, err := s.ai.cloudflareClient.UploadR2Object(ctx, key, audioData, "audio/mpeg")
				if err != nil {
					fmt.Printf("Async Content Audio Upload Error: %v\n", err)
					return
				}
				updateMedia("audio", url)
			}
		}()
	}

	// 3. Audio Generation (Meaning - Native Lang)
	// Extract meaning string from JSON
	var meaningsMap map[string]string
	_ = json.Unmarshal(meaningsRaw, &meaningsMap)
	meaningText := meaningsMap[nativeLang]

	if meaningText != "" && s.ai.azureSpeechClient != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Select voice based on nativeLang
			voice := "en-US-AvaMultilingualNeural" // Default
			switch nativeLang {
			case "th":
				voice = "th-TH-PremwadeeNeural"
			}
			audioData, err := s.ai.azureSpeechClient.Synthesize(ctx, meaningText, voice)
			if err != nil {
				fmt.Printf("Async Meaning Audio error: %v\n", err)
				return
			}
			// Upload
			if s.ai.cloudflareClient != nil {
				key := fmt.Sprintf("learning-items/%s-meaning.mp3", id)
				url, err := s.ai.cloudflareClient.UploadR2Object(ctx, key, audioData, "audio/mpeg")
				if err != nil {
					fmt.Printf("Async Meaning Audio Upload Error: %v\n", err)
					return
				}
				updateMedia("meaning_audio", url)
			}
		}()
	}

	wg.Wait()

	// Update DB with collected URLs
	finalMediaBytes, _ := json.Marshal(currentMedia)
	if err := s.repo.UpdateMedia(ctx, id, finalMediaBytes); err != nil {
		fmt.Printf("Failed to update media for learning item %s: %v\n", id, err)
	}
}
