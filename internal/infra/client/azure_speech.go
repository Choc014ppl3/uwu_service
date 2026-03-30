package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/windfall/uwu_service/pkg/errors"
)

// ConvertLangCode
var ConvertLangCode = map[string]string{
	"english":    "en-US",
	"chinese":    "zh-CN",
	"japanese":   "ja-JP",
	"french":     "fr-FR",
	"spanish":    "es-ES",
	"portuguese": "pt-BR",
	"arabic":     "ar-SA",
	"russian":    "ru-RU",
}

// AzureWord
type AzureWord struct {
	AccuracyScore float64 `json:"AccuracyScore"`
	Confidence    float64 `json:"Confidence"`
	Duration      int     `json:"Duration"`
	ErrorType     string  `json:"ErrorType"`
	Offset        int     `json:"Offset"`
	Word          string  `json:"Word"`
	Phonemes      []any   `json:"Phonemes"`
	Syllables     []any   `json:"Syllables"`
}

// AzureNBest
type AzureNBest struct {
	AccuracyScore     float64     `json:"AccuracyScore"`
	CompletenessScore float64     `json:"CompletenessScore"`
	Confidence        float64     `json:"Confidence"`
	DisplayText       string      `json:"DisplayText"`
	FluencyScore      float64     `json:"FluencyScore"`
	PronScore         float64     `json:"PronScore"`
	Words             []AzureWord `json:"Words"`
}

// AzureEvaluationSpeech
type AzureEvaluationSpeech struct {
	DisplayText string       `json:"DisplayText"`
	Duration    int          `json:"Duration"`
	NBest       []AzureNBest `json:"NBest"`
}

// AzureSpeechClient wraps Azure AI Speech text-to-speech.
type AzureSpeechClient struct {
	apiKey string
	region string
	client *http.Client
}

// NewAzureSpeechClient creates a new Azure speech client.
func NewAzureSpeechClient(apiKey, region string) *AzureSpeechClient {
	return &AzureSpeechClient{
		apiKey: apiKey,
		region: region,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Synthesize generates speech from text using Azure AI Speech.
func (c *AzureSpeechClient) Synthesize(ctx context.Context, text, voice string) ([]byte, *errors.AppError) {
	if c.apiKey == "" || c.region == "" {
		return nil, errors.Internal("Azure speech credentials not configured")
	}

	if voice == "" {
		voice = "en-US-AvaMultilingualNeural"
	}

	u := url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s.tts.speech.microsoft.com", c.region),
		Path:   "/cognitiveservices/v1",
	}

	ssml := fmt.Sprintf(
		"<speak version='1.0' xml:lang='en-US'><voice xml:lang='en-US' xml:gender='Female' name='%s'>%s</voice></speak>",
		voice,
		text,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewBufferString(ssml))
	if err != nil {
		return nil, errors.InternalWrap("failed to create azure speech request", err)
	}

	req.Header.Set("Ocp-Apim-Subscription-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/ssml+xml")
	req.Header.Set("X-Microsoft-OutputFormat", "audio-16khz-128kbitrate-mono-mp3")
	req.Header.Set("User-Agent", "uwu_service")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.InternalWrap("failed to send azure speech request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Internal(fmt.Sprintf("azure speech api error %d: %s", resp.StatusCode, string(body)))
	}

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.InternalWrap("failed to read azure speech response", err)
	}

	return audioBytes, nil
}

// EvaluatePronunciation assesses pronunciation of audio bytes against a reference text.
func (c *AzureSpeechClient) EvaluatePronunciation(ctx context.Context, audioBytes []byte, referenceText string, language string) (*AzureEvaluationSpeech, *errors.AppError) {
	if c.apiKey == "" || c.region == "" {
		return nil, errors.Internal("Azure speech credentials not configured")
	}

	// Convert language to Azure Speech format
	language = ConvertLangCode[language]

	u := url.URL{
		Scheme:   "https",
		Host:     fmt.Sprintf("%s.stt.speech.microsoft.com", c.region),
		Path:     "/speech/recognition/conversation/cognitiveservices/v1",
		RawQuery: fmt.Sprintf("language=%s", url.QueryEscape(language)),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(audioBytes))
	if err != nil {
		return nil, errors.InternalWrap("failed to create azure speech recognition request", err)
	}

	// Create Pronunciation Assessment config
	assessmentConfig := map[string]interface{}{
		"ReferenceText": referenceText,
		"GradingSystem": "HundredMark",
		"Granularity":   "Word", // Word - less granular, Phoneme - more accurate
		"EnableMiscue":  true,   // Enable Insertion, Omission, Substitution detection
		"Dimension":     "Comprehensive",
	}

	configJSON, err := json.Marshal(assessmentConfig)
	if err != nil {
		return nil, errors.InternalWrap("failed to encode pronunciation config", err)
	}

	// Base64 encode
	encodedConfig := base64.StdEncoding.EncodeToString(configJSON)

	req.Header.Set("Ocp-Apim-Subscription-Key", c.apiKey)
	req.Header.Set("Content-Type", "audio/wav; codecs=audio/pcm; samplerate=16000") // Assuming standard 16kHz WAV
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Pronunciation-Assessment", encodedConfig)

	// Execute request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.InternalWrap("failed to send azure speech recognition request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errors.Internal(fmt.Sprintf("azure speech recognition api error %d: %s", resp.StatusCode, string(body)))
	}

	var result AzureEvaluationSpeech
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.InternalWrap("failed to decode azure speech recognition response", err)
	}

	// Deduplicate ErrorType: Insertion
	result = DeduplicateWords(result)

	return &result, nil
}

// DeduplicateWords processes the Azure Speech response to handle duplicated words.
// When Azure returns the same word multiple times (e.g., one with "Insertion" error and one with other errors),
// this function keeps only the word with "Insertion" error type and calculates the average AccuracyScore.
func DeduplicateWords(result AzureEvaluationSpeech) AzureEvaluationSpeech {
	if len(result.NBest) == 0 {
		return result
	}

	// Process the first NBest entry (primary result)
	nBest := &result.NBest[0]
	words := nBest.Words

	// Group words by their Word value
	wordGroups := make(map[string][]int) // word -> indices
	for i, word := range words {
		wordGroups[word.Word] = append(wordGroups[word.Word], i)
	}

	// Find duplicates and process them
	indicesToRemove := make(map[int]bool)
	for _, indices := range wordGroups {
		if len(indices) <= 1 {
			continue // Not a duplicate
		}

		// Find the Insertion index and calculate average AccuracyScore
		var insertionIndex int = -1
		var totalAccuracy float64 = 0
		var count int = 0

		for _, idx := range indices {
			word := words[idx]

			if word.ErrorType == "Insertion" {
				insertionIndex = idx
			}

			totalAccuracy += word.AccuracyScore
			count++
		}

		// If we found an Insertion, keep it and remove others
		if insertionIndex != -1 && count > 0 {
			// Calculate average and update the Insertion word
			avgAccuracy := totalAccuracy / float64(count)
			words[insertionIndex].AccuracyScore = avgAccuracy

			// Mark other indices for removal
			for _, idx := range indices {
				if idx != insertionIndex {
					indicesToRemove[idx] = true
				}
			}
		}
	}

	// Build new words array without removed indices
	if len(indicesToRemove) > 0 {
		newWords := make([]AzureWord, 0, len(words)-len(indicesToRemove))
		for i, word := range words {
			if !indicesToRemove[i] {
				newWords = append(newWords, word)
			}
		}
		nBest.Words = newWords
	}

	return result
}
