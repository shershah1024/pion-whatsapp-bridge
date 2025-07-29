package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/pion/webrtc/v3"
)

// OpenAIRealtimeClient handles the connection to OpenAI's Realtime API
type OpenAIRealtimeClient struct {
	apiKey         string
	peerConnection *webrtc.PeerConnection
	dataChannel    *webrtc.DataChannel
	ephemeralToken string
}

// NewOpenAIRealtimeClient creates a new OpenAI Realtime client
func NewOpenAIRealtimeClient(apiKey string) *OpenAIRealtimeClient {
	return &OpenAIRealtimeClient{
		apiKey: apiKey,
	}
}

// EphemeralTokenResponse represents the response from the ephemeral token endpoint
type EphemeralTokenResponse struct {
	ClientSecret struct {
		Value     string `json:"value"`
		ExpiresAt int64  `json:"expires_at"`
	} `json:"client_secret"`
}

// GetEphemeralToken fetches a temporary token for the Realtime API
func (c *OpenAIRealtimeClient) GetEphemeralToken() error {
	url := "https://api.openai.com/v1/realtime/sessions"
	
	reqBody := map[string]interface{}{
		"model": "gpt-4o-realtime-preview-2024-12-17",
		"voice": "alloy",
	}
	
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get ephemeral token: %s - %s", resp.Status, string(body))
	}
	
	var tokenResp EphemeralTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return err
	}
	
	c.ephemeralToken = tokenResp.ClientSecret.Value
	log.Printf("✅ Got ephemeral token, expires at: %d", tokenResp.ClientSecret.ExpiresAt)
	
	return nil
}

// ConnectToRealtimeAPI establishes a WebRTC connection to OpenAI's Realtime API
func (c *OpenAIRealtimeClient) ConnectToRealtimeAPI(api *webrtc.API) error {
	// Create a new peer connection
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	
	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("failed to create peer connection: %v", err)
	}
	c.peerConnection = pc
	
	// Create a data channel for Realtime API communication
	dataChannel, err := pc.CreateDataChannel("oai-events", &webrtc.DataChannelInit{
		Ordered: func(b bool) *bool { return &b }(true),
	})
	if err != nil {
		return fmt.Errorf("failed to create data channel: %v", err)
	}
	
	// Set up data channel handlers
	dataChannel.OnOpen(func() {
		log.Println("✅ OpenAI Realtime data channel opened")
		
		// Send initial configuration
		config := map[string]interface{}{
			"type": "session.update",
			"session": map[string]interface{}{
				"modalities":     []string{"text", "audio"},
				"instructions":   "You are a helpful AI assistant. Please help the caller with their questions.",
				"voice":         "alloy",
				"input_audio_format":  "pcm16",
				"output_audio_format": "pcm16",
				"input_audio_transcription": map[string]string{
					"model": "whisper-1",
				},
			},
		}
		
		configJSON, _ := json.Marshal(config)
		dataChannel.SendText(string(configJSON))
	})
	
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("📥 Received from OpenAI: %s", string(msg.Data))
		
		// Parse the message
		var event map[string]interface{}
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Printf("Failed to parse message: %v", err)
			return
		}
		
		// Handle different event types
		eventType, _ := event["type"].(string)
		switch eventType {
		case "session.created":
			log.Println("✅ Session created with OpenAI")
		case "conversation.item.created":
			log.Println("📝 Conversation item created")
		case "response.audio.delta":
			// Audio data from OpenAI
			c.handleAudioDelta(event)
		case "response.audio_transcript.delta":
			// Transcript update
			c.handleTranscriptDelta(event)
		}
	})
	
	c.dataChannel = dataChannel
	
	// Create offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("failed to create offer: %v", err)
	}
	
	// Set local description
	if err := pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("failed to set local description: %v", err)
	}
	
	// Send offer to OpenAI
	answer, err := c.sendOfferToOpenAI(offer.SDP)
	if err != nil {
		return fmt.Errorf("failed to send offer to OpenAI: %v", err)
	}
	
	// Set remote description
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answer,
	}); err != nil {
		return fmt.Errorf("failed to set remote description: %v", err)
	}
	
	return nil
}

// sendOfferToOpenAI sends the WebRTC offer to OpenAI and gets the answer
func (c *OpenAIRealtimeClient) sendOfferToOpenAI(offerSDP string) (string, error) {
	if c.ephemeralToken == "" {
		return "", fmt.Errorf("no ephemeral token available")
	}
	
	url := "https://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-12-17"
	
	req, err := http.NewRequest("POST", url, bytes.NewReader([]byte(offerSDP)))
	if err != nil {
		return "", err
	}
	
	req.Header.Set("Authorization", "Bearer "+c.ephemeralToken)
	req.Header.Set("Content-Type", "application/sdp")
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	answerSDP, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API error: %s - %s", resp.Status, string(answerSDP))
	}
	
	return string(answerSDP), nil
}

// SendAudioToOpenAI sends audio data to OpenAI
func (c *OpenAIRealtimeClient) SendAudioToOpenAI(audioData []byte) error {
	if c.dataChannel == nil || c.dataChannel.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("data channel not open")
	}
	
	// Send audio append event
	event := map[string]interface{}{
		"type": "input_audio_buffer.append",
		"audio": audioData, // Should be base64 encoded PCM16 audio
	}
	
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}
	
	return c.dataChannel.SendText(string(eventJSON))
}

// CommitAudioBuffer commits the audio buffer for processing
func (c *OpenAIRealtimeClient) CommitAudioBuffer() error {
	if c.dataChannel == nil || c.dataChannel.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("data channel not open")
	}
	
	event := map[string]interface{}{
		"type": "input_audio_buffer.commit",
	}
	
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}
	
	return c.dataChannel.SendText(string(eventJSON))
}

// handleAudioDelta processes audio responses from OpenAI
func (c *OpenAIRealtimeClient) handleAudioDelta(event map[string]interface{}) {
	// Extract audio data from the event
	// This would contain base64 encoded PCM16 audio data
	// You would decode and play this audio to the WhatsApp caller
	
	if delta, ok := event["delta"].(string); ok {
		log.Printf("🔊 Received audio delta: %d bytes", len(delta))
		// TODO: Decode base64 and send to WhatsApp caller
	}
}

// handleTranscriptDelta processes transcript updates
func (c *OpenAIRealtimeClient) handleTranscriptDelta(event map[string]interface{}) {
	if delta, ok := event["delta"].(string); ok {
		log.Printf("💬 Transcript: %s", delta)
	}
}

// Close closes the connection to OpenAI
func (c *OpenAIRealtimeClient) Close() {
	if c.dataChannel != nil {
		c.dataChannel.Close()
	}
	if c.peerConnection != nil {
		c.peerConnection.Close()
	}
}