package client

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// CloudflareClient wraps the S3 client for Cloudflare R2.
type CloudflareClient struct {
	s3Client *s3.Client
	bucket   string
	cdnURL   string
}

// NewCloudflareClient creates a new Cloudflare R2 client.
func NewCloudflareClient(ctx context.Context, accessKeyID, secretKey, endpoint, bucketName, cdnURL string) (*CloudflareClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretKey, "")),
		config.WithRegion("auto"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	return &CloudflareClient{
		s3Client: s3Client,
		bucket:   bucketName,
		cdnURL:   cdnURL,
	}, nil
}

// UploadR2Object uploads an object to R2 and returns the public URL.
func (c *CloudflareClient) UploadR2Object(ctx context.Context, key string, data io.Reader, contentType string) (string, error) {
	// PutObject API
	_, err := c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        data,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to R2: %w", err)
	}

	// Return the public URL
	return fmt.Sprintf("%s/%s", c.cdnURL, key), nil
}

// GetR2ObjectURL returns the public URL for a given key.
func (c *CloudflareClient) GetR2ObjectURL(key string) string {
	return fmt.Sprintf("%s/%s", c.cdnURL, key)
}
