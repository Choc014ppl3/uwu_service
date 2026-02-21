package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/windfall/uwu_service/internal/errors"
)

// AzureChatClient wraps the Azure OpenAI Chat Completions REST API.
type AzureChatClient struct {
	endpoint string // e.g. https://your-resource.openai.azure.com
	apiKey   string
	client   *http.Client
}

// chatRequest is the request body for the Chat Completions API.
type chatRequest struct {
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
}

// chatMessage is a single message in the chat history.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the response from the Chat Completions API.
type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

// chatChoice is a single completion choice.
type chatChoice struct {
	Message chatMessage `json:"message"`
}

// NewAzureChatClient creates a new Azure OpenAI Chat Completions client.
func NewAzureChatClient(endpoint, apiKey string) *AzureChatClient {
	return &AzureChatClient{
		endpoint: endpoint,
		apiKey:   apiKey,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// ChatCompletion sends a system prompt + user message to Azure OpenAI Chat Completions
// and returns the assistant's response text.
func (c *AzureChatClient) ChatCompletion(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	if c.apiKey == "" || c.endpoint == "" {
		return "", errors.New(errors.ErrAIService, "Azure OpenAI Chat credentials not configured")
	}

	reqBody := chatRequest{
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		// Note: Temperature omitted — GPT-5 Nano only supports default (1)
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Azure OpenAI Chat Completions endpoint
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("azure openai chat api error %d: %s", resp.StatusCode, string(respBody))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from azure openai")
	}

	return result.Choices[0].Message.Content, nil
}
