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

	"github.com/windfall/uwu_service/internal/errors"
)

// AzureSpeechClient wraps the Azure AI Speech REST API.
type AzureSpeechClient struct {
	apiKey string
	region string
	client *http.Client
}

// NewAzureSpeechClient creates a new Azure Speech client.
func NewAzureSpeechClient(apiKey, region string) *AzureSpeechClient {
	return &AzureSpeechClient{
		apiKey: apiKey,
		region: region,
		client: &http.Client{
			Timeout: 30 * time.Second, // 30 second timeout
		},
	}
}

// AnalyzeVocabAudio sends an audio file to Azure Speech-to-Text API for vocabulary practice.
// It accepts wav audio data and returns the raw JSON response.
func (c *AzureSpeechClient) AnalyzeVocabAudio(ctx context.Context, audioData []byte, referenceText string) (map[string]interface{}, error) {
	if c.apiKey == "" || c.region == "" {
		return nil, errors.New(errors.ErrAIService, "Azure Speech credentials not configured")
	}

	// Construct URL for Short Audio API (REST)
	// Docs: https://learn.microsoft.com/en-us/azure/ai-services/speech-service/rest-speech-to-text-short
	u := url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s.stt.speech.microsoft.com", c.region),
		Path:   "/speech/recognition/conversation/cognitiveservices/v1",
	}

	// Query parameters
	q := u.Query()
	q.Set("language", "en-US")  // Default to English, can be parameterized if needed
	q.Set("format", "detailed") // Request detailed output
	u.RawQuery = q.Encode()

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(audioData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 1. สร้าง Config Object สำหรับการตรวจ
	pronAssessmentParams := map[string]interface{}{
		"ReferenceText": referenceText, // เช่น "Apple"
		"GradingSystem": "HundredMark", // คะแนนเต็ม 100
		"Granularity":   "Phoneme",     // ขอรายละเอียดระดับตัวสะกด
		"Dimension":     "Comprehensive",
	}

	// 2. แปลงเป็น JSON Bytes
	jsonBytes, err := json.Marshal(pronAssessmentParams)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	// 3. แปลง JSON เป็น Base64 String
	base64Params := base64.StdEncoding.EncodeToString(jsonBytes)

	// 4. ยัดใส่ Header "Pronunciation-Assessment"
	req.Header.Set("Pronunciation-Assessment", base64Params)

	// Set headers
	req.Header.Set("Ocp-Apim-Subscription-Key", c.apiKey)
	req.Header.Set("Content-Type", "audio/wav; codecs=audio/pcm; samplerate=16000")
	req.Header.Set("Accept", "application/json;text/xml")

	// Execute request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure speech api error %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// AnalyzeShadowingAudio sends an audio file to Azure Speech-to-Text API for shadowing practice.
// It enables miscue detection (Insertion, Omission, Substitution).
// Note: EnableMiscue is only fully supported for en-US in REST API.
func (c *AzureSpeechClient) AnalyzeShadowingAudio(ctx context.Context, audioData []byte, referenceText, language string) (map[string]interface{}, error) {
	if c.apiKey == "" || c.region == "" {
		return nil, errors.New(errors.ErrAIService, "Azure Speech credentials not configured")
	}

	// Default to en-US if not specified (EnableMiscue works best with en-US)
	if language == "" {
		language = "en-US"
	}

	// Construct URL for Short Audio API
	u := url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s.stt.speech.microsoft.com", c.region),
		Path:   "/speech/recognition/conversation/cognitiveservices/v1",
	}

	// Query parameters
	q := u.Query()
	q.Set("language", language)
	q.Set("format", "detailed")
	u.RawQuery = q.Encode()

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(audioData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Config for Shadowing
	pronAssessmentParams := map[string]interface{}{
		"ReferenceText": referenceText, // 今天吃什么
		"GradingSystem": "HundredMark",
		"Granularity":   "Word", // Less granular than Phoneme
		"EnableMiscue":  true,   // Enable Insertion, Omission, Substitution detection
		"Dimension":     "Comprehensive",
	}

	// Marshal params
	jsonBytes, err := json.Marshal(pronAssessmentParams)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	// Base64 encode
	base64Params := base64.StdEncoding.EncodeToString(jsonBytes)

	// Set headers
	req.Header.Set("Pronunciation-Assessment", base64Params)
	req.Header.Set("Ocp-Apim-Subscription-Key", c.apiKey)
	req.Header.Set("Content-Type", "audio/wav; codecs=audio/pcm; samplerate=16000")
	req.Header.Set("Accept", "application/json;text/xml")

	// Execute request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure speech api error %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Deduplicate words in the response
	result = DeduplicateWords(result)

	return result, nil
}

// DeduplicateWords processes the Azure Speech response to handle duplicated words.
// When Azure returns the same word multiple times (e.g., one with "Insertion" error and one with other errors),
// this function keeps only the word with "Insertion" error type and calculates the average AccuracyScore.
func DeduplicateWords(result map[string]interface{}) map[string]interface{} {
	nbestRaw, ok := result["NBest"]
	if !ok {
		return result
	}

	nbest, ok := nbestRaw.([]interface{})
	if !ok || len(nbest) == 0 {
		return result
	}

	// Process the first NBest entry (primary result)
	firstNBest, ok := nbest[0].(map[string]interface{})
	if !ok {
		return result
	}

	wordsRaw, ok := firstNBest["Words"]
	if !ok {
		return result
	}

	words, ok := wordsRaw.([]interface{})
	if !ok {
		return result
	}

	// Group words by their Word value
	wordGroups := make(map[string][]int) // word -> indices
	for i, wordRaw := range words {
		word, ok := wordRaw.(map[string]interface{})
		if !ok {
			continue
		}
		wordText, ok := word["Word"].(string)
		if !ok {
			continue
		}
		wordGroups[wordText] = append(wordGroups[wordText], i)
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
			word := words[idx].(map[string]interface{})
			errorType, _ := word["ErrorType"].(string)

			if errorType == "Insertion" {
				insertionIndex = idx
			}

			// Get AccuracyScore (could be float64 or json.Number)
			var accuracy float64
			switch v := word["AccuracyScore"].(type) {
			case float64:
				accuracy = v
			case int:
				accuracy = float64(v)
			default:
				continue
			}
			totalAccuracy += accuracy
			count++
		}

		// If we found an Insertion, keep it and remove others
		if insertionIndex != -1 && count > 0 {
			// Calculate average and update the Insertion word
			avgAccuracy := totalAccuracy / float64(count)
			words[insertionIndex].(map[string]interface{})["AccuracyScore"] = avgAccuracy

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
		newWords := make([]interface{}, 0, len(words)-len(indicesToRemove))
		for i, word := range words {
			if !indicesToRemove[i] {
				newWords = append(newWords, word)
			}
		}
		firstNBest["Words"] = newWords
	}

	return result
}
