package video

import (
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"os"
	"os/exec"

	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/pkg/errors"
)

// FileRepository interface
type FileRepository interface {
	GetMediaURL(pattern string) (string, *errors.AppError)
	ExtractAudio(ctx context.Context, videoPath, audioPath string) *errors.AppError
	UploadToR2(ctx context.Context, src multipart.File, key, path, contentType string) (string, *errors.AppError)
	UploadReaderToR2(ctx context.Context, audioM4APath, key, contentType string) (string, *errors.AppError)
	ConvertAudioToM4A(ctx context.Context, srcPath, dstPath string) *errors.AppError
	CreateTempFile(file multipart.File, pattern string) (*os.File, *errors.AppError)
}

// fileRepository is the implementation of the FileRepository interface
type fileRepository struct {
	cloudflare *client.CloudflareClient
	log        *slog.Logger
}

// NewFileRepository creates a new fileRepository
func NewFileRepository(cloudflare *client.CloudflareClient, log *slog.Logger) *fileRepository {
	return &fileRepository{cloudflare: cloudflare, log: log}
}

// GetMediaURL generates a temporary file path
func (r *fileRepository) GetMediaURL(key string) (string, *errors.AppError) {
	url := r.cloudflare.GetR2ObjectURL(key)
	if url == "" {
		return "", errors.Internal("failed to get media URL")
	}
	return url, nil
}

// ExtractAudio extracts audio from a video file
func (r *fileRepository) ExtractAudio(ctx context.Context, videoPath, audioPath string) *errors.AppError {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", videoPath,
		"-vn",
		"-acodec", "pcm_s16le",
		"-ar", "16000",
		"-ac", "1",
		"-y",
		audioPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		r.log.Error("FFmpeg audio extraction failed", "error", err.Error(), "ffmpeg_output", string(output))
		return errors.InternalWrap("ffmpeg audio extraction", err)
	}

	return nil
}

// UploadToR2 uploads a file to R2
func (r *fileRepository) UploadToR2(ctx context.Context, src multipart.File, key, path, contentType string) (string, *errors.AppError) {
	// Save file to temp location
	dst, err := os.Create(path)
	if err != nil {
		return "", errors.InternalWrap("create temp file", err)
	}
	defer dst.Close()

	// Copy file to temp location
	if _, err := io.Copy(dst, src); err != nil {
		return "", errors.InternalWrap("write temp file", err)
	}

	// Open file
	file, err := os.Open(path)
	if err != nil {
		return "", errors.InternalWrap("open output file", err)
	}
	defer file.Close()

	// Upload file to R2
	url, err := r.cloudflare.UploadR2Object(ctx, key, file, contentType)
	if err != nil {
		return "", errors.InternalWrap("upload to R2", err)
	}

	return url, nil
}

// UploadReaderToR2 uploads an io.Reader directly to R2 without saving to a temp file.
func (r *fileRepository) UploadReaderToR2(ctx context.Context, audioM4APath, key, contentType string) (string, *errors.AppError) {
	file, openErr := os.Open(audioM4APath)
	if openErr != nil {
		return "", errors.InternalWrap("failed to open m4a file", openErr)
	}
	defer file.Close()

	url, err := r.cloudflare.UploadR2Object(ctx, key, file, contentType)
	if err != nil {
		return "", errors.InternalWrap("upload to R2", err)
	}
	return url, nil
}

// ConvertAudioToM4A converts a WAV audio file to M4A using ffmpeg.
func (r *fileRepository) ConvertAudioToM4A(ctx context.Context, srcPath, dstPath string) *errors.AppError {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", srcPath,
		"-af", "afftdn,loudnorm=I=-16:TP=-1.5:LRA=11",
		"-c:a", "aac", "-b:a", "64k", "-ac", "1",
		"-ar", "16000", "-movflags", "faststart",
		dstPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		r.log.Error("FFmpeg audio conversion failed", "error", err.Error(), "ffmpeg_output", string(output))
		return errors.InternalWrap("ffmpeg audio conversion", err)
	}

	return nil
}

// CreateTempFile saves a multipart file to a temporary file.
func (r *fileRepository) CreateTempFile(file multipart.File, tempPath string) (*os.File, *errors.AppError) {
	// 1. ตรวจสอบว่าไฟล์ต้นทางไม่ได้ว่างเปล่า หรือหัวอ่านค้างอยู่ที่ท้ายไฟล์
	// (หัวอ่านของ multipart.File อาจจะขยับไปแล้วถ้ามีการตรวจสอบไฟล์ก่อนหน้านี้)
	if seeker, ok := file.(io.ReadSeeker); ok {
		_, _ = seeker.Seek(0, 0)
	}

	// 2. สร้างไฟล์ชั่วคราว
	tempFile, err := os.Create(tempPath)
	if err != nil {
		r.log.Error("Failed to create temp file", "error", err.Error())
		return nil, errors.InternalWrap("failed to create temp file", err)
	}

	// 3. ใช้ io.Copy และเช็คจำนวน Byte ที่เขียนได้ (ถ้าเขียนได้ 0 แปลว่าไฟล์ต้นทางว่าง)
	written, err := io.Copy(tempFile, file)
	if err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		r.log.Error("Failed to write to temp file", "error", err.Error())
		return nil, errors.InternalWrap("failed to write to temp file", err)
	}

	if written == 0 {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		r.log.Error("Source file is empty (0 bytes)")
		return nil, errors.Validation("source file is empty (0 bytes)")
	}

	// 4. กรอเทปกลับมาที่จุดเริ่ม เพื่อให้คนรับไปใช้งานต่อ (เช่น Upload) อ่านได้ทันที
	if _, err := tempFile.Seek(0, 0); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		r.log.Error("Failed to seek temp file", "error", err.Error())
		return nil, errors.InternalWrap("failed to seek temp file", err)
	}

	return tempFile, nil
}
