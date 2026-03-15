package video

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

	// 3. Extract Language Header
	req.Language = r.Header.Get("Language")
	if req.Language == "" {
		req.Language = "English"
	}

	// --- 4. Extract and Validate Video ---
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
	videoR2Path := fmt.Sprintf("videos/%s.%s", videoID, videoExt)
	thumbR2Path := fmt.Sprintf("thumbnails/%s.%s", videoID, thumbExt)

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
