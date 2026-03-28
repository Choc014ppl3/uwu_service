package video

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
// Upload Video Request
// -------------------------------------------------------------------------

// UploadVideoRequest is the HTTP request struct for uploading a video
type UploadVideoRequest struct {
	UserID               string
	Language             string
	VideoFile            multipart.File
	VideoContentType     string
	ThumbnailFile        multipart.File
	ThumbnailContentType string
}

// UploadVideoPayload is the payload struct for queue
type UploadVideoPayload struct {
	UserID               string
	VideoID              string
	Language             string
	VideoExt             string
	VideoPath            string
	VideoFile            multipart.File
	VideoContentType     string
	VideoR2Path          string
	ThumbnailExt         string
	ThumbnailPath        string
	ThumbnailFile        multipart.File
	ThumbnailContentType string
	ThumbnailR2Path      string
	AudioPath            string
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

var allowedVideoMIME = map[string]bool{
	"video/mp4":       true,
	"video/quicktime": true,
	"video/x-msvideo": true,
	"video/webm":      true,
}

var allowedImageMIME = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

var mimeToExt = map[string]string{
	"video/mp4":       ".mp4",
	"video/quicktime": ".mov",
	"video/x-msvideo": ".avi",
	"video/webm":      ".webm",
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
}

// Close สำคัญมาก! ใช้เพื่อให้ Handler สั่งปิดไฟล์ตอนทำงานเสร็จ
func (req *UploadVideoRequest) Close() {
	if req.VideoFile != nil {
		req.VideoFile.Close()
	}
	if req.ThumbnailFile != nil {
		req.ThumbnailFile.Close()
	}
}

func (req *UploadVideoRequest) ParseAndValidate(r *http.Request) error {
	// 1. Get user ID from auth context
	req.UserID = middleware.GetUserID(r.Context())
	if req.UserID == "" {
		return errors.Unauthorized("user not authenticated")
	}

	// 2. Parse Multipart Form (30MB limit)
	const maxUploadSize = 30 << 20
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		return errors.Validation("file too large or invalid multipart data")
	}

	// 3. Extract Language Header & Validate
	req.Language = strings.ToLower(r.Header.Get("Language"))
	if !AllowedLanguages[req.Language] {
		return errors.Validation("unsupported language")
	}

	// 4. Extract and Validate Video
	vFile, vHeader, err := r.FormFile("video")
	if err != nil {
		return errors.Validation("video file is required (form field: 'video')")
	}
	req.VideoFile = vFile

	req.VideoContentType = vHeader.Header.Get("Content-Type")
	if req.VideoContentType == "" {
		filename := strings.ToLower(vHeader.Filename)
		if strings.HasSuffix(filename, ".mp4") {
			req.VideoContentType = "video/mp4"
		} else if strings.HasSuffix(filename, ".mov") {
			req.VideoContentType = "video/quicktime"
		}
	}

	if !allowedVideoMIME[req.VideoContentType] {
		return errors.Validation("invalid video file type, allowed: mp4, mov, avi, webm")
	}

	// --- 5. Extract and Validate Thumbnail ---
	tFile, tHeader, err := r.FormFile("thumbnail")
	if err != nil {
		return errors.Validation("thumbnail file is required (form field: 'thumbnail')")
	}
	req.ThumbnailFile = tFile

	req.ThumbnailContentType = tHeader.Header.Get("Content-Type")
	if req.ThumbnailContentType == "" {
		filename := strings.ToLower(tHeader.Filename)
		if strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".jpeg") {
			req.ThumbnailContentType = "image/jpeg"
		} else if strings.HasSuffix(filename, ".png") {
			req.ThumbnailContentType = "image/png"
		} else if strings.HasSuffix(filename, ".webp") {
			req.ThumbnailContentType = "image/webp"
		}
	}

	if !allowedImageMIME[req.ThumbnailContentType] {
		return errors.Validation("invalid thumbnail type, allowed: jpeg, png, webp")
	}

	return nil
}

