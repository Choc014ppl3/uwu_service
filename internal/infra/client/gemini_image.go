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
	"golang.org/x/oauth2/google"
)

// GeminiImageClient wraps Vertex AI Imagen 3 Flash model.
type GeminiImageClient struct {
	projectID string
	location  string
	saJSON    []byte
	client    *http.Client
}

// NewGeminiImageClient creates a new Gemini image client from a Base64-encoded Service Account JSON.
func NewGeminiImageClient(saBase64, location string) (*GeminiImageClient, error) {
	if saBase64 == "" {
		return nil, fmt.Errorf("gemini SA credentials not configured")
	}

	saJSON, err := base64.StdEncoding.DecodeString(saBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Base64 SA JSON: %v", err)
	}

	// Extract project_id from SA JSON
	var sa struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(saJSON, &sa); err != nil {
		return nil, fmt.Errorf("failed to parse SA JSON for project_id: %v", err)
	}

	return &GeminiImageClient{
		projectID: sa.ProjectID,
		location:  location,
		saJSON:    saJSON,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

// GenerateImage creates a PNG image and returns the raw bytes.
func (c *GeminiImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, *errors.AppError) {
	// 1. Get Token
	creds, err := google.CredentialsFromJSON(ctx, c.saJSON, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, errors.InternalWrap("failed to get google credentials", err)
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		return nil, errors.InternalWrap("failed to get access token", err)
	}

	// 2. Model: imagen-3.0-fast-generate-001
	url := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/imagen-3.0-fast-generate-001:predict", c.location, c.projectID, c.location)

	// 3. Request Body
	reqBody := map[string]interface{}{
		"instances": []map[string]interface{}{
			{
				"prompt": prompt,
			},
		},
		"parameters": map[string]interface{}{
			"sampleCount": 1,
			"aspectRatio": "9:16",
			"outputOptions": map[string]interface{}{
				"mimeType": "image/png",
			},
		},
	}

	bodyJSON, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, errors.InternalWrap("failed to create gemini image request", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.InternalWrap("failed to send gemini image request", err)
	}
	defer resp.Body.Close()

	// 4. Error Handling
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, errors.InternalWrap("gemini image api error", fmt.Errorf("status code: %d, response body: %s", resp.StatusCode, string(respBody)))
	}

	// 5. Decode Response
	var result struct {
		Predictions []struct {
			BytesBase64Encoded string `json:"bytesBase64Encoded"`
			MimeType           string `json:"mimeType"`
		} `json:"predictions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.InternalWrap("failed to decode gemini image response", err)
	}

	if len(result.Predictions) == 0 || result.Predictions[0].BytesBase64Encoded == "" {
		return nil, errors.Internal("gemini image api returned no image data")
	}

	imageBytes, err := base64.StdEncoding.DecodeString(result.Predictions[0].BytesBase64Encoded)
	if err != nil {
		return nil, errors.InternalWrap("failed to decode base64 image data", err)
	}

	return imageBytes, nil
}
