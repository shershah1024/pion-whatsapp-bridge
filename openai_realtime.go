package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pion/webrtc/v3"
)

// OpenAIRealtimeClient handles the connection to OpenAI's Realtime API
type OpenAIRealtimeClient struct {
	apiKey           string
	peerConnection   *webrtc.PeerConnection
	dataChannel      *webrtc.DataChannel
	ephemeralToken   string
	audioTrack       *webrtc.TrackLocalStaticRTP
	remoteAudioTrack *webrtc.TrackRemote
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
	// For OpenAI, we need a fresh media engine and settings
	// because OpenAI has different requirements than WhatsApp
	m := &webrtc.MediaEngine{}
	
	// Register Opus codec for OpenAI (they require specific parameters)
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeOpus,
			ClockRate:    48000,
			Channels:     2,
			SDPFmtpLine:  "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return fmt.Errorf("failed to register Opus codec: %v", err)
	}
	
	// Create a new API specifically for OpenAI connection
	openAIAPI := webrtc.NewAPI(webrtc.WithMediaEngine(m))
	
	// Create a new peer connection with proper configuration
	// OpenAI acts as a passive ICE agent, so we need to be active
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	
	pc, err := openAIAPI.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("failed to create peer connection: %v", err)
	}
	c.peerConnection = pc
	
	// Log connection state changes
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("🔌 OpenAI connection state: %s", state.String())
	})
	
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("🧊 OpenAI ICE state: %s", state.String())
	})
	
	// Create a data channel for Realtime API communication FIRST
	// This ensures it's included in the offer
	dataChannel, err := pc.CreateDataChannel("oai-events", &webrtc.DataChannelInit{
		Ordered: func(b bool) *bool { return &b }(true),
	})
	if err != nil {
		return fmt.Errorf("failed to create data channel: %v", err)
	}
	
	// Create audio track for sending audio to OpenAI
	// This must be done BEFORE creating the transceiver
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeOpus,
			ClockRate:    48000,
			Channels:     2,
			SDPFmtpLine:  "minptime=10;useinbandfec=1",
		},
		"audio",
		"pion-to-openai",
	)
	if err != nil {
		return fmt.Errorf("failed to create audio track: %v", err)
	}
	
	// Add transceiver from track - this is the correct way for OpenAI
	// This combines track creation and transceiver setup
	transceiver, err := pc.AddTransceiverFromTrack(audioTrack, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv,
	})
	if err != nil {
		return fmt.Errorf("failed to add audio transceiver from track: %v", err)
	}
	
	log.Printf("✅ Added audio transceiver from track with direction: %v", transceiver.Direction())
	
	// Read incoming RTCP packets (required for audio to work properly)
	go func() {
		rtcpBuf := make([]byte, 1500)
		// Get the sender from the transceiver
		sender := transceiver.Sender()
		if sender == nil {
			log.Printf("⚠️ No sender available for RTCP reading")
			return
		}
		for {
			if _, _, rtcpErr := sender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()
	
	// Store the audio track for later use
	c.audioTrack = audioTrack
	
	// Handle incoming audio from OpenAI
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("🔊 Received audio track from OpenAI: %s (codec: %s)", track.ID(), track.Codec().MimeType)
		c.remoteAudioTrack = track
	})
	
	// Set up data channel handlers
	dataChannel.OnOpen(func() {
		log.Println("✅ OpenAI Realtime data channel opened")
		
		// Send initial configuration
		config := map[string]interface{}{
			"type": "session.update",
			"session": map[string]interface{}{
				"modalities":     []string{"text", "audio"},
				"instructions":   "You are a friendly German language tutor for beginners. Speak primarily in English but introduce German phrases one at a time. Always give the English meaning first, then the German phrase, followed by pronunciation help. Focus on basic greetings and introductions. Be encouraging and patient.",
				"voice":         "alloy",
				"input_audio_format":  "pcm16",
				"output_audio_format": "pcm16",
				"input_audio_transcription": map[string]interface{}{
					"model": "whisper-1",
				},
				"turn_detection": map[string]interface{}{
					"type": "server_vad",
					"threshold": 0.5,
					"prefix_padding_ms": 300,
					"silence_duration_ms": 200,
				},
				"temperature": 0.8,
				"max_response_output_tokens": 150,
			},
		}
		
		configJSON, _ := json.Marshal(config)
		if err := dataChannel.SendText(string(configJSON)); err != nil {
			log.Printf("❌ Failed to send config: %v", err)
		}
	})
	
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
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
			if session, ok := event["session"].(map[string]interface{}); ok {
				log.Printf("📋 Session details: %+v", session)
			}
			// Trigger a welcome message
			go func() {
				time.Sleep(500 * time.Millisecond)
				if err := c.TriggerResponse("Hello! I'm your German language tutor. Let's learn some German together! To say 'hello' in German, we say 'Hallo' - pronounced HAH-loh. Can you try saying Hallo?"); err != nil {
					log.Printf("❌ Failed to send welcome message: %v", err)
				}
			}()
		case "session.updated":
			log.Println("✅ Session updated")
		case "conversation.item.created":
			log.Println("📝 Conversation item created")
		case "response.audio.delta":
			// Audio data from OpenAI
			c.handleAudioDelta(event)
		case "response.audio_transcript.delta":
			// Transcript update
			c.handleTranscriptDelta(event)
		case "response.text.delta":
			// Text response
			if delta, ok := event["delta"].(string); ok {
				log.Printf("💬 Response: %s", delta)
			}
		case "response.done":
			log.Println("✅ Response complete")
		case "input_audio_buffer.speech_started":
			log.Println("🎤 Speech detected by OpenAI")
		case "input_audio_buffer.speech_stopped":
			log.Println("🔇 Speech ended")
		case "input_audio_buffer.committed":
			log.Println("📤 Audio buffer committed to OpenAI")
		case "error":
			log.Printf("❌ OpenAI error: %+v", event)
		default:
			log.Printf("📥 OpenAI event: %s", eventType)
		}
	})
	
	c.dataChannel = dataChannel
	
	// Create offer with proper options to ensure audio is included
	offer, err := pc.CreateOffer(&webrtc.OfferOptions{
		OfferAnswerOptions: webrtc.OfferAnswerOptions{
			VoiceActivityDetection: true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create offer: %v", err)
	}
	
	// Set local description
	if err := pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("failed to set local description: %v", err)
	}
	
	// Wait for ICE gathering to complete
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	
	select {
	case <-gatherComplete:
		log.Println("✅ ICE gathering complete for OpenAI connection")
	case <-time.After(3 * time.Second):
		log.Println("⏱️ ICE gathering timeout for OpenAI connection")
	}
	
	// Get the offer with candidates
	localDesc := pc.LocalDescription()
	if localDesc == nil {
		return fmt.Errorf("no local description available")
	}
	
	// Log the SDP to verify it has audio
	log.Printf("📄 SDP Offer to OpenAI (first 500 chars):\n%.500s", localDesc.SDP)
	if !strings.Contains(localDesc.SDP, "m=audio") {
		log.Printf("⚠️ WARNING: SDP does not contain audio media section!")
		log.Printf("📄 Full SDP:\n%s", localDesc.SDP)
	}
	
	// Send offer to OpenAI
	answer, err := c.sendOfferToOpenAI(localDesc.SDP)
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
	
	log.Printf("📤 Sending SDP offer to OpenAI (length: %d bytes)", len(offerSDP))
	log.Printf("📄 Full SDP Offer:\n%s", offerSDP)
	
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
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("OpenAI API error: %s - %s", resp.Status, string(answerSDP))
	}
	
	log.Printf("📥 Received SDP answer from OpenAI (status: %d, length: %d bytes)", resp.StatusCode, len(answerSDP))
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

