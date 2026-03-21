package dialog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/pkg/errors"
	"github.com/windfall/uwu_service/pkg/response"
)

// DialogService handles dialog operations
type DialogService struct {
	dialogRepo DialogRepository
	aiRepo     AIRepository
	imageRepo  ImageRepository
	audioRepo  AudioRepository
	fileRepo   FileRepository
	batchRepo  BatchRepository
}

// DialogDetailsResponse is returned for dialog details
type DialogDetailsResponse struct {
	Data *LearningItem            `json:"data"`
	Meta *response.MetaProcessing `json:"meta"`
}

// ListDialogContentsResponse is returned when listing dialog contents.
type ListDialogContentsResponse struct {
	Data []*LearningItem          `json:"data"`
	Meta *response.MetaPagination `json:"meta"`
}

// ToggleSavedResponse is returned after toggling saved state.
type ToggleSavedResponse struct {
	ActionID string `json:"action_id"`
	DialogID string `json:"dialog_id"`
	UserID   string `json:"user_id"`
	Saved    bool   `json:"saved"`
}

// StartActionResponse is returned after starting a speech or chat action.
type StartActionResponse struct {
	ActionID string `json:"action_id"`
	DialogID string `json:"dialog_id"`
	UserID   string `json:"user_id"`
}

// GenerateImageResponse is returned after generating an image.
type GenerateImageResponse struct {
	ImageURL string `json:"image_url"`
}

