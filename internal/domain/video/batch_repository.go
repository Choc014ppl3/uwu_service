package video

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/pkg/errors"
	"github.com/windfall/uwu_service/pkg/response"
)

// Constants
const processingBatchTTL = 3 * time.Hour
const completedBatchTTL = 10 * time.Minute

// Batch processes:
const (
	PROCESS_UPLOAD_VIDEO        = "upload_video"
	PROCESS_UPLOAD_THUMBNAIL    = "upload_thumbnail"
	PROCESS_GENERATE_TRANSCRIPT = "generate_transcript"
	PROCESS_GENERATE_DETAILS    = "generate_details"
	PROCESS_SAVE_VIDEO          = "save_video"
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
		PROCESS_UPLOAD_VIDEO,
		PROCESS_UPLOAD_THUMBNAIL,
		PROCESS_GENERATE_TRANSCRIPT,
		PROCESS_GENERATE_DETAILS,
		PROCESS_SAVE_VIDEO,
	}
}

// BatchRepository interface
type BatchRepository interface {
	GetBatch(ctx context.Context, batchID string) (*response.MetaProcessing, *errors.AppError)
	CreateBatch(ctx context.Context, batchID string) (*response.MetaProcessing, *errors.AppError)
	UpdateJob(ctx context.Context, batchID, jobName, status, jobErr string) error
	SetBatchResult(ctx context.Context, batchID string, result json.RawMessage) error
}

// BatchRepository manages batch + job state in Redis
type batchRepository struct {
	redis *client.RedisClient
	log   *slog.Logger
}

// NewBatchRepository creates a new batch repository
func NewBatchRepository(redis *client.RedisClient, log *slog.Logger) BatchRepository {
	return &batchRepository{
		redis: redis,
		log:   log,
	}
}

// GetBatch returns the full batch status including all jobs.
func (r *batchRepository) GetBatch(ctx context.Context, batchID string) (*response.MetaProcessing, *errors.AppError) {
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
	dateCreated := batchFields["date_created"]
	dateUpdated := batchFields["date_updated"]

	batch := &response.MetaProcessing{
		BatchID:       batchID,
		Status:        batchFields["status"],
		TotalJobs:     totalJobs,
		CompletedJobs: completedJobs,
		DateCreated:   &dateCreated,
		DateUpdated:   &dateUpdated,
	}

	jobsKey := fmt.Sprintf("batch:%s:jobs", batchID)
	jobFields, err := r.redis.HGetAll(ctx, jobsKey)
	if err != nil {
		return nil, errors.NotFoundWrap("failed to get jobs", err)
	}

	processNames := GetProcessNames()
	if namesRaw, ok := batchFields["job_names"]; ok && namesRaw != "" {
		var customNames []string
		if err := json.Unmarshal([]byte(namesRaw), &customNames); err == nil && len(customNames) > 0 {
			processNames = customNames
		}
	}

	for _, name := range processNames {
		raw, ok := jobFields[name]
		if !ok {
			batch.BatchJobs = append(batch.BatchJobs, response.BatchJob{Name: name, Status: BATCH_UNKNOWN})
			continue
		}

		var job response.BatchJob
		if err := json.Unmarshal([]byte(raw), &job); err != nil {
			batch.BatchJobs = append(batch.BatchJobs, response.BatchJob{Name: name, Status: BATCH_UNKNOWN})
			continue
		}

		batch.BatchJobs = append(batch.BatchJobs, job)
	}

	return batch, nil
}

// CreateBatch initializes a batch and its jobs in Redis.
func (r *batchRepository) CreateBatch(ctx context.Context, batchID string) (*response.MetaProcessing, *errors.AppError) {
	now := time.Now().UTC().Format(time.RFC3339)
	processNames := GetProcessNames()
	totalJobs := len(processNames)
	batchKey := fmt.Sprintf("batch:%s", batchID)

	if err := r.redis.HSet(ctx, batchKey,
		"status", BATCH_PENDING,
		"total_jobs", strconv.Itoa(totalJobs),
		"completed_jobs", "0",
		"date_created", now,
		"date_updated", now,
	); err != nil {
		r.log.Error("Failed to create video batch", "batch_id", batchID, "error", err)
		return nil, errors.Internal("failed to create video batch")
	}

	namesJSON, _ := json.Marshal(processNames)
	_ = r.redis.HSet(ctx, batchKey, "job_names", string(namesJSON))

	jobsKey := fmt.Sprintf("batch:%s:jobs", batchID)
	for _, name := range processNames {
		jobJSON, _ := json.Marshal(response.BatchJob{Name: name, Status: BATCH_PENDING})
		if err := r.redis.HSet(ctx, jobsKey, name, string(jobJSON)); err != nil {
			r.log.Error("Failed to create video batch job", "batch_id", batchID, "job_name", name, "error", err)
			return nil, errors.Internal("failed to create video batch job")
		}
	}

	_ = r.redis.SetExpiry(ctx, batchKey, processingBatchTTL)
	_ = r.redis.SetExpiry(ctx, jobsKey, processingBatchTTL)

	return &response.MetaProcessing{
		BatchID:       batchID,
		Status:        BATCH_PENDING,
		TotalJobs:     totalJobs,
		CompletedJobs: 0,
		BatchJobs: []response.BatchJob{
			{
				Name:   PROCESS_UPLOAD_VIDEO,
				Status: BATCH_PENDING,
			},
			{
				Name:   PROCESS_UPLOAD_THUMBNAIL,
				Status: BATCH_PENDING,
			},
			{
				Name:   PROCESS_GENERATE_TRANSCRIPT,
				Status: BATCH_PENDING,
			},
			{
				Name:   PROCESS_GENERATE_DETAILS,
				Status: BATCH_PENDING,
			},
			{
				Name:   PROCESS_SAVE_VIDEO,
				Status: BATCH_PENDING,
			},
		},
		DateCreated: &now,
		DateUpdated: &now,
	}, nil
}

// UpdateJob updates a single job within the batch and recalculates batch state.
func (r *batchRepository) UpdateJob(ctx context.Context, batchID, jobName, status, jobErr string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	job := response.BatchJob{
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
		r.log.Error("Failed to update video job", "batch_id", batchID, "job_name", jobName, "error", err)
		return err
	}

	fields, err := r.redis.HGetAll(ctx, jobsKey)
	if err != nil {
		return err
	}

	processNames := GetProcessNames()
	batchKey := fmt.Sprintf("batch:%s", batchID)
	if batchMeta, err := r.redis.HGetAll(ctx, batchKey); err == nil {
		if namesRaw, ok := batchMeta["job_names"]; ok && namesRaw != "" {
			var customNames []string
			if err := json.Unmarshal([]byte(namesRaw), &customNames); err == nil && len(customNames) > 0 {
				processNames = customNames
			}
		}
	}

	completed := 0
	hasFailed := false
	for _, raw := range fields {
		var current response.BatchJob
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
	case completed == len(processNames):
		batchStatus = BATCH_COMPLETED
	}

	if err := r.redis.HSet(ctx, batchKey,
		"status", batchStatus,
		"completed_jobs", strconv.Itoa(completed),
		"date_updated", now,
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
		r.log.Error("Failed to set video batch result", "batch_id", batchID, "error", err)
		return err
	}
	return nil
}
