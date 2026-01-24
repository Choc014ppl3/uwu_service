package service

import (
	"context"
	"time"

	"github.com/windfall/uwu_service/internal/client"
	"github.com/windfall/uwu_service/internal/errors"
)

// Example represents an example entity.
type Example struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ExampleService provides example-related functionality.
type ExampleService struct {
	storageClient *client.StorageClient
	pubsubClient  *client.PubSubClient
}

// NewExampleService creates a new example service.
func NewExampleService(
	storageClient *client.StorageClient,
	pubsubClient *client.PubSubClient,
) *ExampleService {
	return &ExampleService{
		storageClient: storageClient,
		pubsubClient:  pubsubClient,
	}
}

// GetExample retrieves an example by ID.
func (s *ExampleService) GetExample(ctx context.Context, id string) (*Example, error) {
	if id == "" {
		return nil, errors.Validation("id is required")
	}

	// In a real application, this would fetch from a database
	// For now, return a mock example
	return &Example{
		ID:          id,
		Name:        "Example Item",
		Description: "This is an example item",
		CreatedAt:   time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now(),
	}, nil
}

// CreateExample creates a new example.
func (s *ExampleService) CreateExample(ctx context.Context, name, description string) (*Example, error) {
	if name == "" {
		return nil, errors.Validation("name is required")
	}

	example := &Example{
		ID:          generateID(),
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Publish event to Pub/Sub if configured
	if s.pubsubClient != nil {
		if err := s.pubsubClient.Publish(ctx, map[string]interface{}{
			"event":   "example.created",
			"payload": example,
		}); err != nil {
			// Log error but don't fail the request
			// In production, you might want to handle this differently
		}
	}

	return example, nil
}

// UpdateExample updates an existing example.
func (s *ExampleService) UpdateExample(ctx context.Context, id, name, description string) (*Example, error) {
	if id == "" {
		return nil, errors.Validation("id is required")
	}

	// In a real application, this would update in a database
	example := &Example{
		ID:          id,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now(),
	}

	// Publish event
	if s.pubsubClient != nil {
		s.pubsubClient.Publish(ctx, map[string]interface{}{
			"event":   "example.updated",
			"payload": example,
		})
	}

	return example, nil
}

// DeleteExample deletes an example.
func (s *ExampleService) DeleteExample(ctx context.Context, id string) error {
	if id == "" {
		return errors.Validation("id is required")
	}

	// In a real application, this would delete from a database

	// Publish event
	if s.pubsubClient != nil {
		s.pubsubClient.Publish(ctx, map[string]interface{}{
			"event": "example.deleted",
			"payload": map[string]string{
				"id": id,
			},
		})
	}

	return nil
}

// UploadFile uploads a file to cloud storage.
func (s *ExampleService) UploadFile(ctx context.Context, filename string, data []byte) (string, error) {
	if s.storageClient == nil {
		return "", errors.New(errors.ErrStorageService, "storage client not configured")
	}

	return s.storageClient.Upload(ctx, filename, data)
}

// DownloadFile downloads a file from cloud storage.
func (s *ExampleService) DownloadFile(ctx context.Context, filename string) ([]byte, error) {
	if s.storageClient == nil {
		return nil, errors.New(errors.ErrStorageService, "storage client not configured")
	}

	return s.storageClient.Download(ctx, filename)
}

func generateID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}