// NewDialogService creates a new DialogService.
func NewDialogService(
	dialogRepo DialogRepository,
	aiRepo AIRepository,
	imageRepo ImageRepository,
	audioRepo AudioRepository,
	fileRepo FileRepository,
	batchRepo BatchRepository,
) *DialogService {
	return &DialogService{
		dialogRepo: dialogRepo,
		aiRepo:     aiRepo,
		imageRepo:  imageRepo,
		audioRepo:  audioRepo,
		fileRepo:   fileRepo,
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

	meta := &response.MetaPagination{
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

	var metadata response.MetaProcessing
	if len(learningItem.Metadata) > 0 {
		_ = json.Unmarshal(learningItem.Metadata, &metadata)
		if metadata.Status == BATCH_COMPLETED {
			// Response complete batch processing item from database
			return &DialogDetailsResponse{
				Data: learningItem,
				Meta: &metadata,
			}, nil
		}
	}

	// Get batch from Redis
	metaProcessing, err := s.batchRepo.GetBatch(ctx, dialogID)
	if err != nil {
		return nil, err
	}

	return &DialogDetailsResponse{
		Data: learningItem,
		Meta: metaProcessing,
	}, nil
}

// Create Dialog Content
func (s *DialogService) CreateDialogContent(ctx context.Context, input GenerateDialogPayload) (*DialogDetailsResponse, *errors.AppError) {
	batchProcessing, err := s.batchRepo.CreateBatch(ctx, input.DialogID)
	if err != nil {
		return nil, err
	}

	metadataJSON, _ := json.Marshal(batchProcessing)
	learningItem := &LearningItem{
		ID:        uuid.Must(uuid.Parse(input.DialogID)),
		Content:   input.Topic,
		Language:  input.Language,
		Level:     input.Level,
		Tags:      json.RawMessage("[]"),
		Details:   json.RawMessage("{}"),
		Metadata:  metadataJSON,
		CreatedBy: input.UserID,
		IsActive:  false,
	}

	if err := s.dialogRepo.CreateDialog(ctx, learningItem); err != nil {
		return nil, errors.InternalWrap("failed to create dialog content", err)
	}

	return &DialogDetailsResponse{
		Data: learningItem,
		Meta: batchProcessing,
	}, nil
}

// Worker: ProcessGenerateDialog handles the background generation flow for dialogs.
func (s *DialogService) ProcessGenerateDialog(ctx context.Context, payload GenerateDialogPayload) {
	_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_DIALOG, BATCH_PROCESSING, "")

	details, err := s.aiRepo.GenerateDialog(ctx, payload)
	if err != nil {
		_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_DIALOG, BATCH_FAILED, err.GetMessage())
		s.failRemainingMediaJobs(ctx, payload.DialogID, "skipped: dialogue generation failed")
		return
	}

	_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_DIALOG, BATCH_COMPLETED, "")

	// Extract data from details
	speechModeMap := details.SpeechMode
	situationText := speechModeMap.Situation
	speechScripts := speechModeMap.Script

	voice := voiceForDialogLanguage(details.Language)

	var imageURL string
	var audioURL string
	var mediaWg sync.WaitGroup
	var mediaMu sync.Mutex
	var scriptsHasError bool
	var scriptsLastErr error
	scriptsStarted := false

	if details.ImagePrompt != "" && s.imageRepo != nil && s.fileRepo != nil {
		mediaWg.Add(1)
		go func() {
			defer mediaWg.Done()
			_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_IMAGE, BATCH_PROCESSING, "")

			imageBytes, err := s.imageRepo.GenerateImage(ctx, details.ImagePrompt)
			if err != nil {
				_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_IMAGE, BATCH_FAILED, err.GetMessage())
				_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_IMAGE, BATCH_FAILED, "skipped: image generation failed")
				return
			}

			_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_IMAGE, BATCH_COMPLETED, "")
			_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_IMAGE, BATCH_PROCESSING, "")

			url, err := s.fileRepo.UploadBytes(ctx, imageBytes, fmt.Sprintf("dialogs/%s/bg_image.png", payload.DialogID), "image/png")
			if err != nil {
				_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_IMAGE, BATCH_FAILED, err.GetMessage())
				return
			}

			imageURL = url
			_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_IMAGE, BATCH_COMPLETED, "")
		}()
	} else {
		_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_IMAGE, BATCH_FAILED, "")
		_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_IMAGE, BATCH_FAILED, "")
	}

	if situationText != "" && s.audioRepo != nil && s.fileRepo != nil {
		mediaWg.Add(1)
		go func() {
			defer mediaWg.Done()
			_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_AUDIO, BATCH_PROCESSING, "")

			audioBytes, err := s.audioRepo.Synthesize(ctx, situationText, voice)
			if err != nil {
				_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_AUDIO, BATCH_FAILED, err.GetMessage())
				_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_AUDIO, BATCH_FAILED, "skipped: audio generation failed")
				return
			}

			_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_AUDIO, BATCH_COMPLETED, "")
			_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_AUDIO, BATCH_PROCESSING, "")

			url, err := s.fileRepo.UploadBytes(ctx, audioBytes, fmt.Sprintf("dialogs/%s/situation_audio.mp3", payload.DialogID), "audio/mpeg")
			if err != nil {
				_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_AUDIO, BATCH_FAILED, err.GetMessage())
				return
			}

			audioURL = url
			_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_AUDIO, BATCH_COMPLETED, "")
		}()
	} else {
		_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_AUDIO, BATCH_FAILED, "")
		_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_AUDIO, BATCH_FAILED, "")
	}

	if len(speechScripts) > 0 && s.audioRepo != nil && s.fileRepo != nil {
		scriptsStarted = true
		_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_AUDIO_SCRIPTS, BATCH_PROCESSING, "")
		_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_AUDIO_SCRIPTS, BATCH_PROCESSING, "")

		for i := range speechScripts {
			speaker := speechScripts[i].Speaker
			text := speechScripts[i].Text
			if !strings.EqualFold(speaker, "AI") || text == "" {
				continue
			}

			mediaWg.Add(1)
			go func(idx int, scriptText string) {
				defer mediaWg.Done()

				audioBytes, err := s.audioRepo.Synthesize(ctx, scriptText, voice)
				if err != nil {
					mediaMu.Lock()
					scriptsHasError = true
					scriptsLastErr = err
					mediaMu.Unlock()
					return
				}

				url, err := s.fileRepo.UploadBytes(ctx, audioBytes, fmt.Sprintf("dialogs/%s/script_%d.mp3", payload.DialogID, idx), "audio/mpeg")
				if err != nil {
					mediaMu.Lock()
					scriptsHasError = true
					scriptsLastErr = err
					mediaMu.Unlock()
					return
				}

				speechScripts[idx].AudioURL = &url
			}(i, text)
		}
	} else {
		_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_AUDIO_SCRIPTS, BATCH_FAILED, "")
		_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_AUDIO_SCRIPTS, BATCH_FAILED, "")
	}

	mediaWg.Wait()

	if scriptsStarted {
		if scriptsHasError {
			errMessage := "failed to generate script audio"
			if scriptsLastErr != nil {
				errMessage = scriptsLastErr.Error()
			}
			_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_AUDIO_SCRIPTS, BATCH_FAILED, errMessage)
			_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_AUDIO_SCRIPTS, BATCH_FAILED, errMessage)
		} else {
			_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_GENERATE_AUDIO_SCRIPTS, BATCH_COMPLETED, "")
			_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_UPLOAD_AUDIO_SCRIPTS, BATCH_COMPLETED, "")
		}
	}

	details.ImageURL = imageURL
	details.AudioURL = audioURL
	
	_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_SAVE_DIALOG, BATCH_PROCESSING, "")
	
	detailsJSON, _ := json.Marshal(details)
	tagsJSON, _ := json.Marshal(details.Tags)

	batch, _ := s.batchRepo.GetBatch(ctx, payload.DialogID)
	if batch != nil {
		batch.Status = BATCH_COMPLETED
		batch.CompletedJobs = batch.TotalJobs
		now := time.Now().UTC().Format(time.RFC3339)
		for i := range batch.BatchJobs {
			if batch.BatchJobs[i].Name == PROCESS_SAVE_DIALOG {
				batch.BatchJobs[i].Status = BATCH_COMPLETED
				batch.BatchJobs[i].CompletedAt = now
			}
		}
	}

	metadataJSON, _ := json.Marshal(batch)
	learningItem := &LearningItem{
		ID:        uuid.Must(uuid.Parse(payload.DialogID)),
		Content:   details.Topic,
		Language:  details.Language,
		Level:     details.Level,
		Tags:      tagsJSON,
		Details:   detailsJSON,
		Metadata:  metadataJSON,
		CreatedBy: payload.UserID,
		IsActive:  true,
	}

	if err := s.dialogRepo.UpdateDialog(ctx, learningItem); err != nil {
		_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_SAVE_DIALOG, BATCH_FAILED, err.GetMessage())
		return
	} else {
		_ = s.batchRepo.UpdateJob(ctx, payload.DialogID, PROCESS_SAVE_DIALOG, BATCH_COMPLETED, "")
	}
}