// ToPayload convert UploadVideoRequest to UploadVideoPayload
func (req *UploadVideoRequest) ToPayload() UploadVideoPayload {
	videoID := uuid.New().String()

	videoExt, ok := mimeToExt[req.VideoContentType]
	if !ok {
		videoExt = ".mp4"
	}

	thumbExt, ok := mimeToExt[req.ThumbnailContentType]
	if !ok {
		thumbExt = ".webp"
	}

	audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_audio.wav", videoID))
	videoPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_video%s", videoID, videoExt))
	thumbPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s_thumb%s", videoID, thumbExt))
	videoR2Path := fmt.Sprintf("videos/%s%s", videoID, videoExt)
	thumbR2Path := fmt.Sprintf("thumbnails/%s%s", videoID, thumbExt)

	return UploadVideoPayload{
		UserID:               req.UserID,
		VideoID:              videoID,
		Language:             req.Language,
		VideoExt:             videoExt,
		VideoPath:            videoPath,
		VideoFile:            req.VideoFile,
		VideoContentType:     req.VideoContentType,
		VideoR2Path:          videoR2Path,
		ThumbnailExt:         thumbExt,
		ThumbnailPath:        thumbPath,
		ThumbnailFile:        req.ThumbnailFile,
		ThumbnailContentType: req.ThumbnailContentType,
		ThumbnailR2Path:      thumbR2Path,
		AudioPath:            audioPath,
	}
}

// -------------------------------------------------------------------------
// List Video Contents Request
// -------------------------------------------------------------------------

// ListVideoContentsRequest is the HTTP request struct for listing video contents
type ListVideoContentsRequest struct {
	Page     int
	PageSize int
}

// ListVideoContentsInput is the input struct for service
type ListVideoContentsInput struct {
	Page     int
	PageSize int
	Limit    int
	Offset   int
}

// Parse parse pagination params
func (req *ListVideoContentsRequest) Parse(r *http.Request) {
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

// ToInput convert ListVideoContentsRequest to ListVideoContentsInput
func (req *ListVideoContentsRequest) ToInput() ListVideoContentsInput {
	limit := req.PageSize
	offset := (req.Page - 1) * req.PageSize

	return ListVideoContentsInput{
		Page:     req.Page,
		PageSize: req.PageSize,
		Limit:    limit,
		Offset:   offset,
	}
}

// -------------------------------------------------------------------------
// Start Quiz Request
// -------------------------------------------------------------------------

// StartQuizRequest is the HTTP request struct for starting a quiz
type StartQuizRequest struct {
	UserID  string
	VideoID string
}

// StartQuizInput is the input struct for service
type StartQuizInput struct {
	UserID  string
	VideoID string
}

func (req *StartQuizRequest) ParseAndValidate(r *http.Request) error {
	// 1. Get user ID from auth context
	req.UserID = middleware.GetUserID(r.Context())
	if req.UserID == "" {
		return errors.Unauthorized("user not authenticated")
	}

	// 2. Parse URL Params
	req.VideoID = chi.URLParam(r, "videoID")
	if req.VideoID == "" {
		return errors.Validation("Video ID is required")
	}

	return nil
}

func (req *StartQuizRequest) ToInput() StartQuizInput {
	return StartQuizInput{
		UserID:  req.UserID,
		VideoID: req.VideoID,
	}
}

// -------------------------------------------------------------------------
// Start Retell Request
// -------------------------------------------------------------------------

// StartRetellRequest is the HTTP request struct for starting a retell story
type StartRetellRequest struct {
	UserID  string
	VideoID string
}

// StartRetellInput is the input struct for service
type StartRetellInput struct {
	UserID  string
	VideoID string
}

func (req *StartRetellRequest) ParseAndValidate(r *http.Request) error {
	// 1. Get user ID from auth context
	req.UserID = middleware.GetUserID(r.Context())
	if req.UserID == "" {
		return errors.Unauthorized("user not authenticated")
	}

	// 2. Parse URL Params
	req.VideoID = chi.URLParam(r, "videoID")
	if req.VideoID == "" {
		return errors.Validation("Video ID is required")
	}

	return nil
}

func (req *StartRetellRequest) ToInput() StartRetellInput {
	return StartRetellInput{
		UserID:  req.UserID,
		VideoID: req.VideoID,
	}
}

// -------------------------------------------------------------------------
// Submit Gist Quiz Request
// -------------------------------------------------------------------------

// QuizAnswer is the individual answer for a quiz question
type QuizAnswer struct {
	QuizID    int      `json:"quiz_id"`
	Type      string   `json:"type"`
	OptionIDs []string `json:"option_ids,omitempty"`
	Order     []string `json:"order,omitempty"`
}

// SubmitGistQuizRequest is the HTTP request struct for submitting a gist quiz
type SubmitGistQuizRequest struct {
	UserID  string
	VideoID string
	Answers []QuizAnswer `json:"answers"`
}

// SubmitGistQuizInput is the input struct for service
type SubmitGistQuizInput struct {
	UserID  string
	VideoID string
	Answers []QuizAnswer
}

func (req *SubmitGistQuizRequest) ParseAndValidate(r *http.Request) error {
	// 1. Get user ID from auth context
	req.UserID = middleware.GetUserID(r.Context())
	if req.UserID == "" {
		return errors.Unauthorized("user not authenticated")
	}

	// 2. Parse URL Params
	req.VideoID = chi.URLParam(r, "videoID")
	if req.VideoID == "" {
		return errors.Validation("Video ID is required")
	}

	// 3. Parse JSON Body
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return errors.Validation("invalid JSON body")
	}

	if len(req.Answers) == 0 {
		return errors.Validation("answers cannot be empty")
	}

	return nil
}

