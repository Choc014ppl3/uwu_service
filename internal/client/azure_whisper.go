package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/windfall/uwu_service/internal/errors"
)

// AzureWhisperClient wraps the Azure OpenAI Whisper REST API for audio transcription.
type AzureWhisperClient struct {
	endpoint string // e.g. https://your-resource.openai.azure.com
	apiKey   string
	client   *http.Client
}

// WhisperResponse is the verbose_json response from Azure OpenAI Whisper.
type WhisperResponse struct {
	Task     string           `json:"task"`
	Language string           `json:"language"`
	Duration float64          `json:"duration"`
	Text     string           `json:"text"`
	Segments []WhisperSegment `json:"segments"` // sentence-level (for subtitles)
	Words    []WhisperWord    `json:"words"`    // word-level (for karaoke highlighting)
}

// WhisperSegment represents a sentence-level segment with timing.
type WhisperSegment struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"` // seconds
	End   float64 `json:"end"`   // seconds
	Text  string  `json:"text"`
}

// WhisperWord represents a single word with timing (in seconds).
type WhisperWord struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// NewAzureWhisperClient creates a new Azure OpenAI Whisper client.
func NewAzureWhisperClient(endpoint, apiKey string) *AzureWhisperClient {
	return &AzureWhisperClient{
		endpoint: endpoint,
		apiKey:   apiKey,
		client: &http.Client{
			Timeout: 120 * time.Second, // Whisper can take longer for large files
		},
	}
}

// TranscribeFile sends a WAV audio file to Azure OpenAI Whisper for transcription.
// Returns the full WhisperResponse with word-level timestamps.
// lang is optional (e.g. "en", "th"); if empty, Whisper auto-detects.
func (c *AzureWhisperClient) TranscribeFile(ctx context.Context, wavPath, language string) (*WhisperResponse, error) {
	if c.apiKey == "" || c.endpoint == "" {
		return nil, errors.New(errors.ErrAIService, "Azure Whisper credentials not configured")
	}

	// Read the audio file
	audioData, err := os.ReadFile(wavPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio file: %w", err)
	}

	// Build multipart/form-data body
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add file field
	part, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(audioData); err != nil {
		return nil, fmt.Errorf("failed to write audio data: %w", err)
	}

	// Add response_format field (verbose_json for word-level timestamps)
	_ = writer.WriteField("response_format", "verbose_json")

	// Add language field
	_ = writer.WriteField("language", language)

	// Add timestamp granularities (segment and word)
	_ = writer.WriteField("timestamp_granularities[]", "segment")
	_ = writer.WriteField("timestamp_granularities[]", "word")

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure whisper api error %d: %s", resp.StatusCode, string(respBody))
	}

	var result WhisperResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}
