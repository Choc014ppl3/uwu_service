package dialog

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/pkg/errors"
	"github.com/windfall/uwu_service/pkg/response"
)

// DialogService handles dialog operations
type DialogService struct {
	dialogRepo DialogRepository
	aiRepo     AIRepository
	batchRepo  BatchRepository
}

// DialogDetailsResponse is returned for dialog details
type DialogDetailsResponse struct {
	DialogID string        `json:"dialog_id"`
	UserID   string        `json:"user_id"`
	Status   string        `json:"status"`
	Progress *BatchResult  `json:"progress"`
	Data     *LearningItem `json:"data"`
}

// ListDialogContentsResponse is returned when listing dialog contents.
type ListDialogContentsResponse struct {
	Data []*LearningItem `json:"data"`
	Meta *response.Meta  `json:"meta"`
}

// NewDialogService creates a new DialogService.
func NewDialogService(dialogRepo DialogRepository, aiRepo AIRepository, batchRepo BatchRepository) *DialogService {
	return &DialogService{
		dialogRepo: dialogRepo,
		aiRepo:     aiRepo,
		batchRepo:  batchRepo,
	}
}

// List Dialog Contents
func (s *DialogService) ListDialogContents(ctx context.Context, input ListDialogContentsInput) (*ListDialogContentsResponse, *errors.AppError) {
	// 1. Get dialog contents from database
	dialogs, total, err := s.dialogRepo.ListDialogs(ctx, input.Limit, input.Offset)
	if err != nil {
		return nil, err
	}

	// 2. Calculate total pages
	totalPages := 0
	if input.PageSize > 0 {
		totalPages = (total + input.PageSize - 1) / input.PageSize
	}

	meta := &response.Meta{
		Page:       input.Page,
		PerPage:    input.PageSize,
		Total:      total,
		TotalPages: totalPages,
	}

	return &ListDialogContentsResponse{
		Data: dialogs,
		Meta: meta,
	}, nil
}

// Get Dialog Details
func (s *DialogService) GetDialogDetails(ctx context.Context, dialogID, userID string) (*DialogDetailsResponse, *errors.AppError) {
	// Get dialog from database
	learningItem, err := s.dialogRepo.GetDialog(ctx, dialogID)
	if err != nil {
		return nil, err
	}

	var batch *BatchResult
	if s.batchRepo != nil {
		batch, _ = s.batchRepo.GetBatch(ctx, dialogID)
	}

	if learningItem != nil {
		status := BATCH_COMPLETED

		var metadata DialogMetadata
		if len(learningItem.Metadata) > 0 {
			_ = json.Unmarshal(learningItem.Metadata, &metadata)
			if metadata.Status != "" {
				status = metadata.Status
			}
		}

		return &DialogDetailsResponse{
			DialogID: dialogID,
			UserID:   userID,
			Status:   status,
			Progress: batch,
			Data:     learningItem,
		}, nil
	}

	if batch != nil {
		var dialogData *LearningItem
		_ = json.Unmarshal(batch.Result, &dialogData)

		return &DialogDetailsResponse{
			DialogID: dialogID,
			UserID:   userID,
			Status:   batch.Status,
			Progress: batch,
			Data:     dialogData,
		}, nil
	}

	return nil, errors.NotFound("dialog not found")
}

// Create Dialog Content
func (s *DialogService) CreateDialogContent(ctx context.Context, input GenerateDialogPayload) (*DialogDetailsResponse, *errors.AppError) {
	if s.batchRepo != nil {
		_ = s.batchRepo.CreateBatch(ctx, input.DialogID)
	}

	metadataJSON, _ := json.Marshal(DialogMetadata{
		UserID: input.UserID,
		Status: BATCH_PENDING,
	})

	learningItem := &LearningItem{
		ID:        uuid.Must(uuid.Parse(input.DialogID)),
		Content:   input.Topic,
		Language:  input.Language,
		Level:     nil,
		Details:   json.RawMessage("{}"),
		Tags:      json.RawMessage("[]"),
		Metadata:  metadataJSON,
		CreatedBy: input.UserID,
		IsActive:  false,
	}

	if err := s.dialogRepo.CreateDialog(ctx, learningItem); err != nil {
		return nil, errors.InternalWrap("failed to create dialog content", err)
	}

	return &DialogDetailsResponse{
		DialogID: input.DialogID,
		UserID:   input.UserID,
		Status:   BATCH_PENDING,
	}, nil
}

// ProcessGenerateDialog handles the background generation flow for dialogs.
func (s *DialogService) ProcessGenerateDialog(ctx context.Context, payload GenerateDialogPayload) {
	if s.aiRepo == nil || s.batchRepo == nil {
		return
	}

	if err := s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_DIALOG, BATCH_PROCESSING, ""); err != nil {
		return
	}

	details, err := s.aiRepo.GenerateDialog(ctx, payload)
	if err != nil {
		_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_DIALOG, BATCH_FAILED, err.GetMessage())
		return
	}

	detailsJSON, _ := json.Marshal(details)
	tagsJSON, _ := json.Marshal(details.Tags)
	metadataJSON, _ := json.Marshal(DialogMetadata{
		UserID: payload.UserID,
		Status: BATCH_COMPLETED,
	})

	learningItem := &LearningItem{
		ID:        uuid.Must(uuid.Parse(payload.DialogID)),
		Content:   details.Topic,
		Language:  details.Language,
		Level:     &details.Level,
		Details:   detailsJSON,
		Tags:      tagsJSON,
		Metadata:  metadataJSON,
		CreatedBy: payload.UserID,
		IsActive:  true,
	}

	if err := s.dialogRepo.UpdateDialog(ctx, learningItem); err != nil {
		_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_DIALOG, BATCH_FAILED, err.GetMessage())
		return
	}

	resultJSON, _ := json.Marshal(learningItem)
	_ = s.batchRepo.SetBatchResult(ctx, payload.DialogID, resultJSON)
	_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_DIALOG, BATCH_COMPLETED, "")
}
