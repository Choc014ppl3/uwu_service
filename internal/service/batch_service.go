package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog"

	"github.com/windfall/uwu_service/internal/client"
)

const batchTTL = 24 * time.Hour

// JobStatus holds the status of a single job within a batch.
type JobStatus struct {
	Name        string `json:"name"`
	Status      string `json:"status"` // pending, processing, completed, failed
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

// BatchStatus is the combined status of a batch and all its jobs.
type BatchStatus struct {
	BatchID       string          `json:"batch_id"`
	VideoID       string          `json:"video_id"`
	Status        string          `json:"status"` // processing, completed, failed
	TotalJobs     int             `json:"total_jobs"`
	CompletedJobs int             `json:"completed_jobs"`
	Jobs          []JobStatus     `json:"jobs"`
	CreatedAt     string          `json:"created_at"`
	Result        json.RawMessage `json:"result,omitempty"`
}

// BatchService manages batch + job state in Redis.
type BatchService struct {
	redis *client.RedisClient
	log   zerolog.Logger
}

// NewBatchService creates a new BatchService.
func NewBatchService(redis *client.RedisClient, log zerolog.Logger) *BatchService {
	return &BatchService{
		redis: redis,
		log:   log,
	}
}

// jobNames defines the ordered list of jobs in a video processing batch.
var jobNames = []string{"upload", "transcript", "quiz"}

// CreateBatch initializes a batch and its jobs in Redis.
func (s *BatchService) CreateBatch(ctx context.Context, batchID, videoID, userID string) error {
	if s.redis == nil {
		return nil // Redis not configured, skip silently
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Set batch metadata hash
	batchKey := fmt.Sprintf("batch:%s", batchID)
	err := s.redis.HSet(ctx, batchKey,
		"video_id", videoID,
		"user_id", userID,
		"status", "processing",
		"created_at", now,
		"total_jobs", strconv.Itoa(len(jobNames)),
		"completed_jobs", "0",
	)
	if err != nil {
		return fmt.Errorf("failed to create batch: %w", err)
	}

	// Set initial job statuses (all pending, except upload which starts immediately)
	jobsKey := fmt.Sprintf("batch:%s:jobs", batchID)
	for _, name := range jobNames {
		job := JobStatus{Name: name, Status: "pending"}
		if name == "upload" {
			job.Status = "processing"
			job.StartedAt = now
		}
		jobJSON, _ := json.Marshal(job)
		if err := s.redis.HSet(ctx, jobsKey, name, string(jobJSON)); err != nil {
			return fmt.Errorf("failed to set job %s: %w", name, err)
		}
	}

	// Set TTL on both keys
	_ = s.redis.SetExpiry(ctx, batchKey, batchTTL)
	_ = s.redis.SetExpiry(ctx, jobsKey, batchTTL)

	s.log.Info().
		Str("batch_id", batchID).
		Str("video_id", videoID).
		Int("total_jobs", len(jobNames)).
		Msg("Batch created")

	return nil
}

// UpdateJob updates a single job's status within a batch.
func (s *BatchService) UpdateJob(ctx context.Context, batchID, jobName, status, jobErr string) error {
	if s.redis == nil {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	job := JobStatus{
		Name:   jobName,
		Status: status,
	}

	switch status {
	case "processing":
		job.StartedAt = now
	case "completed":
		job.CompletedAt = now
	case "failed":
		job.CompletedAt = now
		job.Error = jobErr
	}

	jobJSON, _ := json.Marshal(job)
	jobsKey := fmt.Sprintf("batch:%s:jobs", batchID)
	if err := s.redis.HSet(ctx, jobsKey, jobName, string(jobJSON)); err != nil {
		return fmt.Errorf("failed to update job %s: %w", jobName, err)
	}

	// Recalculate batch status
	return s.recalculateBatchStatus(ctx, batchID)
}

// recalculateBatchStatus derives batch status from job statuses.
func (s *BatchService) recalculateBatchStatus(ctx context.Context, batchID string) error {
	jobsKey := fmt.Sprintf("batch:%s:jobs", batchID)
	fields, err := s.redis.HGetAll(ctx, jobsKey)
	if err != nil {
		return err
	}

	// Read total_jobs from batch metadata
	batchKey := fmt.Sprintf("batch:%s", batchID)
	batchMeta, _ := s.redis.HGetAll(ctx, batchKey)
	totalJobs, _ := strconv.Atoi(batchMeta["total_jobs"])
	if totalJobs == 0 {
		totalJobs = len(jobNames) // fallback
	}

	completed := 0
	hasFailed := false
	for _, raw := range fields {
		var job JobStatus
		if err := json.Unmarshal([]byte(raw), &job); err != nil {
			continue
		}
		if job.Status == "completed" {
			completed++
		}
		if job.Status == "failed" {
			hasFailed = true
		}
	}

	batchStatus := "processing"
	if hasFailed {
		batchStatus = "failed"
	} else if completed == totalJobs {
		batchStatus = "completed"
	}

	_ = s.redis.HSet(ctx, batchKey, "status", batchStatus, "completed_jobs", strconv.Itoa(completed))
	return nil
}

// GetBatch returns the full batch status including all jobs.
func (s *BatchService) GetBatch(ctx context.Context, batchID string) (*BatchStatus, error) {
	if s.redis == nil {
		return nil, fmt.Errorf("redis not configured")
	}

	batchKey := fmt.Sprintf("batch:%s", batchID)
	batchFields, err := s.redis.HGetAll(ctx, batchKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch: %w", err)
	}

	if len(batchFields) == 0 {
		return nil, nil // batch not found
	}

	totalJobs, _ := strconv.Atoi(batchFields["total_jobs"])
	completedJobs, _ := strconv.Atoi(batchFields["completed_jobs"])

	batch := &BatchStatus{
		BatchID:       batchID,
		VideoID:       batchFields["video_id"],
		Status:        batchFields["status"],
		TotalJobs:     totalJobs,
		CompletedJobs: completedJobs,
		CreatedAt:     batchFields["created_at"],
	}

	// Read job statuses
	jobsKey := fmt.Sprintf("batch:%s:jobs", batchID)
	jobFields, err := s.redis.HGetAll(ctx, jobsKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs: %w", err)
	}

	// Maintain order from jobNames
	for _, name := range jobNames {
		raw, ok := jobFields[name]
		if !ok {
			batch.Jobs = append(batch.Jobs, JobStatus{Name: name, Status: "unknown"})
			continue
		}
		var job JobStatus
		if err := json.Unmarshal([]byte(raw), &job); err != nil {
			batch.Jobs = append(batch.Jobs, JobStatus{Name: name, Status: "unknown"})
			continue
		}
		batch.Jobs = append(batch.Jobs, job)
	}

	return batch, nil
}

// CreateBatchWithJobs initializes a batch with a custom list of job names.
func (s *BatchService) CreateBatchWithJobs(ctx context.Context, batchID, refID string, customJobNames []string) error {
	if s.redis == nil {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	batchKey := fmt.Sprintf("batch:%s", batchID)
	err := s.redis.HSet(ctx, batchKey,
		"video_id", refID,
		"status", "processing",
		"created_at", now,
		"total_jobs", strconv.Itoa(len(customJobNames)),
		"completed_jobs", "0",
	)
	if err != nil {
		return fmt.Errorf("failed to create batch: %w", err)
	}

	jobsKey := fmt.Sprintf("batch:%s:jobs", batchID)
	for _, name := range customJobNames {
		job := JobStatus{Name: name, Status: "pending"}
		jobJSON, _ := json.Marshal(job)
		if err := s.redis.HSet(ctx, jobsKey, name, string(jobJSON)); err != nil {
			return fmt.Errorf("failed to set job %s: %w", name, err)
		}
	}

	// Store job names list for recalculation
	namesJSON, _ := json.Marshal(customJobNames)
	_ = s.redis.HSet(ctx, batchKey, "job_names", string(namesJSON))

	_ = s.redis.SetExpiry(ctx, batchKey, batchTTL)
	_ = s.redis.SetExpiry(ctx, jobsKey, batchTTL)

	s.log.Info().
		Str("batch_id", batchID).
		Str("ref_id", refID).
		Int("total_jobs", len(customJobNames)).
		Msg("Custom batch created")

	return nil
}

// GetBatchWithJobs returns batch status using a dynamic job name list from Redis.
func (s *BatchService) GetBatchWithJobs(ctx context.Context, batchID string) (*BatchStatus, error) {
	if s.redis == nil {
		return nil, fmt.Errorf("redis not configured")
	}

	batchKey := fmt.Sprintf("batch:%s", batchID)
	batchFields, err := s.redis.HGetAll(ctx, batchKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch: %w", err)
	}

	if len(batchFields) == 0 {
		return nil, nil
	}

	totalJobs, _ := strconv.Atoi(batchFields["total_jobs"])
	completedJobs, _ := strconv.Atoi(batchFields["completed_jobs"])

	batch := &BatchStatus{
		BatchID:       batchID,
		VideoID:       batchFields["video_id"],
		Status:        batchFields["status"],
		TotalJobs:     totalJobs,
		CompletedJobs: completedJobs,
		CreatedAt:     batchFields["created_at"],
	}

	// Read custom job names
	var customNames []string
	if namesRaw, ok := batchFields["job_names"]; ok {
		_ = json.Unmarshal([]byte(namesRaw), &customNames)
	}
	if len(customNames) == 0 {
		customNames = jobNames // fallback to default
	}

	jobsKey := fmt.Sprintf("batch:%s:jobs", batchID)
	jobFields, err := s.redis.HGetAll(ctx, jobsKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get jobs: %w", err)
	}

	for _, name := range customNames {
		raw, ok := jobFields[name]
		if !ok {
			batch.Jobs = append(batch.Jobs, JobStatus{Name: name, Status: "unknown"})
			continue
		}
		var job JobStatus
		if err := json.Unmarshal([]byte(raw), &job); err != nil {
			batch.Jobs = append(batch.Jobs, JobStatus{Name: name, Status: "unknown"})
			continue
		}
		batch.Jobs = append(batch.Jobs, job)
	}

	// Include result data if present
	if resultRaw, ok := batchFields["result"]; ok && resultRaw != "" {
		batch.Result = json.RawMessage(resultRaw)
	}

	return batch, nil
}

// SetBatchResult stores result data in batch metadata.
func (s *BatchService) SetBatchResult(ctx context.Context, batchID string, result json.RawMessage) error {
	if s.redis == nil {
		return nil
	}
	batchKey := fmt.Sprintf("batch:%s", batchID)
	return s.redis.HSet(ctx, batchKey, "result", string(result))
}
