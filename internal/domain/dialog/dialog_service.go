package dialog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/windfall/uwu_service/internal/infra/client"
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

// StartChatResponse is returned after starting a chat action.
type StartChatResponse struct {
	ActionID            string               `json:"action_id"`
	DialogID            string               `json:"dialog_id"`
	UserID              string               `json:"user_id"`
	ChatMode            *ChatMode            `json:"chat_mode"`
	Messages            []client.ChatMessage `json:"messages"`
	CompletedObjectives []int                `json:"completed_objectives"`
}

type StartDialogResponse struct {
	ActionID string          `json:"action_id"`
	DialogID string          `json:"dialog_id"`
	UserID   string          `json:"user_id"`
	Metadata *SpeechMetadata `json:"metadata"`
}

// SpeechMetadata represents the metadata for speech actions
type SpeechMetadata struct {
	SituationText     string           `json:"situation_text"`
	SituationAudioURL string           `json:"situation_audio_url"`
	Scripts           []SpeechScript   `json:"scripts"`
	Attempts          [][]SpeechScript `json:"attempts"`
}

// ChatMetadata is the structure stored in user_actions.metadata for chat actions.
type ChatMetadata struct {
	ChatMode            *ChatMode            `json:"chat_mode,omitempty"`
	Messages            []client.ChatMessage `json:"messages"`
	CompletedObjectives []int                `json:"completed_objectives"`
	TotalObjectives     int                  `json:"total_objectives"`
}

