package http

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/errors"
	"github.com/windfall/uwu_service/internal/service"
	"github.com/windfall/uwu_service/pkg/response"
)

// APIHandler handles REST API endpoints.
type APIHandler struct {
	log           zerolog.Logger
	aiService     *service.AIService
	speechService *service.SpeechService
}

// NewAPIHandler creates a new API handler.
func NewAPIHandler(
	log zerolog.Logger,
	aiService *service.AIService,
	speechService *service.SpeechService,
) *APIHandler {
	return &APIHandler{
		log:           log,
		aiService:     aiService,
		speechService: speechService,
	}
}

// ChatRequest represents the request body for AI chat.
type ChatRequest struct {
	Message  string `json:"message"`
	Provider string `json:"provider"` // "openai" or "gemini"
}

// Chat handles POST /api/v1/ai/chat
func (h *APIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(w, errors.Validation("invalid request body"))
		return
	}

	if req.Message == "" {
		h.handleError(w, errors.Validation("message is required"))
		return
	}

	result, err := h.aiService.Chat(ctx, req.Message, req.Provider)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"response": result,
		"provider": req.Provider,
	})
}

// CompleteRequest represents the request body for AI completion.
type CompleteRequest struct {
	Prompt   string `json:"prompt"`
	Provider string `json:"provider"` // "openai" or "gemini"
}

// Complete handles POST /api/v1/ai/complete
func (h *APIHandler) Complete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(w, errors.Validation("invalid request body"))
		return
	}

	if req.Prompt == "" {
		h.handleError(w, errors.Validation("prompt is required"))
		return
	}

	result, err := h.aiService.Complete(ctx, req.Prompt, req.Provider)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"completion": result,
		"provider":   req.Provider,
	})
}

// AnalyzeVocab handles POST /api/v1/speech/analyze/vocab
func (h *APIHandler) AnalyzeVocab(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse multipart form (10 MB max)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		h.handleError(w, errors.Validation("failed to parse multipart form"))
		return
	}

	// Get file
	file, _, err := r.FormFile("audio")
	if err != nil {
		h.handleError(w, errors.Validation("audio file is required"))
		return
	}
	defer file.Close()

	// Get reference text
	referenceText := r.FormValue("reference_text")
	// Read file content
	// In production, might want to check file type/magic bytes here
	audioData := make([]byte, 0)
	buf := make([]byte, 1024)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			audioData = append(audioData, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	result, err := h.speechService.AnalyzeVocabAudio(ctx, audioData, referenceText)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, result)
}

// AnalyzeShadowing handles POST /api/v1/speech/analyze/shadowing
func (h *APIHandler) AnalyzeShadowing(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse multipart form (10 MB max)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		h.handleError(w, errors.Validation("failed to parse multipart form"))
		return
	}

	// Get file
	file, _, err := r.FormFile("audio")
	if err != nil {
		h.handleError(w, errors.Validation("audio file is required"))
		return
	}
	defer file.Close()

	// Get reference text and language
	referenceText := r.FormValue("reference_text")
	language := r.FormValue("language") // Optional, defaults to en-US

	// Read file content
	audioData := make([]byte, 0)
	buf := make([]byte, 1024)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			audioData = append(audioData, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	result, err := h.speechService.AnalyzeShadowingAudio(ctx, audioData, referenceText, language)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, result)
}

// GetMockVocab handles GET /api/v1/vocab/mock
func (h *APIHandler) GetMockVocab(w http.ResponseWriter, r *http.Request) {
	vocab := map[string]interface{}{
		"id":   "vocab_101",
		"word": "Apple",
		"pronunciation": map[string]interface{}{
			"ipa":            "/ˈæp.l/",
			"simple_reading": "AP-pul",
			"syllables":      []string{"ap", "ple"},
			"stress_index":   0,
		},
		"part_of_speech": "noun",
		"meanings": map[string]interface{}{
			"target_lang": "A round fruit with red or green skin.",
			"native_lang": "ผลไม้ชนิดหนึ่ง มีรสหวาน เปลือกสีแดงหรือเขียว",
		},
		"media": map[string]interface{}{
			"image_url":         "https://pub-d85099e9916143fcb172f661babc3497.r2.dev/image/apple.png",
			"word_audio_url":    "https://pub-d85099e9916143fcb172f661babc3497.r2.dev/audio/apple-en.mp3",
			"meaning_audio_url": "https://pub-d85099e9916143fcb172f661babc3497.r2.dev/audio/apple-meaning-th.mp3",
		},
		"tags":             []string{"food", "fruit", "beginner", "A1"},
		"difficulty_level": 1,
	}

	response.JSON(w, http.StatusOK, vocab)
}

// GetMockShadowing handles GET /api/v1/shadowing/mock
func (h *APIHandler) GetMockShadowing(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"id":               "shadow_205",
		"difficulty_level": 1,
		"tags":             []string{"daily-life", "food", "question"},
		"content": map[string]interface{}{
			"target_lang": "今天吃什么？",
			"native_lang": "วันนี้กินอะไรดี?",
			"phonetic": map[string]string{
				"readable": "Jīntiān chī shénme?",
				"ipa":      "/tɕin⁵⁵ tʰjɛn⁵⁵ tʂʰz̩⁵⁵ ʂən³⁵ mə/",
			},
		},
		"media": map[string]string{
			"image_url":          "https://pub-d85099e9916143fcb172f661babc3497.r2.dev/image/shadowing-00000.png",
			"meaning_audio_url":  "https://pub-d85099e9916143fcb172f661babc3497.r2.dev/audio/shadowing-th00000.mp3",
			"sentence_audio_url": "https://pub-d85099e9916143fcb172f661babc3497.r2.dev/audio/shadowing-zh00000.mp3",
		},
		"grammar": map[string]interface{}{
			"context": []string{"Casual", "Friendly"},
			"breakdown": []map[string]string{
				{"segment": "今天", "meaning": "วันนี้", "role": "เวลา"},
				{"segment": "吃", "meaning": "กิน", "role": "กริยา"},
				{"segment": "什么？", "meaning": "อะไร", "role": "กรรม/คำถาม"},
			},
		},
	}

	response.JSON(w, http.StatusOK, data)
}

func (h *APIHandler) handleError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*errors.AppError); ok {
		response.Error(w, appErr.HTTPStatus(), appErr)
		return
	}
	h.log.Error().Err(err).Msg("Internal server error")
	response.Error(w, http.StatusInternalServerError, errors.Internal("internal server error"))
}

// GenerateScenario handles POST /api/v1/scenario/generate
func (h *APIHandler) GenerateScenario(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req service.GenerateScenarioReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(w, errors.Validation("invalid request body"))
		return
	}

	if req.Topic == "" || req.Difficulty == "" {
		h.handleError(w, errors.Validation("topic and difficulty are required"))
		return
	}

	result, err := h.aiService.GenerateScenario(ctx, req)
	if err != nil {
		h.handleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, result)
}
