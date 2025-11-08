package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// DownloadAudio downloads an audio file from WhatsApp
// Returns the path to the downloaded temporary file
func DownloadAudio(audioID, phoneNumberID, token string) (string, error) {
	// Get media URL from WhatsApp API
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s", audioID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("❌ Error getting media info: %s - %s", resp.Status, string(body))
		return "", fmt.Errorf("failed to get media info: %s", resp.Status)
	}

	// Parse media info to get download URL
	var mediaInfo struct {
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
		Size     int64  `json:"file_size"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &mediaInfo); err != nil {
		return "", err
	}

	if mediaInfo.URL == "" {
		log.Printf("❌ No URL in media info")
		return "", fmt.Errorf("no URL in media info")
	}

	log.Printf("📥 Downloading audio from: %s", mediaInfo.URL)

	// Download the actual audio file
	audioReq, err := http.NewRequest("GET", mediaInfo.URL, nil)
	if err != nil {
		return "", err
	}

	audioReq.Header.Set("Authorization", "Bearer "+token)

	audioResp, err := client.Do(audioReq)
	if err != nil {
		return "", err
	}
	defer audioResp.Body.Close()

	if audioResp.StatusCode != http.StatusOK {
		log.Printf("❌ Error downloading audio: %s", audioResp.Status)
		return "", fmt.Errorf("failed to download audio: %s", audioResp.Status)
	}

	// Save to temporary file
	tmpFile, err := os.CreateTemp("", "whatsapp_audio_*.ogg")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	audioData, err := io.ReadAll(audioResp.Body)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	if _, err := tmpFile.Write(audioData); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	log.Printf("✅ Audio saved to temporary file: %s (size: %d bytes)", tmpFile.Name(), len(audioData))
	return tmpFile.Name(), nil
}

// TranscribeAudio transcribes an audio file using Azure GPT-4o
// Returns the transcription text
func TranscribeAudio(audioFilePath string) (string, error) {
	// Get Azure transcription endpoint and API key
	endpoint := os.Getenv("AZURE_TRANSCRIBE_ENDPOINT")
	apiKey := os.Getenv("AZURE_API_KEY")

	// Fallback to Azure OpenAI API key if AZURE_API_KEY not set
	if apiKey == "" {
		apiKey = os.Getenv("AZURE_OPENAI_API_KEY")
	}

	if endpoint == "" || apiKey == "" {
		log.Printf("⚠️ Azure transcription not configured (AZURE_TRANSCRIBE_ENDPOINT or API key missing)")
		return "", fmt.Errorf("transcription not configured")
	}

	// Check if file exists
	if _, err := os.Stat(audioFilePath); os.IsNotExist(err) {
		return "", fmt.Errorf("audio file not found: %s", audioFilePath)
	}

	// Read audio file
	audioFile, err := os.Open(audioFilePath)
	if err != nil {
		return "", err
	}
	defer audioFile.Close()

	audioData, err := io.ReadAll(audioFile)
	if err != nil {
		return "", err
	}

	// Create multipart form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file field
	part, err := writer.CreateFormFile("file", filepath.Base(audioFilePath))
	if err != nil {
		return "", err
	}

	if _, err := part.Write(audioData); err != nil {
		return "", err
	}

	// Add model field
	if err := writer.WriteField("model", "gpt-4o-transcribe"); err != nil {
		return "", err
	}

	if err := writer.Close(); err != nil {
		return "", err
	}

	// Send transcription request
	req, err := http.NewRequest("POST", endpoint, body)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	log.Printf("🎤 Sending audio for transcription to Azure (size: %d bytes)", len(audioData))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ Transcription error: %s - %s", resp.Status, string(respBody))
		return "", fmt.Errorf("transcription failed: %s", resp.Status)
	}

	// Parse transcription response
	var result struct {
		Text string `json:"text"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("❌ Failed to parse transcription response: %v", err)
		return "", err
	}

	if result.Text == "" {
		log.Printf("⚠️ Empty transcription result")
		return "", fmt.Errorf("empty transcription")
	}

	log.Printf("✅ Transcription: %s", result.Text)
	return result.Text, nil
}

// CleanupAudioFile removes a temporary audio file
func CleanupAudioFile(filePath string) {
	if filePath == "" {
		return
	}

	// Check if file exists before attempting to remove
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return // File doesn't exist, nothing to clean up
	}

	// Remove the file
	if err := os.Remove(filePath); err != nil {
		log.Printf("⚠️ Failed to cleanup audio file %s: %v", filePath, err)
	} else {
		log.Printf("🧹 Cleaned up audio file: %s", filePath)
	}
}