// ToggleSaved toggles the saved action for a dialog.
func (s *DialogService) ToggleSaved(ctx context.Context, dialogID, userID string) (*ToggleSavedResponse, *errors.AppError) {
	actionID, saved, err := s.dialogRepo.ToggleSaved(ctx, dialogID, userID)
	if err != nil {
		return nil, err
	}

	return &ToggleSavedResponse{
		ActionID: actionID,
		DialogID: dialogID,
		UserID:   userID,
		Saved:    saved,
	}, nil
}

// StartSpeech starts a speech action for a dialog.
func (s *DialogService) StartSpeech(ctx context.Context, dialogID, userID string) (*StartActionResponse, *errors.AppError) {
	actionID, err := s.dialogRepo.StartSpeech(ctx, dialogID, userID)
	if err != nil {
		return nil, err
	}

	return &StartActionResponse{
		ActionID: actionID,
		DialogID: dialogID,
		UserID:   userID,
	}, nil
}

// SubmitSpeech handles the logic of scoring speech and saving the result.
func (s *DialogService) SubmitSpeech(ctx context.Context, req SubmitSpeechInput) (interface{}, *errors.AppError) {
	// 1. Read audio bytes
	audioBytes, err := io.ReadAll(req.AudioFile)
	if err != nil {
		return nil, errors.ValidationWrap("failed to read audio file", err)
	}

	// 2. Evaluate Pronunciation via Azure Speech
	scoreData, evalErr := s.audioRepo.EvaluateSpeech(ctx, audioBytes, req.OriginalText, req.Language)
	if evalErr != nil {
		return nil, evalErr
	}

	// 3. Upload User Audio to R2
	audioPath := fmt.Sprintf("dialogs/%s/actions/%s_speech.wav", req.DialogID, req.ActionID)
	audioURL, uploadErr := s.fileRepo.UploadBytes(ctx, audioBytes, audioPath, req.AudioContentType)
	if uploadErr != nil {
		return nil, uploadErr
	}

	// 4. Construct Metadata
	metadata := map[string]interface{}{
		"audio_url":    audioURL,
		"script_index": req.ScriptIndex,
		"score":        scoreData,
	}
	metadataJSON, _ := json.Marshal(metadata)

	// 5. Update user_actions table
	if saveErr := s.dialogRepo.SubmitSpeechAction(ctx, req.ActionID, req.UserID, metadataJSON); saveErr != nil {
		return nil, saveErr
	}

	return scoreData, nil
}

// StartChat starts a chat action for a dialog.
func (s *DialogService) StartChat(ctx context.Context, dialogID, userID string) (*StartActionResponse, *errors.AppError) {
	actionID, err := s.dialogRepo.StartChat(ctx, dialogID, userID)
	if err != nil {
		return nil, err
	}

	return &StartActionResponse{
		ActionID: actionID,
		DialogID: dialogID,
		UserID:   userID,
	}, nil
}

// GenerateImage generates an image from a prompt, uploads it to R2, and returns the URL.
func (s *DialogService) GenerateImage(ctx context.Context, prompt string) (*GenerateImageResponse, *errors.AppError) {
	// 1. Generate image bytes
	imageBytes, err := s.imageRepo.GenerateImage(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// 2. Upload to R2
	imageID := uuid.New().String()
	key := fmt.Sprintf("dialogs/images/%s.png", imageID)
	imageURL, err := s.fileRepo.UploadBytes(ctx, imageBytes, key, "image/png")
	if err != nil {
		return nil, err
	}

	return &GenerateImageResponse{
		ImageURL: imageURL,
	}, nil
}

func (s *DialogService) failRemainingMediaJobs(ctx context.Context, dialogID, message string) {
	for _, processName := range GetProcessNames()[1:] {
		_ = s.batchRepo.UpdateJob(ctx, dialogID, processName, BATCH_FAILED, message)
	}
}

func voiceForDialogLanguage(language string) string {
	switch strings.ToLower(language) {
	case "chinese":
		return "zh-CN-XiaoxiaoNeural"
	case "japanese":
		return "ja-JP-NanamiNeural"
	case "french":
		return "fr-FR-DeniseNeural"
	case "spanish":
		return "es-ES-ElviraNeural"
	case "portuguese":
		return "pt-BR-FranciscaNeural"
	case "arabic":
		return "ar-SA-ZariyahNeural"
	case "russian":
		return "ru-RU-SvetlanaNeural"
	default:
		return "en-US-AvaMultilingualNeural"
	}
}
