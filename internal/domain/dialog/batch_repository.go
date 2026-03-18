package dialog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/pkg/errors"
)

// Constants
const processingBatchTTL = 3 * time.Hour
const completedBatchTTL = 10 * time.Minute

// Batch processes:
const (
	PROCESS_GENERATE_DIALOG = "generate_dialog"
)

// Batch status:
const (
	BATCH_PENDING    = "pending"
	BATCH_PROCESSING = "processing"
	BATCH_COMPLETED  = "completed"
	BATCH_FAILED     = "failed"
	BATCH_UNKNOWN    = "unknown"
)

func GetProcessNames() []string {
	return []string{
		PROCESS_GENERATE_DIALOG,
	}
}

// BatchRepository interface
type BatchRepository interface {
	GetBatch(ctx context.Context, batchID string) (*BatchResult, *errors.AppError)
	CreateBatch(ctx context.Context, batchID string) error
	UpdateJob(ctx context.Context, batchID, jobName, status, jobErr string) error
	SetBatchResult(ctx context.Context, batchID string, result json.RawMessage) error
}

// BatchResult is the combined status of a batch and all its jobs.
type BatchResult struct {
	BatchID       string          `json:"batch_id"`
	Status        string          `json:"status"`
	TotalJobs     int             `json:"total_jobs"`
	CompletedJobs int             `json:"completed_jobs"`
	Jobs          []BatchJob      `json:"jobs"`
	CreatedAt     string          `json:"created_at"`
	Result        json.RawMessage `json:"result,omitempty"`
}

// BatchJob holds the status of a single job within a batch.
type BatchJob struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

type batchRepository struct {
	redis *client.RedisClient
	log   *slog.Logger
}

// NewBatchRepository creates a new dialog batch repository.
func NewBatchRepository(redis *client.RedisClient, log *slog.Logger) BatchRepository {
	return &batchRepository{
		redis: redis,
		log:   log,
	}
}

// GetBatch returns the full batch status including all jobs.
func (r *batchRepository) GetBatch(ctx context.Context, batchID string) (*BatchResult, *errors.AppError) {
	batchKey := fmt.Sprintf("batch:%s", batchID)
	batchFields, err := r.redis.HGetAll(ctx, batchKey)
	if err != nil {
		return nil, errors.NotFoundWrap("failed to get batch", err)
	}

	if len(batchFields) == 0 {
		return nil, nil
	}

	totalJobs, _ := strconv.Atoi(batchFields["total_jobs"])
	completedJobs, _ := strconv.Atoi(batchFields["completed_jobs"])

	batch := &BatchResult{
		BatchID:       batchID,
		Status:        batchFields["status"],
		TotalJobs:     totalJobs,
		CompletedJobs: completedJobs,
		CreatedAt:     batchFields["created_at"],
		Result:        json.RawMessage(batchFields["result"]),
	}

	jobsKey := fmt.Sprintf("batch:%s:jobs", batchID)
	jobFields, err := r.redis.HGetAll(ctx, jobsKey)
	if err != nil {
		return nil, errors.NotFoundWrap("failed to get jobs", err)
	}

	for _, name := range GetProcessNames() {
		raw, ok := jobFields[name]
		if !ok {
			batch.Jobs = append(batch.Jobs, BatchJob{Name: name, Status: BATCH_UNKNOWN})
			continue
		}

		var job BatchJob
		if err := json.Unmarshal([]byte(raw), &job); err != nil {
			batch.Jobs = append(batch.Jobs, BatchJob{Name: name, Status: BATCH_UNKNOWN})
			continue
		}

		batch.Jobs = append(batch.Jobs, job)
	}

	return batch, nil
}

// CreateBatch initializes a batch and its jobs in Redis.
func (r *batchRepository) CreateBatch(ctx context.Context, batchID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	totalJobs := len(GetProcessNames())
	batchKey := fmt.Sprintf("batch:%s", batchID)

	if err := r.redis.HSet(ctx, batchKey,
		"status", BATCH_PENDING,
		"created_at", now,
		"total_jobs", strconv.Itoa(totalJobs),
		"completed_jobs", "0",
	); err != nil {
		r.log.Error("Failed to create dialog batch", "batch_id", batchID, "error", err)
		return err
	}

	jobsKey := fmt.Sprintf("batch:%s:jobs", batchID)
	for _, name := range GetProcessNames() {
		jobJSON, _ := json.Marshal(BatchJob{Name: name, Status: BATCH_PENDING})
		if err := r.redis.HSet(ctx, jobsKey, name, string(jobJSON)); err != nil {
			r.log.Error("Failed to create dialog batch job", "batch_id", batchID, "job_name", name, "error", err)
			return err
		}
	}

	_ = r.redis.SetExpiry(ctx, batchKey, processingBatchTTL)
	_ = r.redis.SetExpiry(ctx, jobsKey, processingBatchTTL)

	return nil
}

// UpdateJob updates a single job within the batch and recalculates batch state.
func (r *batchRepository) UpdateJob(ctx context.Context, batchID, jobName, status, jobErr string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	job := BatchJob{
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
	if err := r.redis.HSet(ctx, jobsKey, jobName, string(jobJSON)); err != nil {
		r.log.Error("Failed to update dialog job", "batch_id", batchID, "job_name", jobName, "error", err)
		return err
	}

	fields, err := r.redis.HGetAll(ctx, jobsKey)
	if err != nil {
		return err
	}

	completed := 0
	hasFailed := false
	for _, raw := range fields {
		var current BatchJob
		if err := json.Unmarshal([]byte(raw), &current); err != nil {
			continue
		}
		if current.Status == BATCH_COMPLETED {
			completed++
		}
		if current.Status == BATCH_FAILED {
			hasFailed = true
		}
	}

	batchStatus := BATCH_PROCESSING
	switch {
	case hasFailed:
		batchStatus = BATCH_FAILED
	case completed == len(GetProcessNames()):
		batchStatus = BATCH_COMPLETED
	}

	batchKey := fmt.Sprintf("batch:%s", batchID)
	if err := r.redis.HSet(ctx, batchKey,
		"status", batchStatus,
		"completed_jobs", strconv.Itoa(completed),
	); err != nil {
		return err
	}

	if batchStatus == BATCH_COMPLETED || batchStatus == BATCH_FAILED {
		_ = r.redis.SetExpiry(ctx, batchKey, completedBatchTTL)
		_ = r.redis.SetExpiry(ctx, jobsKey, completedBatchTTL)
	}

	return nil
}

// SetBatchResult stores the final serialized result in the batch hash.
func (r *batchRepository) SetBatchResult(ctx context.Context, batchID string, result json.RawMessage) error {
	batchKey := fmt.Sprintf("batch:%s", batchID)
	if err := r.redis.HSet(ctx, batchKey, "result", string(result)); err != nil {
		r.log.Error("Failed to set dialog batch result", "batch_id", batchID, "error", err)
		return err
	}
	return nil
}
