package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pion/webrtc/v3"
)

// OpenAIRealtimeClient handles the connection to OpenAI's Realtime API
type OpenAIRealtimeClient struct {
	apiKey           string
	azureEndpoint    string
	azureDeployment  string
	peerConnection   *webrtc.PeerConnection
	dataChannel      *webrtc.DataChannel
	ephemeralToken   string
	audioTrack       *webrtc.TrackLocalStaticRTP
	remoteAudioTrack *webrtc.TrackRemote
	phoneNumber      string
	reminderText     string // If this is a reminder call, what to remind about
}

// NewOpenAIRealtimeClient creates a new OpenAI Realtime client
func NewOpenAIRealtimeClient(apiKey, phoneNumber, reminderText string) *OpenAIRealtimeClient {
	// Check if using Azure OpenAI
	azureEndpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	azureDeployment := os.Getenv("AZURE_OPENAI_DEPLOYMENT")

	if azureEndpoint != "" {
		log.Printf("🔵 Using Azure OpenAI: %s", azureEndpoint)
	}

	if reminderText != "" {
		log.Printf("⏰ Creating OpenAI client for reminder call: %s", reminderText)
	}

	return &OpenAIRealtimeClient{
		apiKey:          apiKey,
		azureEndpoint:   azureEndpoint,
		azureDeployment: azureDeployment,
		phoneNumber:     phoneNumber,
		reminderText:    reminderText,
	}
}

// getInstructions returns the appropriate instructions based on whether this is a reminder call
func (c *OpenAIRealtimeClient) getInstructions() string {
	if c.reminderText != "" {
		// This is a reminder call - announce the reminder immediately
		return fmt.Sprintf("You are Ziggy, a helpful voice assistant. This is a reminder call. IMMEDIATELY when the call starts, announce the reminder: 'Hello! This is Ziggy calling to remind you about: %s' Then ask if they have completed this task or would like to reschedule. Speak ONLY in English. Be friendly and concise.", c.reminderText)
	}

	// Detect user's timezone from phone number to provide context
	timezone, err := GetTimezoneFromPhoneNumber(c.phoneNumber)
	if err != nil {
		timezone = "your local time" // Fallback if detection fails
	}

	// Get current date and time in user's timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC // Fallback to UTC
	}
	currentTime := time.Now().In(loc)
	currentDateTimeStr := currentTime.Format("Monday, January 2, 2006 at 3:04 PM MST")

	// Regular call - standard instructions with timezone awareness and current date/time
	return fmt.Sprintf("You are Ziggy, a helpful voice assistant for task management and reminders. IMMEDIATELY greet the caller when the call starts - say 'Hello! I'm Ziggy, your assistant. How can I help you today?' Speak ONLY in English. You can help with: 1) Task management - create, list, and update tasks, 2) Reminders - set reminders and I'll call you back at the specified time. IMPORTANT CONTEXT: The current date and time is %s. When setting reminders, convert user's time to format: YYYY-MM-DD HH:MM (24-hour format). The user is in timezone %s. Be friendly, concise, and proactive in your responses.", currentDateTimeStr, timezone)
}

// EphemeralTokenResponse represents the response from the ephemeral token endpoint (GA)
type EphemeralTokenResponse struct {
	Value     string `json:"value"`
	ExpiresAt int64  `json:"expires_at"`
}

