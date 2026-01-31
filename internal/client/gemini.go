package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	aiplatform "cloud.google.com/go/aiplatform/apiv1"
	"cloud.google.com/go/aiplatform/apiv1/aiplatformpb"
	"cloud.google.com/go/vertexai/genai"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/structpb"
)

// GeminiClient wraps the Google Vertex AI Gemini client.
type GeminiClient struct {
	client    *genai.Client
	model     string
	projectID string
	location  string
	creds     *google.Credentials // Store credentials for REST API calls
}

// NewGeminiClientWithServiceAccount creates a new Gemini client using a service account file.
func NewGeminiClientWithServiceAccount(ctx context.Context, projectID, location, serviceAccountPath string) (*GeminiClient, error) {
	// 1. อ่านไฟล์ JSON ออกมาเป็น bytes ก่อน (จำเป็นสำหรับ google.CredentialsFromJSON)
	jsonKey, err := os.ReadFile(serviceAccountPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read service account file: %w", err)
	}

	// 2. แปลง bytes เป็น Credentials Object
	// (เก็บใส่ตัวแปร creds ไว้ เพื่อยัดใส่ Struct บรรทัดสุดท้าย)
	creds, err := google.CredentialsFromJSON(ctx, jsonKey, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to create credentials from json: %w", err)
	}

	// 3. สร้าง Client ของ genai (Chat)
	// สังเกต: เราส่ง option.WithCredentials(creds) เข้าไปได้เลย ไม่ต้องอ่านไฟล์ซ้ำ
	client, err := genai.NewClient(ctx, projectID, location, option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("failed to create vertex ai client: %w", err)
	}

	return &GeminiClient{
		client:    client,
		model:     "gemini-2.5-flash-lite",
		projectID: projectID,
		location:  location,
		creds:     creds,
	}, nil
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

// GenerateImage generates an image from a prompt using Imagen via Vertex AI SDK.
func (c *GeminiClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	// Create prediction client with credentials
	var clientOpts []option.ClientOption
	if c.creds != nil {
		clientOpts = append(clientOpts, option.WithCredentials(c.creds))
	}
	clientOpts = append(clientOpts, option.WithEndpoint(fmt.Sprintf("%s-aiplatform.googleapis.com:443", c.location)))

	predClient, err := aiplatform.NewPredictionClient(ctx, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create prediction client: %w", err)
	}
	defer predClient.Close()

	endpoint := fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s",
		c.projectID, c.location, "imagen-3.0-fast-generate-001")

	// Build parameters
	params, err := structpb.NewValue(map[string]interface{}{
		"sampleCount": 1,
		"aspectRatio": "9:16",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create parameters: %w", err)
	}

	// Build instance with prompt
	inst, err := structpb.NewValue(map[string]interface{}{
		"prompt": prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}

	req := &aiplatformpb.PredictRequest{
		Endpoint:   endpoint,
		Instances:  []*structpb.Value{inst},
		Parameters: params,
	}

	resp, err := predClient.Predict(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("prediction failed: %w", err)
	}

	if len(resp.Predictions) == 0 {
		return nil, fmt.Errorf("no predictions found")
	}

	// Extract base64 image from response
	firstPred := resp.Predictions[0].GetStructValue()
	if firstPred == nil {
		return nil, fmt.Errorf("invalid prediction format")
	}

	b64Field := firstPred.Fields["bytesBase64Encoded"]
	if b64Field == nil {
		return nil, fmt.Errorf("bytesBase64Encoded not found in prediction")
	}

	b64Str := b64Field.GetStringValue()
	return base64.StdEncoding.DecodeString(b64Str)
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

	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			return string(text), nil
		}
	}
	return "", nil
}

// Complete generates a completion for the given prompt.
func (c *GeminiClient) Complete(ctx context.Context, prompt string) (string, error) {
	return c.Chat(ctx, prompt)
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
