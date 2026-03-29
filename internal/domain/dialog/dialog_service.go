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

// -------------------------------------------------
// Remove this later and use Metadata instead
// -------------------------------------------------
type StartDialogResponse struct {
	ActionID string          `json:"action_id"`
	DialogID string          `json:"dialog_id"`
	UserID   string          `json:"user_id"`
	Metadata *SpeechMetadata `json:"metadata"`
}

// SpeechMetadata represents the metadata for speech actions
type SpeechMetadata struct {
	SituationText     string         `json:"situation_text"`
	SituationAudioURL string         `json:"situation_audio_url"`
	Scripts           []SpeechScript `json:"scripts"`
	// To implement later
	Attempts [][]SpeechScript `json:"attempts"`
}

// ChatMetadata is the structure stored in user_actions.metadata for chat actions.
type ChatMetadata struct {
	SituationText       string        `json:"situation_text"`
	ChatObjective       ChatObjective `json:"chat_objective"`
	Messages            []ChatMessage `json:"messages"`
	CompletedObjectives []string      `json:"completed_objectives"`
	Status              string        `json:"status,omitempty"`
}

type ChatMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	Suggestion string `json:"suggestion"`
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
// This function will reset the chat history and completed objectives every time the user starts a chat.
func (s *DialogService) StartChat(ctx context.Context, dialogID, userID string) (*ChatMetadata, *errors.AppError) {
	// 1. Fetch dialog details to get chat snapshot
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
		SituationText:       details.ChatMode.Situation,
		ChatObjective:       details.ChatMode.Objectives,
		Messages:            []ChatMessage{},
		CompletedObjectives: []string{},
	}

	// 4. Create action record
	metadataJSON, _ := json.Marshal(metadata)
	if _, err := s.dialogRepo.StartChat(ctx, dialogID, userID, metadataJSON); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// SubmitChat handles enqueuing a chat message for background processing.
func (s *DialogService) SubmitChat(ctx context.Context, payload ReplyChatMessagePayload) (*ChatMetadata, *errors.AppError) {
	// 1. Validate that a submit_chat action exists
	action, exists, err := s.dialogRepo.GetActionByUserID(ctx, payload.DialogID, payload.UserID, "submit_chat")
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

	// 3. Update status to processing and append user message (temp version)
	chatMeta.Status = BATCH_PROCESSING
	chatMeta.Messages = append(chatMeta.Messages, ChatMessage{Role: "user", Content: payload.Message})

	metadataJSON, _ := json.Marshal(chatMeta)
	if err := s.dialogRepo.UpdateChatAction(ctx, action.ID, payload.UserID, metadataJSON); err != nil {
		return nil, err
	}

	return &chatMeta, nil
}

// ProcessReplyChatMessage handles the background logic of replying to a chat message.
// worker method
func (s *DialogService) ProcessReplyChatMessage(ctx context.Context, payload ReplyChatMessagePayload) {
	// 1. Get existing chat action metadata (conversation history + progress)
	action, exists, err := s.dialogRepo.GetActionByUserID(ctx, payload.DialogID, payload.UserID, "submit_chat")
	if err != nil || !exists {
		return
	}

	var chatMeta ChatMetadata
	if len(action.Metadata) > 0 {
		_ = json.Unmarshal(action.Metadata, &chatMeta)
	}

	// 2. Remove the temp user message (to avoid duplication)
	if len(chatMeta.Messages) > 0 {
		lastMsg := chatMeta.Messages[len(chatMeta.Messages)-1]
		if lastMsg.Role == "user" && lastMsg.Content == payload.Message {
			chatMeta.Messages = chatMeta.Messages[:len(chatMeta.Messages)-1]
		}
	}

	// 3. Call AI with conversation history
	result, appErr := s.aiRepo.ReplyUserMessage(ctx, chatMeta.ChatObjective, chatMeta.Messages, chatMeta.SituationText, payload.Message)
	if appErr != nil {
		chatMeta.Status = BATCH_FAILED
		metadataJSON, _ := json.Marshal(chatMeta)
		_ = s.dialogRepo.UpdateChatAction(ctx, action.ID, payload.UserID, metadataJSON)
		return
	}

	// 3. Append messages to history
	chatMeta.Messages = append(chatMeta.Messages,
		ChatMessage{Role: "user", Content: payload.Message},
		ChatMessage{Role: "assistant", Content: result.ReplyMessage},
	)

	// 4. Merge completed objectives (deduplicate)
	existing := make(map[string]bool)
	for _, text := range chatMeta.CompletedObjectives {
		existing[text] = true
	}
	totalRequirements := len(chatMeta.ChatObjective.Requirements)
	for _, idx := range result.CompletedObjectivesIndexes {
		if idx >= 0 && idx < totalRequirements {
			newCompleted := chatMeta.ChatObjective.Requirements[idx]
			if !existing[newCompleted] {
				chatMeta.CompletedObjectives = append(chatMeta.CompletedObjectives, newCompleted)
			}
		}
	}

	// 5. Update status and save metadata
	chatMeta.Status = BATCH_COMPLETED
	metadataJSON, _ := json.Marshal(chatMeta)

	_ = s.dialogRepo.UpdateChatAction(ctx, action.ID, payload.UserID, metadataJSON)
}

// GetSubmitChat returns the current status and metadata of a chat submission.
func (s *DialogService) GetSubmitChat(ctx context.Context, dialogID, userID string) (*ChatMetadata, *errors.AppError) {
	action, exists, err := s.dialogRepo.GetActionByUserID(ctx, dialogID, userID, "submit_chat")
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

	return &chatMeta, nil
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
