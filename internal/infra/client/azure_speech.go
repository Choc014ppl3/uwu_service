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
	"time"

	"github.com/windfall/uwu_service/pkg/errors"
)

// AzureSpeechClient wraps Azure AI Speech text-to-speech.
type AzureSpeechClient struct {
	apiKey string
	region string
	client *http.Client
}

// NewAzureSpeechClient creates a new Azure speech client.
func NewAzureSpeechClient(apiKey, region string) *AzureSpeechClient {
	return &AzureSpeechClient{
		apiKey: apiKey,
		region: region,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Synthesize generates speech from text using Azure AI Speech.
func (c *AzureSpeechClient) Synthesize(ctx context.Context, text, voice string) ([]byte, *errors.AppError) {
	if c.apiKey == "" || c.region == "" {
		return nil, errors.Internal("Azure speech credentials not configured")
	}

	if voice == "" {
		voice = "en-US-AvaMultilingualNeural"
	}

	u := url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s.tts.speech.microsoft.com", c.region),
		Path:   "/cognitiveservices/v1",
	}

	ssml := fmt.Sprintf(
		"<speak version='1.0' xml:lang='en-US'><voice xml:lang='en-US' xml:gender='Female' name='%s'>%s</voice></speak>",
		voice,
		text,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewBufferString(ssml))
	if err != nil {
		return nil, errors.InternalWrap("failed to create azure speech request", err)
	}

	req.Header.Set("Ocp-Apim-Subscription-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/ssml+xml")
	req.Header.Set("X-Microsoft-OutputFormat", "audio-16khz-128kbitrate-mono-mp3")
	req.Header.Set("User-Agent", "uwu_service")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.InternalWrap("failed to send azure speech request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Internal(fmt.Sprintf("azure speech api error %d: %s", resp.StatusCode, string(body)))
	}

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.InternalWrap("failed to read azure speech response", err)
	}

	return audioBytes, nil
}

// EvaluatePronunciation assesses pronunciation of audio bytes against a reference text.
func (c *AzureSpeechClient) EvaluatePronunciation(ctx context.Context, audioBytes []byte, referenceText string, language string) (map[string]interface{}, *errors.AppError) {
	if c.apiKey == "" || c.region == "" {
		return nil, errors.Internal("Azure speech credentials not configured")
	}

	if language == "" {
		language = "en-US"
	}

	u := url.URL{
		Scheme:   "https",
		Host:     fmt.Sprintf("%s.stt.speech.microsoft.com", c.region),
		Path:     "/speech/recognition/conversation/cognitiveservices/v1",
		RawQuery: fmt.Sprintf("language=%s", url.QueryEscape(language)),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(audioBytes))
	if err != nil {
		return nil, errors.InternalWrap("failed to create azure speech recognition request", err)
	}

	// Create Pronunciation Assessment config
	assessmentConfig := map[string]interface{}{
		"ReferenceText": referenceText,
		"GradingSystem": "HundredMark",
		"Granularity":   "Phoneme",
		"Dimension":     "Comprehensive",
	}

	configJSON, err := json.Marshal(assessmentConfig)
	if err != nil {
		return nil, errors.InternalWrap("failed to encode pronunciation config", err)
	}

	encodedConfig := base64.StdEncoding.EncodeToString(configJSON)

	req.Header.Set("Ocp-Apim-Subscription-Key", c.apiKey)
	req.Header.Set("Content-Type", "audio/wav; codecs=audio/pcm; samplerate=16000") // Assuming standard 16kHz WAV
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Pronunciation-Assessment", encodedConfig)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.InternalWrap("failed to send azure speech recognition request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Internal(fmt.Sprintf("azure speech recognition api error %d: %s", resp.StatusCode, string(body)))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.InternalWrap("failed to decode azure speech recognition response", err)
	}

	return result, nil
}
