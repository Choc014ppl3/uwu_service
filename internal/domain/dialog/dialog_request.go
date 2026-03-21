package dialog

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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
// Generate Image Request
// -------------------------------------------------------------------------

// GenerateImageRequest is the HTTP request struct for generating an image
type GenerateImageRequest struct {
	Prompt string `json:"prompt"`
}

// ParseAndValidate parses and validates the generate image request.
func (req *GenerateImageRequest) ParseAndValidate(r *http.Request) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		return errors.Validation("invalid request body")
	}

	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		return errors.Validation("prompt is required")
	}

	return nil
}