// ForwardRTPToOpenAI forwards RTP packets from WhatsApp to OpenAI
func (c *OpenAIRealtimeClient) ForwardRTPToOpenAI(rtpPacket []byte) error {
	if c.audioTrack == nil {
		return fmt.Errorf("audio track not initialized")
	}
	
	// Write RTP packet directly to the track
	// This sends Opus-encoded audio from WhatsApp to OpenAI
	if _, err := c.audioTrack.Write(rtpPacket); err != nil {
		return fmt.Errorf("failed to write RTP packet: %v", err)
	}
	
	return nil
}

// GetRemoteAudioTrack returns the audio track from OpenAI
func (c *OpenAIRealtimeClient) GetRemoteAudioTrack() *webrtc.TrackRemote {
	return c.remoteAudioTrack
}

// TriggerResponse sends a response.create event to make OpenAI speak
func (c *OpenAIRealtimeClient) TriggerResponse(text string) error {
	if c.dataChannel == nil || c.dataChannel.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("data channel not open")
	}
	
	event := map[string]interface{}{
		"type": "response.create",
		"response": map[string]interface{}{
			"modalities": []string{"text", "audio"},
			"instructions": text,
		},
	}
	
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}
	
	log.Printf("📤 Triggering OpenAI response: %s", text)
	return c.dataChannel.SendText(string(eventJSON))
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