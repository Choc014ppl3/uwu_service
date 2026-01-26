package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/genai"
)

// GeminiClient wraps the Google Vertex AI Gemini client.
type GeminiClient struct {
	client    *genai.Client
	model     string
	projectID string
	location  string
	creds     *google.Credentials // Store credentials for REST API calls
}

// NewGeminiClient creates a new Gemini client using Vertex AI.
func NewGeminiClient(ctx context.Context, projectID, location string, apiKey string) (*GeminiClient, error) {
	cfg := &genai.ClientConfig{
		Project:  projectID,
		Location: location,
		Backend:  genai.BackendVertexAI,
	}

	client, err := genai.NewClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Get default credentials for REST API calls
	creds, _ := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")

	return &GeminiClient{
		client:    client,
		model:     "gemini-2.0-flash",
		projectID: projectID,
		location:  location,
		creds:     creds,
	}, nil
}

// NewGeminiClientWithServiceAccount creates a new Gemini client using a service account file.
func NewGeminiClientWithServiceAccount(ctx context.Context, projectID, location, serviceAccountPath string) (*GeminiClient, error) {
	// Set the environment variable so the SDK can find the credentials
	if err := os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", serviceAccountPath); err != nil {
		return nil, fmt.Errorf("failed to set GOOGLE_APPLICATION_CREDENTIALS: %w", err)
	}

	cfg := &genai.ClientConfig{
		Project:  projectID,
		Location: location,
		Backend:  genai.BackendVertexAI,
	}

	client, err := genai.NewClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Get credentials from file for REST API calls
	creds, err := google.CredentialsFromJSON(ctx, mustReadFile(serviceAccountPath), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to load credentials from file: %w", err)
	}

	return &GeminiClient{
		client:    client,
		model:     "gemini-2.0-flash",
		projectID: projectID,
		location:  location,
		creds:     creds,
	}, nil
}

func mustReadFile(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

// WithModel sets the model to use.
func (c *GeminiClient) WithModel(model string) *GeminiClient {
	c.model = model
	return c
}

// Close closes the client.
func (c *GeminiClient) Close() {
	// No explicit close needed for new SDK
}

// Chat sends a chat message and returns the response.
func (c *GeminiClient) Chat(ctx context.Context, message string) (string, error) {
	resp, err := c.client.Models.GenerateContent(ctx, c.model, genai.Text(message), nil)
	if err != nil {
		return "", err
	}
	return resp.Text(), nil
}

// Complete generates a completion for the given prompt.
func (c *GeminiClient) Complete(ctx context.Context, prompt string) (string, error) {
	return c.Chat(ctx, prompt)
}

// ChatStream streams chat responses.
func (c *GeminiClient) ChatStream(ctx context.Context, message string, onChunk func(string) error) error {
	stream := c.client.Models.GenerateContentStream(ctx, c.model, genai.Text(message), nil)

	for resp, err := range stream {
		if err != nil {
			return err
		}
		if err := onChunk(resp.Text()); err != nil {
			return err
		}
	}
	return nil
}

// GenerateImage generates an image from a prompt using Imagen via REST API.
func (c *GeminiClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	url := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:predict",
		c.location, c.projectID, c.location, "imagen-3.0-generate-001")

	reqBody := map[string]interface{}{
		"instances": []map[string]interface{}{
			{"prompt": prompt},
		},
		"parameters": map[string]interface{}{
			"sampleCount":      1,
			"aspectRatio":      "9:16",
			"personGeneration": "allow_adult",
		},
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	// Use stored credentials or find default
	var token *oauth2.Token
	if c.creds != nil {
		token, err = c.creds.TokenSource.Token()
	} else {
		creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			return nil, fmt.Errorf("failed to get credentials: %w", err)
		}
		token, err = creds.TokenSource.Token()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imagen api error: status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	predictions, ok := result["predictions"].([]interface{})
	if !ok || len(predictions) == 0 {
		return nil, fmt.Errorf("no predictions found")
	}

	firstPred := predictions[0]
	var b64Str string
	if str, ok := firstPred.(string); ok {
		b64Str = str
	} else if obj, ok := firstPred.(map[string]interface{}); ok {
		if val, ok := obj["bytesBase64Encoded"].(string); ok {
			b64Str = val
		} else if val, ok := obj["image"].(string); ok {
			b64Str = val
		} else {
			return nil, fmt.Errorf("unknown prediction format")
		}
	} else {
		return nil, fmt.Errorf("unknown prediction type")
	}

	return base64.StdEncoding.DecodeString(b64Str)
}

// Ensure option import is used (for future use)
var _ = option.WithCredentialsFile