func (req *SubmitGistQuizRequest) ToInput() SubmitGistQuizInput {
	return SubmitGistQuizInput{
		UserID:  req.UserID,
		VideoID: req.VideoID,
		Answers: req.Answers,
	}
}

// -------------------------------------------------------------------------
// Submit Retell Request
// -------------------------------------------------------------------------

// SubmitRetellRequest is the HTTP request struct for submitting a retell story
type SubmitRetellRequest struct {
	UserID      string
	VideoID     string
	Language    string
	AudioFile   multipart.File
	AudioHeader *multipart.FileHeader
}

// SubmitRetellPayload is the payload struct for service
type SubmitRetellPayload struct {
	UserID       string
	VideoID      string
	AttemptID    string
	Language     string
	AudioFile    multipart.File
	AudioR2Path  string
	AudioM4aPath string
	AudioWavPath string
	AudioType    string
}

func (req *SubmitRetellRequest) ParseAndValidate(r *http.Request) error {
	// 1. Get user ID from auth context
	req.UserID = middleware.GetUserID(r.Context())
	if req.UserID == "" {
		return errors.Unauthorized("user not authenticated")
	}

	// 2. Parse URL Params
	req.VideoID = chi.URLParam(r, "videoID")
	if req.VideoID == "" {
		return errors.Validation("Video ID is required")
	}

	// 3. Extract Language Header & Validate
	req.Language = strings.ToLower(r.Header.Get("Language"))
	if !AllowedLanguages[req.Language] {
		return errors.Validation("unsupported language")
	}

	// 4. Parse multipart body
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return errors.Validation("invalid multipart body")
	}

	// 5. Extract audio file
	audioFile, audioHeader, err := r.FormFile("audio")
	if err != nil {
		return errors.Validation("audio file is required (form field: 'audio')")
	}
	defer audioFile.Close()

	req.AudioFile = audioFile
	req.AudioHeader = audioHeader
	return nil
}

func (req *SubmitRetellRequest) ToPayload() SubmitRetellPayload {
	attemptID := uuid.New().String()

	audioR2Path := fmt.Sprintf("retell-story/%s.m4a", attemptID)
	audioWavPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s.wav", attemptID))
	audioM4aPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s.m4a", attemptID))

	return SubmitRetellPayload{
		AttemptID:    attemptID,
		UserID:       req.UserID,
		VideoID:      req.VideoID,
		Language:     req.Language,
		AudioFile:    req.AudioFile,
		AudioR2Path:  audioR2Path,
		AudioWavPath: audioWavPath,
		AudioM4aPath: audioM4aPath,
		AudioType:    "audio/m4a",
	}
}

// -------------------------------------------------------------------------
// Toggle Saved Request
// -------------------------------------------------------------------------

// ToggleSavedRequest is the HTTP request struct for toggling saved status
type ToggleSavedRequest struct {
	UserID  string
	VideoID string
}

// ToggleSavedInput is the input struct for service
type ToggleSavedInput struct {
	UserID  string
	VideoID string
}

func (req *ToggleSavedRequest) ParseAndValidate(r *http.Request) error {
	// 1. Get user ID from auth context
	req.UserID = middleware.GetUserID(r.Context())
	if req.UserID == "" {
		return errors.Unauthorized("user not authenticated")
	}

	// 2. Parse URL Params
	req.VideoID = chi.URLParam(r, "videoID")
	if req.VideoID == "" {
		return errors.Validation("Video ID is required")
	}

	return nil
}

func (req *ToggleSavedRequest) ToInput() ToggleSavedInput {
	return ToggleSavedInput{
		UserID:  req.UserID,
		VideoID: req.VideoID,
	}
}
