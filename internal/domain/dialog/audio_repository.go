package dialog

import (
	"context"
	"os"

	"github.com/windfall/uwu_service/internal/infra/client"
	"github.com/windfall/uwu_service/pkg/errors"
)

// AudioRepository generates dialog audio.
type AudioRepository interface {
	Synthesize(ctx context.Context, text, voice string) ([]byte, *errors.AppError)
	EvaluateSpeech(ctx context.Context, tempWav *os.File, referenceText string, language string) (*client.AzureEvaluationSpeech, *errors.AppError)
}

type audioRepository struct {
	speechClient *client.AzureSpeechClient
}

// NewAudioRepository creates a new dialog audio repository.
func NewAudioRepository(speechClient *client.AzureSpeechClient) AudioRepository {
	return &audioRepository{speechClient: speechClient}
}

func (r *audioRepository) Synthesize(ctx context.Context, text, voice string) ([]byte, *errors.AppError) {
	if r.speechClient == nil {
		return nil, errors.Internal("dialog speech client not configured")
	}
	return r.speechClient.Synthesize(ctx, text, voice)
}

func (r *audioRepository) EvaluateSpeech(ctx context.Context, tempWav *os.File, referenceText string, language string) (*client.AzureEvaluationSpeech, *errors.AppError) {
	if r.speechClient == nil {
		return nil, errors.Internal("dialog speech client not configured")
	}

	audioData, err := os.ReadFile(tempWav.Name())
	if err != nil {
		return nil, errors.InternalWrap("failed to read temp file", err)
	}

	return r.speechClient.EvaluatePronunciation(ctx, audioData, referenceText, language)
}
