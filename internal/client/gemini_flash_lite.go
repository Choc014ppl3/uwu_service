package client

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// GeminiFlashLiteClient wraps the Google Gemini Flash Lite API client.
// Uses service account credentials for audio/live API features.
type GeminiFlashLiteClient struct {
	client *genai.Client
	model  string
}

// NewGeminiFlashLiteClient creates a new Gemini Flash Lite client using service account credentials.
func NewGeminiFlashLiteClient(ctx context.Context, credentialsPath string) (*GeminiFlashLiteClient, error) {
	if credentialsPath == "" {
		return nil, fmt.Errorf("gemini flash lite credentials path not configured")
	}

	client, err := genai.NewClient(ctx, option.WithCredentialsFile(credentialsPath))
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini flash lite client: %w", err)
	}

	return &GeminiFlashLiteClient{
		client: client,
		model:  "gemini-2.5-flash-lite",
	}, nil
}

// WithModel sets the model to use.
func (c *GeminiFlashLiteClient) WithModel(model string) *GeminiFlashLiteClient {
	c.model = model
	return c
}

// Close closes the client.
func (c *GeminiFlashLiteClient) Close() {
	if c.client != nil {
		c.client.Close()
	}
}

// GetClient returns the underlying genai client for advanced use cases (e.g., Live API).
func (c *GeminiFlashLiteClient) GetClient() *genai.Client {
	return c.client
}

// GetModel returns the current model name.
func (c *GeminiFlashLiteClient) GetModel() string {
	return c.model
}

// Chat sends a chat message and returns the response.
func (c *GeminiFlashLiteClient) Chat(ctx context.Context, message string) (string, error) {
	model := c.client.GenerativeModel(c.model)
	resp, err := model.GenerateContent(ctx, genai.Text(message))
	if err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", nil
	}

	// Extract text from response
	var result string
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			result += string(text)
		}
	}

	return result, nil
}

// ChatStream streams chat responses.
func (c *GeminiFlashLiteClient) ChatStream(ctx context.Context, message string, onChunk func(string) error) error {
	model := c.client.GenerativeModel(c.model)
	iter := model.GenerateContentStream(ctx, genai.Text(message))

	for {
		resp, err := iter.Next()
		if err == iterator.Done {
			return nil
		}
		if err != nil {
			return err
		}

		if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
			for _, part := range resp.Candidates[0].Content.Parts {
				if text, ok := part.(genai.Text); ok {
					if err := onChunk(string(text)); err != nil {
						return err
					}
				}
			}
		}
	}
}

// ChatWithHistory sends a chat with message history.
func (c *GeminiFlashLiteClient) ChatWithHistory(ctx context.Context, history []*genai.Content, message string) (string, error) {
	model := c.client.GenerativeModel(c.model)
	cs := model.StartChat()
	cs.History = history

	resp, err := cs.SendMessage(ctx, genai.Text(message))
	if err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", nil
	}

	var result string
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			result += string(text)
		}
	}

	return result, nil
}
