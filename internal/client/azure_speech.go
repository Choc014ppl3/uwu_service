package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/windfall/uwu_service/internal/errors"
)

// AzureSpeechClient wraps the Azure AI Speech REST API.
type AzureSpeechClient struct {
	apiKey string
	region string
	client *http.Client
}

// NewAzureSpeechClient creates a new Azure Speech client.
func NewAzureSpeechClient(apiKey, region string) *AzureSpeechClient {
	return &AzureSpeechClient{
		apiKey: apiKey,
		region: region,
		client: &http.Client{},
	}
}

// AnalyzeAudio sends an audio file to Azure Speech-to-Text API.
// It accepts wav audio data and returns the raw JSON response.
func (c *AzureSpeechClient) AnalyzeAudio(ctx context.Context, audioData []byte, referenceText string) (map[string]interface{}, error) {
	if c.apiKey == "" || c.region == "" {
		return nil, errors.New(errors.ErrAIService, "Azure Speech credentials not configured")
	}

	// Construct URL for Short Audio API (REST)
	// Docs: https://learn.microsoft.com/en-us/azure/ai-services/speech-service/rest-speech-to-text-short
	u := url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s.stt.speech.microsoft.com", c.region),
		Path:   "/speech/recognition/conversation/cognitiveservices/v1",
	}

	// Query parameters
	q := u.Query()
	q.Set("language", "en-US")  // Default to English, can be parameterized if needed
	q.Set("format", "detailed") // Request detailed output
	u.RawQuery = q.Encode()

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(audioData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 1. สร้าง Config Object สำหรับการตรวจ
	pronAssessmentParams := map[string]interface{}{
		"ReferenceText": referenceText, // เช่น "Apple"
		"GradingSystem": "HundredMark", // คะแนนเต็ม 100
		"Granularity":   "Phoneme",     // ขอรายละเอียดระดับตัวสะกด
		"Dimension":     "Comprehensive",
	}

	// 2. แปลงเป็น JSON Bytes
	jsonBytes, err := json.Marshal(pronAssessmentParams)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	// 3. แปลง JSON เป็น Base64 String
	base64Params := base64.StdEncoding.EncodeToString(jsonBytes)

	// 4. ยัดใส่ Header "Pronunciation-Assessment"
	req.Header.Set("Pronunciation-Assessment", base64Params)

	// Set headers
	req.Header.Set("Ocp-Apim-Subscription-Key", c.apiKey)
	req.Header.Set("Content-Type", "audio/wav; codecs=audio/pcm; samplerate=16000")
	req.Header.Set("Accept", "application/json;text/xml")

	// Execute request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure speech api error %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}
