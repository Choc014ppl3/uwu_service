package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/windfall/uwu_service/pkg/errors"
)

// AzureChatGPTClient wraps the Azure OpenAI Chat Completions REST API.
type AzureChatGPTClient struct {
	endpoint string // e.g. https://your-resource.openai.azure.com
	apiKey   string
	client   *http.Client
}

// ChatMessage is a single message in the chat history.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the request body for the Chat Completions API.
type chatRequest struct {
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
}

// chatResponse is the response from the Chat Completions API.
type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message ChatMessage `json:"message"`
}

// NewAzureChatGPTClient creates a new Azure OpenAI Chat Completions client.
func NewAzureChatGPTClient(endpoint, apiKey string) *AzureChatGPTClient {
	return &AzureChatGPTClient{
		endpoint: endpoint,
		apiKey:   apiKey,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// ChatCompletion sends a system prompt + user message to Azure OpenAI Chat Completions
// and returns the assistant's response text.
func (c *AzureChatGPTClient) ChatCompletion(ctx context.Context, systemPrompt, userMessage string) (string, *errors.AppError) {
	if c.apiKey == "" || c.endpoint == "" {
		return "", errors.Internal("Azure OpenAI Chat credentials not configured")
	}

	reqBody := chatRequest{
		Messages: []ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		// Note: Temperature omitted — GPT-5 Nano only supports default (1)
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", errors.InternalWrap("failed to marshal request", err)
	}

	// Azure OpenAI Chat Completions endpoint
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", errors.InternalWrap("failed to create request", err)
	}

	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", errors.InternalWrap("failed to send request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", errors.InternalWrap("azure openai chat api error", fmt.Errorf("status code: %d, response body: %s", resp.StatusCode, string(respBody)))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errors.InternalWrap("failed to decode response", err)
	}

	if len(result.Choices) == 0 {
		return "", errors.Internal("no choices returned from azure openai")
	}

	return result.Choices[0].Message.Content, nil
}

// ChatCompletionMultiTurn sends a full message history to Azure OpenAI Chat Completions
// and returns the assistant's response text. Use this for multi-turn conversations.
func (c *AzureChatGPTClient) ChatCompletionMultiTurn(ctx context.Context, messages []ChatMessage) (string, *errors.AppError) {
	if c.apiKey == "" || c.endpoint == "" {
		return "", errors.Internal("Azure OpenAI Chat credentials not configured")
	}

	reqBody := chatRequest{Messages: messages}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", errors.InternalWrap("failed to marshal request", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", errors.InternalWrap("failed to create request", err)
	}

	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", errors.InternalWrap("failed to send request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", errors.InternalWrap("azure openai chat api error", fmt.Errorf("status code: %d, response body: %s", resp.StatusCode, string(respBody)))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errors.InternalWrap("failed to decode response", err)
	}

	if len(result.Choices) == 0 {
		return "", errors.Internal("no choices returned from azure openai")
	}

	return result.Choices[0].Message.Content, nil
}
