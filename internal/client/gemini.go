package client

import (
	"context"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// GeminiClient wraps the Google Gemini API client.
type GeminiClient struct {
	client *genai.Client
	model  string
}

// NewGeminiClient creates a new Gemini client.
func NewGeminiClient(ctx context.Context, apiKey string) (*GeminiClient, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}

	return &GeminiClient{
		client: client,
		model:  "gemini-2.0-flash",
	}, nil
}

// WithModel sets the model to use.
func (c *GeminiClient) WithModel(model string) *GeminiClient {
	c.model = model
	return c
}

// Close closes the client.
func (c *GeminiClient) Close() {
	if c.client != nil {
		c.client.Close()
	}
}

// Chat sends a chat message and returns the response.
func (c *GeminiClient) Chat(ctx context.Context, message string) (string, error) {
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

// Complete generates a completion for the given prompt.
func (c *GeminiClient) Complete(ctx context.Context, prompt string) (string, error) {
	return c.Chat(ctx, prompt)
}

// ChatWithHistory sends a chat with message history.
func (c *GeminiClient) ChatWithHistory(ctx context.Context, history []*genai.Content, message string) (string, error) {
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

// ChatStream streams chat responses.
func (c *GeminiClient) ChatStream(ctx context.Context, message string, onChunk func(string) error) error {
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
