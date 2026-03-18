package client

import (
	"bytes"
	"context"
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
