package video

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/windfall/uwu_service/internal/infra/client"
)

// Constants
const processingBatchTTL = 60 * time.Minute
const completedBatchTTL = 10 * time.Minute

// Batch processes:
const (
	PROCESS_UPLOAD_VIDEO        = "upload_video"
	PROCESS_UPLOAD_THUMBNAIL    = "upload_thumbnail"
	PROCESS_GENERATE_TRANSCRIPT = "generate_transcript"
	PROCESS_GENERATE_DETAILS    = "generate_details"
)

// Batch status:
const (
	BATCH_PENDING    = "pending"
	BATCH_PROCESSING = "processing"
	BATCH_COMPLETED  = "completed"
	BATCH_FAILED     = "failed"
)

func GetProcessNames() []string {
	return []string{
		PROCESS_UPLOAD_VIDEO,
		PROCESS_UPLOAD_THUMBNAIL,
		PROCESS_GENERATE_TRANSCRIPT,
		PROCESS_GENERATE_DETAILS,
	}
}

// BatchRepository interface
type BatchRepository interface {
	CreateBatch(ctx context.Context, batchID, videoID, userID string) error
	UpdateJob(ctx context.Context, batchID, jobName, status, jobErr string) error
	GetBatch(ctx context.Context, batchID string) (*BatchStatus, error)
	SetBatchResult(ctx context.Context, batchID string, result json.RawMessage) error
}

// BatchStatus is the combined status of a batch and all its jobs.
type BatchStatus struct {
	BatchID       string          `json:"batch_id"`
	VideoID       string          `json:"video_id"`
	UserID        string          `json:"user_id"`
	Status        string          `json:"status"` // pending, processing, completed, failed
	TotalJobs     int             `json:"total_jobs"`
	CompletedJobs int             `json:"completed_jobs"`
	Jobs          []JobStatus     `json:"jobs"`
	CreatedAt     string          `json:"created_at"`
	Result        json.RawMessage `json:"result,omitempty"`
}

// JobStatus holds the status of a single job within a batch.
type JobStatus struct {
	Name        string `json:"name"`
	Status      string `json:"status"` // pending, processing, completed, failed
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

// BatchRepository manages batch + job state in Redis
type batchRepository struct {
	redis *client.RedisClient
	log   *slog.Logger
}

// NewBatchRepository creates a new batch repository
func NewBatchRepository(redis *client.RedisClient, log *slog.Logger) *batchRepository {
	return &batchRepository{
		redis: redis,
		log:   log,
	}
}

// CreateBatch initializes a batch and its jobs in Redis.
func (s *batchRepository) CreateBatch(ctx context.Context, batchID, videoID, userID string) error {
	if s.redis == nil {
		return nil // Redis not configured, skip silently
	}

	now := time.Now().UTC().Format(time.RFC3339)
	totalJobs := len(GetProcessNames())

	// Set batch metadata hash
	batchKey := fmt.Sprintf("batch:%s", batchID)
	err := s.redis.HSet(ctx, batchKey,
		"video_id", videoID,
		"user_id", userID,
		"status", BATCH_PENDING,
		"created_at", now,
		"total_jobs", strconv.Itoa(totalJobs),
		"completed_jobs", "0",
	)
	if err != nil {
		s.log.Error("Failed to create batch", "batch_id", batchID, "error", err)
		return err
	}

	// Set initial job statuses (all pending)
	jobsKey := fmt.Sprintf("batch:%s:jobs", batchID)
	for _, name := range GetProcessNames() {
		job := JobStatus{Name: name, Status: BATCH_PENDING}
		jobJSON, _ := json.Marshal(job)
		if err := s.redis.HSet(ctx, jobsKey, name, string(jobJSON)); err != nil {
			s.log.Error("Failed to set job", "batch_id", batchID, "job_name", name, "error", err)
			return err
		}
	}

	// Set TTL on both keys
	_ = s.redis.SetExpiry(ctx, batchKey, processingBatchTTL)
	_ = s.redis.SetExpiry(ctx, jobsKey, processingBatchTTL)

	return nil
}

// UpdateJob updates a single job's status within a batch.
func (s *batchRepository) UpdateJob(ctx context.Context, batchID, jobName, status, jobErr string) error {
	if s.redis == nil {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	job := JobStatus{
		Name:   jobName,
		Status: status,
	}

	switch status {
	case BATCH_PROCESSING:
		job.StartedAt = now
	case BATCH_COMPLETED:
		job.CompletedAt = now
	case BATCH_FAILED:
		job.CompletedAt = now
		job.Error = jobErr
	}

	jobJSON, _ := json.Marshal(job)
	jobsKey := fmt.Sprintf("batch:%s:jobs", batchID)
	if err := s.redis.HSet(ctx, jobsKey, jobName, string(jobJSON)); err != nil {
		s.log.Error("Failed to update job", "batch_id", batchID, "job_name", jobName, "error", err)
		return err
	}

	// Recalculate batch status
	return s.recalculateBatchStatus(ctx, batchID)
}

// recalculateBatchStatus derives batch status from job statuses.
func (s *batchRepository) recalculateBatchStatus(ctx context.Context, batchID string) error {
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
		totalJobs = len(GetProcessNames()) // fallback
	}

	completed := 0
	hasFailed := false
	for _, raw := range fields {
		var job JobStatus
		if err := json.Unmarshal([]byte(raw), &job); err != nil {
			continue
		}
		if job.Status == BATCH_COMPLETED {
			completed++
		}
		if job.Status == BATCH_FAILED {
			hasFailed = true
		}
	}

	batchStatus := BATCH_PROCESSING
	if hasFailed {
		batchStatus = BATCH_FAILED
	} else if completed == totalJobs {
		batchStatus = BATCH_COMPLETED
	}

	_ = s.redis.HSet(ctx, batchKey, "status", batchStatus, "completed_jobs", strconv.Itoa(completed))

	// Shorten TTL for completed/failed batches to free Redis memory
	if batchStatus == BATCH_COMPLETED || batchStatus == BATCH_FAILED {
		jobsKey := fmt.Sprintf("batch:%s:jobs", batchID)
		_ = s.redis.SetExpiry(ctx, batchKey, completedBatchTTL)
		_ = s.redis.SetExpiry(ctx, jobsKey, completedBatchTTL)
	}

	return nil
}

// GetBatch returns the full batch status including all jobs.
func (s *batchRepository) GetBatch(ctx context.Context, batchID string) (*BatchStatus, error) {
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
		UserID:        batchFields["user_id"],
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
	for _, name := range GetProcessNames() {
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

// SetBatchResult stores result data in batch metadata.
func (s *batchRepository) SetBatchResult(ctx context.Context, batchID string, result json.RawMessage) error {
	if s.redis == nil {
		return nil
	}
	batchKey := fmt.Sprintf("batch:%s", batchID)
	return s.redis.HSet(ctx, batchKey, "result", string(result))
}
