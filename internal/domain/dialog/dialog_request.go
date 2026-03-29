package dialog

import (
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/infra/middleware"
	"github.com/windfall/uwu_service/pkg/errors"
)

// -------------------------------------------------------------------------
// Generate Dialog Request
// -------------------------------------------------------------------------

// GenerateDialogRequest is the HTTP request struct for generating a dialog
type GenerateDialogRequest struct {
	UserID      string   `json:"user_id"`
	Topic       string   `json:"topic"`
	Description string   `json:"description"`
	Language    string   `json:"language"`
	Level       string   `json:"level"`
	Tags        []string `json:"tags"`
}

// GenerateDialogPayload is the payload struct for service
type GenerateDialogPayload struct {
	DialogID    string
	UserID      string
	Topic       string
	Description string
	Language    string
	Level       string
	Tags        []string
}

// AllowedLanguages
var AllowedLanguages = map[string]bool{
	"english":    true,
	"chinese":    true,
	"japanese":   true,
	"french":     true,
	"spanish":    true,
	"portuguese": true,
	"arabic":     true,
	"russian":    true,
}

// ParseAndValidate แกะกล่อง JSON และตรวจสอบความถูกต้องของข้อมูล
func (req *GenerateDialogRequest) ParseAndValidate(r *http.Request) error {
	// 1. Get user ID from auth context
	req.UserID = middleware.GetUserID(r.Context())
	if req.UserID == "" {
		return errors.Unauthorized("user not authenticated")
	}

	// 2. parse request body
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		return errors.Validation("invalid request body")
	}

	// 3. เช็ก topic
	if req.Topic == "" {
		return errors.Validation("topic is required")
	}

	// 4. เช็กภาษา
	req.Language = strings.ToLower(req.Language)
	if !AllowedLanguages[req.Language] {
		return errors.Validation("unsupported language")
	}

	// 5. เช็ก level
	if req.Level == "" {
		return errors.Validation("level is required")
	}

	return nil
}

// ToPayload convert GenerateDialogRequest to GenerateDialogPayload
func (req *GenerateDialogRequest) ToPayload() GenerateDialogPayload {
	dialogID := uuid.New().String()

	return GenerateDialogPayload{
		DialogID:    dialogID,
		UserID:      req.UserID,
		Topic:       req.Topic,
		Description: req.Description,
		Language:    req.Language,
		Level:       req.Level,
		Tags:        req.Tags,
	}
}

// -------------------------------------------------------------------------
// List Dialog Contents Request
// -------------------------------------------------------------------------

// ListDialogContentsRequest is the HTTP request struct for listing dialog contents
type ListDialogContentsRequest struct {
	Page     int
	PageSize int
}

// ListDialogContentsInput is the input struct for service
type ListDialogContentsInput struct {
	Page     int
	PageSize int
	Limit    int
	Offset   int
}

// Parse parse pagination params
func (req *ListDialogContentsRequest) Parse(r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	pageSizeStr := r.URL.Query().Get("page_size")

	page, _ := strconv.Atoi(pageStr)
	if page <= 0 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(pageSizeStr)
	if pageSize <= 0 {
		pageSize = 10
	}

	req.Page = page
	req.PageSize = pageSize
}

// ToInput convert ListDialogContentsRequest to ListDialogContentsInput
func (req *ListDialogContentsRequest) ToInput() ListDialogContentsInput {
	limit := req.PageSize
	offset := (req.Page - 1) * req.PageSize

	return ListDialogContentsInput{
		Page:     req.Page,
		PageSize: req.PageSize,
		Limit:    limit,
		Offset:   offset,
	}
}

// -------------------------------------------------------------------------
// Submit Speech Request
// -------------------------------------------------------------------------

// SubmitSpeechRequest is the HTTP request struct for submitting speech audio
type SubmitSpeechRequest struct {
	UserID           string
	DialogID         string
	AudioFile        multipart.File
	AudioContentType string
	ReferenceText    string
	ScriptIndex      int
	Language         string
}

// SubmitSpeechInput is the input struct for service
type SubmitSpeechInput struct {
	UserID           string
	DialogID         string
	AudioID          string
	AudioFile        multipart.File
	AudioWavPath     string
	AudioContentType string
	ReferenceText    string
	ScriptIndex      int
	Language         string
}

