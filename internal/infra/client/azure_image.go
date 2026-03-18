package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/windfall/uwu_service/pkg/errors"
)

// AzureImageClient wraps Azure OpenAI image generation.
type AzureImageClient struct {
	endpoint string
	apiKey   string
	model    string
	client   *http.Client
}

type azureImageRequest struct {
	Prompt       string `json:"prompt"`
	Model        string `json:"model"`
	Size         string `json:"size,omitempty"`
	N            int    `json:"n,omitempty"`
	Quality      string `json:"quality,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
}

type azureImageResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// NewAzureImageClient creates a new Azure image client.
func NewAzureImageClient(endpoint, apiKey string) *AzureImageClient {
	return &AzureImageClient{
		endpoint: endpoint,
		apiKey:   apiKey,
		model:    "gpt-image-1-mini",
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// GenerateImage creates a PNG image and returns the raw bytes.
func (c *AzureImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, *errors.AppError) {
	if c.endpoint == "" || c.apiKey == "" {
		return nil, errors.Internal("Azure image credentials not configured")
	}

	reqBody := azureImageRequest{
		Prompt:       prompt,
		Model:        c.model,
		Size:         "1024x1536",
		N:            1,
		Quality:      "medium",
		OutputFormat: "PNG",
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errors.InternalWrap("failed to marshal azure image request", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, errors.InternalWrap("failed to create azure image request", err)
	}

	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.InternalWrap("failed to send azure image request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, errors.InternalWrap("azure image api error", fmt.Errorf("status code: %d, response body: %s", resp.StatusCode, string(respBody)))
	}

	var result azureImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.InternalWrap("failed to decode azure image response", err)
	}

	if result.Error != nil {
		return nil, errors.Internal(result.Error.Message)
	}
	if len(result.Data) == 0 || result.Data[0].B64JSON == "" {
		return nil, errors.Internal("azure image api returned no image data")
	}

	imageBytes, err := base64.StdEncoding.DecodeString(result.Data[0].B64JSON)
	if err != nil {
		return nil, errors.InternalWrap("failed to decode azure image response data", err)
	}

	return imageBytes, nil
}