// SubmitChatResponse is returned after submitting a chat message.
type SubmitChatResponse struct {
	ReplyMessage        string `json:"reply_message"`
	CompletedObjectives []int  `json:"completed_objectives"`
	TotalObjectives     int    `json:"total_objectives"`
	Feedback            string `json:"feedback,omitempty"`
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
func (s *DialogService) GetDialogDetails(ctx context.Context, dialogID string) (*DialogDetailsResponse, *errors.AppError) {
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

	if metaProcessing == nil {
		metaProcessing = &metadata
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
func (s *DialogService) StartSpeech(ctx context.Context, dialogID, userID string) (*StartDialogResponse, *errors.AppError) {
	// 1. Check if user already started this action (Idempotency)
	action, exists, err := s.dialogRepo.GetActionByUserID(ctx, dialogID, userID, "submit_speech")
	if err != nil {
		return nil, err
	}

	if exists {
		var metadata SpeechMetadata
		if err := json.Unmarshal(action.Metadata, &metadata); err != nil {
			return nil, errors.InternalWrap("failed to parse speech metadata", err)
		}

		return &StartDialogResponse{
			ActionID: action.ID,
			DialogID: dialogID,
			UserID:   userID,
			Metadata: &metadata,
		}, nil
	}

	// 2. Fetch dialog details to get speech snapshot
	learningItem, err := s.dialogRepo.GetDialog(ctx, dialogID)
	if err != nil {
		return nil, err
	}

	var details DialogDetails
	if err := json.Unmarshal(learningItem.Details, &details); err != nil {
		return nil, errors.InternalWrap("failed to parse dialog details", err)
	}

	// 3. Create initial metadata snapshot
	metadata := SpeechMetadata{
		SituationText:     details.SpeechMode.Situation,
		SituationAudioURL: details.AudioURL,
		Scripts:           details.SpeechMode.Script,
		Attempts:          [][]SpeechScript{},
	}
	metadataJSON, _ := json.Marshal(metadata)

	// 4. Create action record
	actionID, err := s.dialogRepo.StartSpeech(ctx, dialogID, userID, metadataJSON)
	if err != nil {
		return nil, err
	}

	return &StartDialogResponse{
		ActionID: actionID,
		DialogID: dialogID,
		UserID:   userID,
		Metadata: &metadata,
	}, nil
}

// SubmitSpeech handles the logic of scoring speech and saving the result.
func (s *DialogService) SubmitSpeech(ctx context.Context, input SubmitSpeechInput) (*SpeechMetadata, *errors.AppError) {
	// 1. Get active action
	action, exists, err := s.dialogRepo.GetActionByUserID(ctx, input.DialogID, input.UserID, "submit_speech")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NotFound("speech action not found for this dialog")
	}

	var metadata SpeechMetadata
	if err := json.Unmarshal(action.Metadata, &metadata); err != nil {
		return nil, errors.InternalWrap("failed to parse speech metadata", err)
	}

	if input.ScriptIndex < 0 || input.ScriptIndex >= len(metadata.Scripts) {
		return nil, errors.Validation("invalid script index")
	}

	// 2. Create temp file & Analyze with Azure Speech
	tempWav, err := s.fileRepo.CreateTempFile(input.AudioFile, input.AudioWavPath)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempWav.Name())

	evaluation, err := s.audioRepo.EvaluateSpeech(ctx, tempWav, input.ReferenceText, input.Language)
	if err != nil {
		return nil, errors.InternalWrap("failed to analyze shadowing audio", err)
	}

	// loop remove property: Phonemes, Syllables
	newWords := make([]EvaluationWord, 0)
	for _, word := range evaluation.NBest[0].Words {
		newWords = append(newWords, EvaluationWord{
			AccuracyScore: word.AccuracyScore,
			Confidence:    word.Confidence,
			Duration:      word.Duration,
			ErrorType:     word.ErrorType,
			Offset:        word.Offset,
			Word:          word.Word,
		})
	}

	// 3. Update metadata
	metadata.Scripts[input.ScriptIndex].Evaluation = &Evaluation{
		AccuracyScore:     evaluation.NBest[0].AccuracyScore,
		FluencyScore:      evaluation.NBest[0].FluencyScore,
		PronScore:         evaluation.NBest[0].PronScore,
		CompletenessScore: evaluation.NBest[0].CompletenessScore,
		DisplayText:       evaluation.NBest[0].DisplayText,
		Duration:          evaluation.Duration,
		Words:             newWords,
	}
	metadataJSON, _ := json.Marshal(metadata)
	if err := s.dialogRepo.SubmitSpeechAction(ctx, action.ID, input.UserID, metadataJSON); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// StartChat starts a chat action for a dialog.
func (s *DialogService) StartChat(ctx context.Context, dialogID, userID string) (*StartChatResponse, *errors.AppError) {
	// 1. Check if user already started this action (Idempotency)
	action, exists, err := s.dialogRepo.GetActionByUserID(ctx, dialogID, userID, "submit_chat")
	if err != nil {
		return nil, err
	}

	if exists {
		var metadata ChatMetadata
		if err := json.Unmarshal(action.Metadata, &metadata); err != nil {
			return nil, errors.InternalWrap("failed to parse chat metadata", err)
		}

		return &StartChatResponse{
			ActionID:            action.ID,
			DialogID:            dialogID,
			UserID:              userID,
			ChatMode:            metadata.ChatMode,
			Messages:            metadata.Messages,
			CompletedObjectives: metadata.CompletedObjectives,
		}, nil
	}

	// 2. Fetch dialog details to get chat snapshot
	learningItem, err := s.dialogRepo.GetDialog(ctx, dialogID)
	if err != nil {
		return nil, err
	}

	var details DialogDetails
	if err := json.Unmarshal(learningItem.Details, &details); err != nil {
		return nil, errors.InternalWrap("failed to parse dialog details", err)
	}

	// 3. Create initial metadata snapshot
	metadata := ChatMetadata{
		Messages:            []client.ChatMessage{},
		CompletedObjectives: []int{},
		TotalObjectives:     len(details.ChatMode.Objectives.Requirements),
	}
	chatJSON, _ := json.Marshal(details.ChatMode)
	_ = json.Unmarshal(chatJSON, &metadata.ChatMode)
	metadataJSON, _ := json.Marshal(metadata)

	// 4. Create action record
	actionID, err := s.dialogRepo.StartChat(ctx, dialogID, userID, metadataJSON)
	if err != nil {
		return nil, err
	}

	return &StartChatResponse{
		ActionID:            actionID,
		DialogID:            dialogID,
		UserID:              userID,
		ChatMode:            metadata.ChatMode,
		Messages:            metadata.Messages,
		CompletedObjectives: metadata.CompletedObjectives,
	}, nil
}

// SubmitChat handles the logic of replying to a chat message and tracking objectives.
func (s *DialogService) SubmitChat(ctx context.Context, input SubmitChatInput) (*SubmitChatResponse, *errors.AppError) {
	// 1. Get dialog to extract ChatMode
	learningItem, appErr := s.dialogRepo.GetDialog(ctx, input.DialogID)
	if appErr != nil {
		return nil, appErr
	}

	var details DialogDetails
	if err := json.Unmarshal(learningItem.Details, &details); err != nil {
		return nil, errors.InternalWrap("failed to parse dialog details", err)
	}

	if details.ChatMode.Situation == "" {
		return nil, errors.Validation("this dialog does not have a chat mode")
	}

	// 2. Get existing chat action metadata (conversation history + progress)
	action, exists, err := s.dialogRepo.GetActionByUserID(ctx, input.DialogID, input.UserID, "submit_chat")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NotFound("chat action not found for this dialog")
	}

	var chatMeta ChatMetadata
	if len(action.Metadata) > 0 {
		_ = json.Unmarshal(action.Metadata, &chatMeta)
	}

	// Sync snapshot if missing (legacy)
	if chatMeta.ChatMode == nil && details.ChatMode.Situation != "" {
		chatMeta.ChatMode = &details.ChatMode
	}

	// Initialize total objectives if missing
	if chatMeta.TotalObjectives == 0 {
		chatMeta.TotalObjectives = len(details.ChatMode.Objectives.Requirements)
	}

	// 3. Call AI with conversation history (using snapshot or fresh details)
	targetChatMode := details.ChatMode
	if chatMeta.ChatMode != nil {
		targetChatMode = *chatMeta.ChatMode
	}

	result, appErr := s.aiRepo.ChatReply(ctx, targetChatMode, chatMeta.Messages, input.Message)
	if appErr != nil {
		return nil, appErr
	}

	// 4. Append messages to history
	chatMeta.Messages = append(chatMeta.Messages,
		client.ChatMessage{Role: "user", Content: input.Message},
		client.ChatMessage{Role: "assistant", Content: result.ReplyMessage},
	)

	// 5. Merge completed objectives (deduplicate)
	existing := make(map[int]bool)
	for _, idx := range chatMeta.CompletedObjectives {
		existing[idx] = true
	}
	for _, idx := range result.CompletedObjectivesIndexes {
		if idx >= 0 && idx < chatMeta.TotalObjectives && !existing[idx] {
			chatMeta.CompletedObjectives = append(chatMeta.CompletedObjectives, idx)
			existing[idx] = true
		}
	}

	// 6. Save updated metadata
	metadataJSON, mErr := json.Marshal(chatMeta)
	if mErr != nil {
		return nil, errors.InternalWrap("failed to marshal chat metadata", mErr)
	}

	if err := s.dialogRepo.UpdateChatAction(ctx, action.ID, input.UserID, metadataJSON); err != nil {
		return nil, err
	}

	// 7. Return response
	return &SubmitChatResponse{
		ReplyMessage:        result.ReplyMessage,
		CompletedObjectives: chatMeta.CompletedObjectives,
		TotalObjectives:     chatMeta.TotalObjectives,
		Feedback:            result.Feedback,
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
