package client

import (
	"context"
	"io"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAIClient wraps the OpenAI API client.
type OpenAIClient struct {
	client *openai.Client
	model  string
}

// NewOpenAIClient creates a new OpenAI client.
func NewOpenAIClient(apiKey string) *OpenAIClient {
	return &OpenAIClient{
		client: openai.NewClient(apiKey),
		model:  openai.GPT4oMini,
	}
}

// WithModel sets the model to use.
func (c *OpenAIClient) WithModel(model string) *OpenAIClient {
	c.model = model
	return c
}

// Chat sends a chat message and returns the response.
func (c *OpenAIClient) Chat(ctx context.Context, message string) (string, error) {
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: message,
			},
		},
	})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", nil
	}

	return resp.Choices[0].Message.Content, nil
}

// Complete generates a completion for the given prompt.
func (c *OpenAIClient) Complete(ctx context.Context, prompt string) (string, error) {
	return c.Chat(ctx, prompt)
}

// ChatWithHistory sends a chat with message history.
func (c *OpenAIClient) ChatWithHistory(ctx context.Context, messages []openai.ChatCompletionMessage) (string, error) {
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: messages,
	})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", nil
	}

	return resp.Choices[0].Message.Content, nil
}

// ChatStream streams chat responses.
func (c *OpenAIClient) ChatStream(ctx context.Context, message string, onChunk func(string) error) error {
	stream, err := c.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: message,
			},
		},
		Stream: true,
	})
	if err != nil {
		return err
	}
	defer stream.Close()

	for {
		response, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if len(response.Choices) > 0 {
			if err := onChunk(response.Choices[0].Delta.Content); err != nil {
				return err
			}
		}
	}
}

// CreateEmbedding creates an embedding for the given text.
func (c *OpenAIClient) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.AdaEmbeddingV2,
		Input: []string{text},
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 {
		return nil, nil
	}

	return resp.Data[0].Embedding, nil
}