func (req *SubmitSpeechRequest) ParseAndValidate(r *http.Request) error {
	// 1. Get user ID
	req.UserID = middleware.GetUserID(r.Context())
	if req.UserID == "" {
		return errors.Unauthorized("user not authenticated")
	}

	// 2. Parse URL Params
	req.DialogID = chi.URLParam(r, "dialogID")
	if req.DialogID == "" {
		return errors.Validation("Dialog ID is required")
	}

	// 3. Parse Multipart Form (10MB limit is enough for audio)
	const maxUploadSize = 10 << 20
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		return errors.Validation("file too large or invalid multipart data")
	}

	// 3. Extract Form Fields
	req.ReferenceText = r.FormValue("reference_text")
	if req.ReferenceText == "" {
		return errors.Validation("reference_text is required")
	}

	scriptIdxStr := r.FormValue("script_index")
	if idx, err := strconv.Atoi(scriptIdxStr); err == nil {
		req.ScriptIndex = idx
	} else {
		return errors.Validation("invalid or missing script_index")
	}

	req.Language = strings.ToLower(r.Header.Get("Language"))
	if !AllowedLanguages[req.Language] {
		return errors.Validation("unsupported language")
	}

	// 4. Extract Audio File
	aFile, aHeader, err := r.FormFile("audio")
	if err != nil {
		return errors.Validation("audio file is required (form field: 'audio')")
	}
	defer aFile.Close()
	req.AudioFile = aFile

	req.AudioContentType = aHeader.Header.Get("Content-Type")
	if req.AudioContentType == "" {
		req.AudioContentType = "audio/wav"
	}

	return nil
}

// ToInput convert SubmitSpeechRequest to SubmitSpeechInput
func (req *SubmitSpeechRequest) ToInput() SubmitSpeechInput {
	audioID := uuid.New().String()
	audioWavPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s.wav", audioID))

	return SubmitSpeechInput{
		UserID:           req.UserID,
		DialogID:         req.DialogID,
		AudioID:          audioID,
		AudioFile:        req.AudioFile,
		AudioWavPath:     audioWavPath,
		AudioContentType: req.AudioContentType,
		ReferenceText:    req.ReferenceText,
		ScriptIndex:      req.ScriptIndex,
		Language:         req.Language,
	}
}

// -------------------------------------------------------------------------
// Submit Chat Request
// -------------------------------------------------------------------------

// SubmitChatRequest is the HTTP request struct for submitting a chat message
type SubmitChatRequest struct {
	UserID   string `json:"-"`
	DialogID string `json:"-"`
	Message  string `json:"message"`
}

// ReplyChatMessagePayload is the payload struct for the reply chat message worker
type ReplyChatMessagePayload struct {
	UserID   string
	DialogID string
	Message  string
}

// SubmitChatInput is the input struct for service
type SubmitChatInput struct {
	UserID   string
	DialogID string
	Message  string
}

func (req *SubmitChatRequest) ParseAndValidate(r *http.Request) error {
	// 1. Get user ID
	req.UserID = middleware.GetUserID(r.Context())
	if req.UserID == "" {
		return errors.Unauthorized("user not authenticated")
	}

	// 2. Parse URL Params
	req.DialogID = chi.URLParam(r, "dialogID")
	if req.DialogID == "" {
		return errors.Validation("Dialog ID is required")
	}

	// 3. Parse JSON Body
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		return errors.Validation("invalid request body")
	}

	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		return errors.Validation("message is required")
	}

	return nil
}

// ToInput convert SubmitChatRequest to SubmitChatInput
func (req *SubmitChatRequest) ToInput() SubmitChatInput {
	return SubmitChatInput{
		UserID:   req.UserID,
		DialogID: req.DialogID,
		Message:  req.Message,
	}
}

// ToPayload convert SubmitChatRequest to ReplyChatMessagePayload
func (req *SubmitChatRequest) ToPayload() ReplyChatMessagePayload {
	return ReplyChatMessagePayload{
		UserID:   req.UserID,
		DialogID: req.DialogID,
		Message:  req.Message,
	}
}