// GetEphemeralToken fetches a temporary token for the Realtime API (GA interface)
func (c *OpenAIRealtimeClient) GetEphemeralToken() error {
	var url string

	// Use Azure endpoint if configured, otherwise use OpenAI
	if c.azureEndpoint != "" {
		// Azure requires ephemeral token from sessions endpoint
		log.Printf("🔵 Getting Azure OpenAI ephemeral token from sessions endpoint")
		sessionsURL := fmt.Sprintf("%s/openai/realtimeapi/sessions?api-version=2025-04-01-preview",
			c.azureEndpoint)
		log.Printf("📤 Sessions URL: %s", sessionsURL)
		log.Printf("🔑 API Key (first 20 chars): %.20s...", c.apiKey)

		reqBody := map[string]interface{}{
			"model": c.azureDeployment,
			"voice": "shimmer",
			"modalities": []string{"audio", "text"},
			"instructions": c.getInstructions(),
			"turn_detection": map[string]interface{}{
				"type":                "server_vad",
				"threshold":           0.5,
				"prefix_padding_ms":   100,
				"silence_duration_ms": 100,
			},
			"input_audio_transcription": map[string]interface{}{
				"model": "whisper-1",
			},
			"tools": []map[string]interface{}{
				{
					"type":        "function",
					"name":        "add_task",
					"description": "Create a new task for the caller. Use this when they ask to add, create, or remember a task or todo item.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"title": map[string]interface{}{
								"type":        "string",
								"description": "Brief title of the task",
							},
							"description": map[string]interface{}{
								"type":        "string",
								"description": "Detailed description of the task (optional)",
							},
							"priority": map[string]interface{}{
								"type":        "string",
								"description": "Priority level: low, medium, high, or urgent",
								"enum":        []string{"low", "medium", "high", "urgent"},
							},
						},
						"required": []string{"title"},
					},
				},
				{
					"type":        "function",
					"name":        "list_tasks",
					"description": "List all tasks for the caller. Can optionally filter by status.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"status": map[string]interface{}{
								"type":        "string",
								"description": "Filter by status: pending, in_progress, completed, or cancelled (optional)",
								"enum":        []string{"pending", "in_progress", "completed", "cancelled"},
							},
						},
					},
				},
				{
					"type":        "function",
					"name":        "update_task_status",
					"description": "Update the status of a task. Use when user wants to mark task as done, complete, in progress, etc.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"task_id": map[string]interface{}{
								"type":        "string",
								"description": "The ID of the task to update",
							},
							"status": map[string]interface{}{
								"type":        "string",
								"description": "New status: pending, in_progress, completed, or cancelled",
								"enum":        []string{"pending", "in_progress", "completed", "cancelled"},
							},
						},
						"required": []string{"task_id", "status"},
					},
				},
				{
					"type":        "function",
					"name":        "add_reminder",
					"description": "Set a reminder for the caller. Supports one-time and recurring reminders. When the reminder time comes, Ziggy will call them back.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"reminder_text": map[string]interface{}{
								"type":        "string",
								"description": "What to remind the user about",
							},
							"reminder_time": map[string]interface{}{
								"type":        "string",
								"description": "When to send the reminder in local timezone using format YYYY-MM-DD HH:MM (e.g., 2025-11-09 14:30 for 2:30 PM). Use 24-hour format. Ask the user for the exact date and time if not provided.",
							},
							"recurrence": map[string]interface{}{
								"type":        "string",
								"description": "Recurrence pattern: 'once' (default, one-time), 'daily', 'weekly', 'monthly', 'yearly'. Only specify if user wants recurring reminder.",
								"enum":        []string{"once", "daily", "weekly", "monthly", "yearly"},
							},
						},
						"required": []string{"reminder_text", "reminder_time"},
					},
				},
				{
					"type":        "function",
					"name":        "list_reminders",
					"description": "List all reminders for the caller. Can filter by status (pending, called, completed, cancelled).",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"status": map[string]interface{}{
								"type":        "string",
								"description": "Optional filter by status: 'pending', 'called', 'completed', 'cancelled'. If not provided, shows all reminders.",
								"enum":        []string{"pending", "called", "completed", "cancelled"},
							},
						},
					},
				},
				{
					"type":        "function",
					"name":        "cancel_reminder",
					"description": "Cancel a reminder. Use this when user wants to stop or delete a reminder.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"reminder_id": map[string]interface{}{
								"type":        "string",
								"description": "The ID of the reminder to cancel. Get this from list_reminders.",
							},
						},
						"required": []string{"reminder_id"},
					},
				},
			},
			"tool_choice": "auto",
			"temperature": 1.0,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}

		req, err := http.NewRequest("POST", sessionsURL, bytes.NewBuffer(jsonData))
		if err != nil {
			return err
		}

		req.Header.Set("api-key", c.apiKey) // Azure uses 'api-key' header
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

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			log.Printf("❌ Azure sessions API error. Status: %s, Body: %s", resp.Status, string(body))
			return fmt.Errorf("Azure sessions API error: %s - %s", resp.Status, string(body))
		}

		log.Printf("📥 Azure session response: %s", string(body))

		var sessionResp struct {
			ID           string `json:"id"`
			ClientSecret struct {
				Value string `json:"value"`
			} `json:"client_secret"`
		}

		if err := json.Unmarshal(body, &sessionResp); err != nil {
			log.Printf("❌ Failed to parse Azure session response: %v", err)
			return err
		}

		c.ephemeralToken = sessionResp.ClientSecret.Value
		log.Printf("✅ Got Azure ephemeral token for session: %s", sessionResp.ID)
		return nil
	}

	url = "https://api.openai.com/v1/realtime/client_secrets"

	reqBody := map[string]interface{}{
		"session": map[string]interface{}{
			"type": "realtime",
			"model": "gpt-realtime",
			"audio": map[string]interface{}{
				"output": map[string]interface{}{
					"voice": "shimmer",
				},
			},
		},
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
		log.Printf("❌ Failed to get ephemeral token. Status: %s, Body: %s", resp.Status, string(body))
		return fmt.Errorf("failed to get ephemeral token: %s - %s", resp.Status, string(body))
	}

	log.Printf("📥 Token response body: %s", string(body))

	var tokenResp EphemeralTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		log.Printf("❌ Failed to parse token response: %v, Body: %s", err, string(body))
		return err
	}

	if tokenResp.Value == "" {
		log.Printf("❌ Empty token value in response")
		return fmt.Errorf("empty token value received")
	}

	c.ephemeralToken = tokenResp.Value
	log.Printf("✅ Got ephemeral token (first 20 chars): %.20s..., expires at: %d", tokenResp.Value, tokenResp.ExpiresAt)

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
		
		// Send initial configuration (Azure format - flat structure)
		config := map[string]interface{}{
			"type": "session.update",
			"session": map[string]interface{}{
				"modalities": []string{"audio", "text"},
				"instructions": c.getInstructions(),
				"voice": "shimmer",
				"turn_detection": map[string]interface{}{
					"type": "server_vad",
					"threshold": 0.5,
					"prefix_padding_ms": 100,
					"silence_duration_ms": 100,
				},
				"input_audio_transcription": map[string]interface{}{
					"model": "whisper-1",
				},
				"tools": []map[string]interface{}{
					{
						"type": "function",
						"name": "add_task",
						"description": "Create a new task for the caller. Use this when they ask to add, create, or remember a task or todo item.",
						"parameters": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"title": map[string]interface{}{
									"type": "string",
									"description": "Brief title of the task",
								},
								"description": map[string]interface{}{
									"type": "string",
									"description": "Detailed description of the task (optional)",
								},
								"priority": map[string]interface{}{
									"type": "string",
									"description": "Priority level: low, medium, high, or urgent",
									"enum": []string{"low", "medium", "high", "urgent"},
								},
							},
							"required": []string{"title"},
						},
					},
					{
						"type": "function",
						"name": "list_tasks",
						"description": "List all tasks for the caller. Can optionally filter by status.",
						"parameters": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"status": map[string]interface{}{
									"type": "string",
									"description": "Filter by status: pending, in_progress, completed, or cancelled (optional)",
									"enum": []string{"pending", "in_progress", "completed", "cancelled"},
								},
							},
						},
					},
					{
						"type": "function",
						"name": "update_task_status",
						"description": "Update the status of a task. Use when user wants to mark task as done, complete, in progress, etc.",
						"parameters": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"task_id": map[string]interface{}{
									"type": "string",
									"description": "The ID of the task to update",
								},
								"status": map[string]interface{}{
									"type": "string",
									"description": "New status: pending, in_progress, completed, or cancelled",
									"enum": []string{"pending", "in_progress", "completed", "cancelled"},
								},
							},
							"required": []string{"task_id", "status"},
						},
					},
					{
						"type": "function",
						"name": "add_reminder",
						"description": "Set a reminder for the caller. Supports one-time and recurring reminders. When the reminder time comes, Ziggy will call them back.",
						"parameters": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"reminder_text": map[string]interface{}{
									"type": "string",
									"description": "What to remind the user about",
								},
								"reminder_time": map[string]interface{}{
									"type": "string",
									"description": "When to send the reminder in local timezone using format YYYY-MM-DD HH:MM (e.g., 2025-11-09 14:30 for 2:30 PM). Use 24-hour format. Ask the user for the exact date and time if not provided.",
								},
								"recurrence": map[string]interface{}{
									"type": "string",
									"description": "Recurrence pattern: 'once' (default, one-time), 'daily', 'weekly', 'monthly', 'yearly'. Only specify if user wants recurring reminder.",
									"enum": []string{"once", "daily", "weekly", "monthly", "yearly"},
								},
							},
							"required": []string{"reminder_text", "reminder_time"},
						},
					},
					{
						"type": "function",
						"name": "list_reminders",
						"description": "List all reminders for the caller. Can filter by status (pending, called, completed, cancelled).",
						"parameters": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"status": map[string]interface{}{
									"type": "string",
									"description": "Optional filter by status: 'pending', 'called', 'completed', 'cancelled'. If not provided, shows all reminders.",
									"enum": []string{"pending", "called", "completed", "cancelled"},
								},
							},
						},
					},
					{
						"type": "function",
						"name": "cancel_reminder",
						"description": "Cancel a reminder. Use this when user wants to stop or delete a reminder.",
						"parameters": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"reminder_id": map[string]interface{}{
									"type": "string",
									"description": "The ID of the reminder to cancel. Get this from list_reminders.",
								},
							},
							"required": []string{"reminder_id"},
						},
					},
				},
				"tool_choice": "auto",
				"temperature": 1.0,
			},
		}
		
		configJSON, _ := json.Marshal(config)
		log.Printf("📤 Sending session update config: %s", string(configJSON))
		if err := dataChannel.SendText(string(configJSON)); err != nil {
			log.Printf("❌ Failed to send config: %v", err)
		} else {
			log.Printf("✅ Session update config sent successfully")
		}
	})
	
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		// Parse the message
		var event map[string]interface{}
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Printf("❌ Failed to parse message: %v, Data: %s", err, string(msg.Data))
			return
		}

		// Handle different event types (GA interface)
		eventType, _ := event["type"].(string)
		switch eventType {
		case "session.created":
			log.Println("✅ Session created with OpenAI")
			if session, ok := event["session"].(map[string]interface{}); ok {
				log.Printf("📋 Session details: %+v", session)
			}
			// Trigger immediate greeting with NO delay
			greeting := map[string]interface{}{
				"type": "response.create",
			}
			greetingJSON, _ := json.Marshal(greeting)
			if err := c.dataChannel.SendText(string(greetingJSON)); err != nil {
				log.Printf("❌ Failed to send initial greeting: %v", err)
			} else {
				log.Println("🎙️ Triggered immediate greeting")
			}
		case "session.updated":
			log.Println("✅ Session updated")
			if session, ok := event["session"].(map[string]interface{}); ok {
				if instructions, ok := session["instructions"].(string); ok {
					log.Printf("📋 Active instructions: %s", instructions)
				}
				log.Printf("📋 Full session config: %+v", session)
			}
		case "conversation.item.created":
			log.Println("📝 Conversation item created")
		case "conversation.item.added":
			log.Println("📝 Conversation item added")
		case "conversation.item.done":
			log.Println("✅ Conversation item done")
		case "response.output_audio.delta":
			// Audio data from OpenAI (GA interface - new event name)
			if delta, ok := event["delta"].(string); ok {
				log.Printf("🔊 Received audio delta from OpenAI: %d bytes (base64)", len(delta))
			}
			c.handleAudioDelta(event)
		case "response.output_audio_transcript.delta":
			// Transcript update (GA interface - new event name)
			c.handleTranscriptDelta(event)
		case "response.output_text.delta":
			// Text response (GA interface - new event name)
			if delta, ok := event["delta"].(string); ok {
				log.Printf("💬 Response: %s", delta)
			}
		case "response.done":
			log.Println("✅ Response complete")
			// Log response details to debug why no audio
			if response, ok := event["response"].(map[string]interface{}); ok {
				log.Printf("📋 Response details: %+v", response)
			}
		case "input_audio_buffer.speech_started":
			log.Println("🎤 Speech detected by OpenAI")
		case "input_audio_buffer.speech_stopped":
			log.Println("🔇 Speech ended")
		case "input_audio_buffer.committed":
			log.Println("📤 Audio buffer committed to OpenAI")
		case "conversation.item.input_audio_transcription.completed":
			// Transcription succeeded (GA interface)
			if transcript, ok := event["transcript"].(string); ok {
				log.Printf("📝 Transcription: %s", transcript)
			}
		case "conversation.item.input_audio_transcription.failed":
			// Transcription failed - log detailed error
			log.Printf("❌ Transcription failed! Event details: %+v", event)
			if errorData, ok := event["error"].(map[string]interface{}); ok {
				log.Printf("❌ Error details: %+v", errorData)
				if code, ok := errorData["code"].(string); ok {
					log.Printf("❌ Error code: %s", code)
				}
				if message, ok := errorData["message"].(string); ok {
					log.Printf("❌ Error message: %s", message)
				}
			}
		case "response.function_call_arguments.done":
			// Function call completed
			c.handleFunctionCall(event)
		case "error":
			log.Printf("❌ OpenAI error event: %+v", event)
			if errorData, ok := event["error"].(map[string]interface{}); ok {
				log.Printf("❌ Error details: %+v", errorData)
			}
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

// sendOfferToOpenAI sends the WebRTC offer to OpenAI and gets the answer (GA interface)
func (c *OpenAIRealtimeClient) sendOfferToOpenAI(offerSDP string) (string, error) {
	if c.ephemeralToken == "" {
		return "", fmt.Errorf("no ephemeral token available")
	}

	var url string

	// Use Azure endpoint if configured
	if c.azureEndpoint != "" && c.azureDeployment != "" {
		// Azure WebRTC endpoint uses region-specific subdomain
		url = fmt.Sprintf("https://eastus2.realtimeapi-preview.ai.azure.com/v1/realtimertc?model=%s",
			c.azureDeployment)
		log.Printf("🔵 Using Azure OpenAI WebRTC endpoint: %s", url)
	} else {
		url = "https://api.openai.com/v1/realtime/calls"
	}

	log.Printf("📤 Sending SDP offer (length: %d bytes)", len(offerSDP))
	log.Printf("📄 Full SDP Offer:\n%s", offerSDP)

	req, err := http.NewRequest("POST", url, bytes.NewReader([]byte(offerSDP)))
	if err != nil {
		return "", err
	}

	// Both Azure and OpenAI use Bearer token for WebRTC endpoint
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
		log.Printf("❌ OpenAI SDP exchange failed. Status: %s, Response: %s", resp.Status, string(answerSDP))
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

// handleFunctionCall processes function call requests from OpenAI
func (c *OpenAIRealtimeClient) handleFunctionCall(event map[string]interface{}) {
	log.Printf("🔧 Function call event: %+v", event)
	
	// Extract function name and arguments
	functionName, _ := event["name"].(string)
	arguments, _ := event["arguments"].(string)
	callID, _ := event["call_id"].(string)
	
	log.Printf("📞 Function call: %s with args: %s", functionName, arguments)
	
	// Parse arguments
	var args map[string]interface{}
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			log.Printf("❌ Failed to parse function arguments: %v", err)
			return
		}
	}

	var resultJSON []byte

	switch functionName {
	case "add_task":
		title, _ := args["title"].(string)
		description, _ := args["description"].(string)
		priority, _ := args["priority"].(string)

		if priority == "" {
			priority = "medium"
		}

		log.Printf("📝 Adding task: %s (priority: %s)", title, priority)

		task, err := AddTask(title, description, priority, c.phoneNumber)
		if err != nil {
			log.Printf("❌ Failed to add task: %v", err)
			errorResult := map[string]string{
				"status": "error",
				"message": fmt.Sprintf("Failed to create task: %v", err),
			}
			resultJSON, _ = json.Marshal(errorResult)
		} else {
			resultJSON, _ = json.Marshal(map[string]interface{}{
				"status": "success",
				"message": fmt.Sprintf("Task '%s' created successfully", title),
				"task_id": task.ID,
				"title": task.Title,
			})
			log.Printf("✅ Task created: %s (ID: %s)", title, task.ID)
		}

	case "list_tasks":
		status, _ := args["status"].(string)
		log.Printf("📋 Listing tasks (status filter: %s)", status)

		tasks, err := ListTasks(c.phoneNumber, status)
		if err != nil {
			log.Printf("❌ Failed to list tasks: %v", err)
			errorResult := map[string]string{
				"status": "error",
				"message": fmt.Sprintf("Failed to list tasks: %v", err),
			}
			resultJSON, _ = json.Marshal(errorResult)
		} else {
			// Format tasks for the AI
			taskList := make([]map[string]interface{}, len(tasks))
			for i, task := range tasks {
				taskList[i] = map[string]interface{}{
					"id":          task.ID,
					"title":       task.Title,
					"description": task.Description,
					"status":      task.Status,
					"priority":    task.Priority,
				}
			}
			resultJSON, _ = json.Marshal(map[string]interface{}{
				"status": "success",
				"count":  len(tasks),
				"tasks":  taskList,
			})
			log.Printf("✅ Retrieved %d tasks", len(tasks))
		}

	case "update_task_status":
		taskID, _ := args["task_id"].(string)
		newStatus, _ := args["status"].(string)

		log.Printf("🔄 Updating task %s to status: %s", taskID, newStatus)

		err := UpdateTaskStatus(taskID, newStatus)
		if err != nil {
			log.Printf("❌ Failed to update task: %v", err)
			errorResult := map[string]string{
				"status": "error",
				"message": fmt.Sprintf("Failed to update task: %v", err),
			}
			resultJSON, _ = json.Marshal(errorResult)
		} else {
			resultJSON, _ = json.Marshal(map[string]interface{}{
				"status": "success",
				"message": fmt.Sprintf("Task updated to %s", newStatus),
				"task_id": taskID,
			})
			log.Printf("✅ Task %s updated to %s", taskID, newStatus)
		}

	case "add_reminder":
		reminderText, _ := args["reminder_text"].(string)
		reminderTime, _ := args["reminder_time"].(string)
		recurrence, _ := args["recurrence"].(string)
		if recurrence == "" {
			recurrence = "once"
		}

		log.Printf("⏰ Adding %s reminder: %s at %s", recurrence, reminderText, reminderTime)

		reminder, err := AddReminder(reminderText, reminderTime, c.phoneNumber, recurrence)
		if err != nil {
			log.Printf("❌ Failed to add reminder: %v", err)
			errorResult := map[string]string{
				"status": "error",
				"message": fmt.Sprintf("Failed to create reminder: %v", err),
			}
			resultJSON, _ = json.Marshal(errorResult)
		} else {
			var message string
			if recurrence == "once" {
				message = fmt.Sprintf("Reminder set for %s. I'll call you back at that time.", reminderTime)
			} else {
				message = fmt.Sprintf("%s reminder set for %s. I'll call you back %s.",
					recurrence, reminderTime, recurrence)
			}
			resultJSON, _ = json.Marshal(map[string]interface{}{
				"status": "success",
				"message": message,
				"reminder_id": reminder.ID,
				"reminder_text": reminder.ReminderText,
				"reminder_time": reminder.ReminderTime,
				"recurrence": reminder.RecurrencePattern,
			})
			log.Printf("✅ %s reminder created: %s at %s (ID: %s)", recurrence, reminderText, reminderTime, reminder.ID)
		}

	case "list_reminders":
		status, _ := args["status"].(string)

		log.Printf("📋 Listing reminders (status: %s)", status)

		reminders, err := ListReminders(c.phoneNumber, status)
		if err != nil {
			log.Printf("❌ Failed to list reminders: %v", err)
			errorResult := map[string]string{
				"status": "error",
				"message": fmt.Sprintf("Failed to list reminders: %v", err),
			}
			resultJSON, _ = json.Marshal(errorResult)
		} else {
			var reminderList []map[string]interface{}
			for _, r := range reminders {
				reminderList = append(reminderList, map[string]interface{}{
					"id":          r.ID,
					"text":        r.ReminderText,
					"time":        r.ReminderTime,
					"recurrence":  r.RecurrencePattern,
					"status":      r.Status,
				})
			}
			resultJSON, _ = json.Marshal(map[string]interface{}{
				"status": "success",
				"count":  len(reminders),
				"reminders": reminderList,
			})
			log.Printf("✅ Retrieved %d reminders", len(reminders))
		}

	case "cancel_reminder":
		reminderID, _ := args["reminder_id"].(string)

		log.Printf("🗑️ Cancelling reminder: %s", reminderID)

		err := CancelReminder(reminderID)
		if err != nil {
			log.Printf("❌ Failed to cancel reminder: %v", err)
			errorResult := map[string]string{
				"status": "error",
				"message": fmt.Sprintf("Failed to cancel reminder: %v", err),
			}
			resultJSON, _ = json.Marshal(errorResult)
		} else {
			resultJSON, _ = json.Marshal(map[string]interface{}{
				"status": "success",
				"message": "Reminder cancelled successfully",
				"reminder_id": reminderID,
			})
			log.Printf("✅ Reminder %s cancelled", reminderID)
		}

	default:
		log.Printf("⚠️ Unknown function: %s", functionName)
		errorResult := map[string]string{
			"status": "error",
			"message": fmt.Sprintf("Unknown function: %s", functionName),
		}
		resultJSON, _ = json.Marshal(errorResult)
	}

	// Send function result back
	functionOutput := map[string]interface{}{
		"type": "conversation.item.create",
		"item": map[string]interface{}{
			"type":    "function_call_output",
			"call_id": callID,
			"output":  string(resultJSON),
		},
	}

	outputJSON, _ := json.Marshal(functionOutput)
	if err := c.dataChannel.SendText(string(outputJSON)); err != nil {
		log.Printf("❌ Failed to send function output: %v", err)
		return
	}

	// Trigger model to respond
	responseCreate := map[string]interface{}{
		"type": "response.create",
	}

	responseJSON, _ := json.Marshal(responseCreate)
	if err := c.dataChannel.SendText(string(responseJSON)); err != nil {
		log.Printf("❌ Failed to trigger response: %v", err)
	} else {
		log.Printf("🎙️ Triggered model response")
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