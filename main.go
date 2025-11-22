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
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/pion/webrtc/v4"
)

const (
	// Default webhook configuration (overridden by env vars)
	DEFAULT_VERIFY_TOKEN = "whatsapp_bridge_token"
)

// WhatsAppBridge handles WhatsApp call bridging using Pion WebRTC
type WhatsAppBridge struct {
	api                 *webrtc.API
	config              webrtc.Configuration
	activeCalls         map[string]*Call
	mu                  sync.Mutex
	verifyToken         string
	accessToken         string
	phoneNumberID       string
	allowedPhoneNumber  string // Only process webhooks from this display phone number
}

// Call represents an active WhatsApp call session
type Call struct {
	ID             string
	PeerConnection *webrtc.PeerConnection
	AudioTrack     *webrtc.TrackLocalStaticRTP
	StartTime      time.Time
	OpenAIClient   *OpenAIRealtimeClient
	ReminderID     string // If this is a reminder call
	ReminderText   string // What to remind the user about
}

// NewWhatsAppBridge creates a new bridge instance
func NewWhatsAppBridge() *WhatsAppBridge {
	// Create a MediaEngine with audio codecs
	m := &webrtc.MediaEngine{}
	
	// First register the Opus codec with WhatsApp's EXACT parameters
	opusCodec := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			// Use WhatsApp's exact fmtp line from their SDP
			SDPFmtpLine: "maxaveragebitrate=20000;maxplaybackrate=16000;minptime=20;sprop-maxcapturerate=16000;useinbandfec=1",
			RTCPFeedback: []webrtc.RTCPFeedback{
				{Type: "transport-cc"},
			},
		},
		PayloadType: 111,
	}
	if err := m.RegisterCodec(opusCodec, webrtc.RTPCodecTypeAudio); err != nil {
		log.Fatal("Failed to register Opus codec:", err)
	}
	
	// Register telephone-event for DTMF with WhatsApp's payload type
	telephoneEvent := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  "audio/telephone-event",
			ClockRate: 8000,
		},
		PayloadType: 126,
	}
	if err := m.RegisterCodec(telephoneEvent, webrtc.RTPCodecTypeAudio); err != nil {
		log.Fatal("Failed to register telephone-event codec:", err)
	}
	
	// Register the RTP header extensions that WhatsApp uses
	// These MUST be registered to parse the SDP correctly
	extensions := []string{
		"urn:ietf:params:rtp-hdrext:ssrc-audio-level",
		"http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time",
		"http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01",
	}
	
	for _, uri := range extensions {
		ext := webrtc.RTPHeaderExtensionCapability{URI: uri}
		if err := m.RegisterHeaderExtension(ext, webrtc.RTPCodecTypeAudio); err != nil {
			log.Printf("Warning: Failed to register extension %s: %v", uri, err)
		}
	}
	
	// Create a SettingEngine
	s := webrtc.SettingEngine{}
	// Don't set ice-lite mode - WhatsApp uses ice-lite in offer, but we shouldn't in answer
	s.SetAnsweringDTLSRole(webrtc.DTLSRoleClient) // We should be active/client when WhatsApp is actpass
	
	// Configure network types - disable TCP since WhatsApp uses UDP only
	s.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeUDP6,
	})
	
	// Create the API with our custom engines
	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(m),
		webrtc.WithSettingEngine(s),
	)
	
	// Configure ICE servers - we need STUN since we're not ice-lite
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	
	verifyToken := os.Getenv("VERIFY_TOKEN")
	if verifyToken == "" {
		verifyToken = DEFAULT_VERIFY_TOKEN
	}
	
	accessToken := os.Getenv("WHATSAPP_TOKEN")
	if accessToken == "" {
		log.Println("⚠️  WHATSAPP_TOKEN not set - API calls will fail")
	}
	
	phoneNumberID := os.Getenv("PHONE_NUMBER_ID")
	if phoneNumberID == "" {
		log.Println("⚠️  PHONE_NUMBER_ID not set - API calls will fail")
	}

	// Get allowed phone number for security filtering (only process webhooks from this number)
	allowedPhoneNumber := os.Getenv("ALLOWED_DISPLAY_PHONE_NUMBER")
	if allowedPhoneNumber == "" {
		allowedPhoneNumber = "917306356514" // Default to your phone number
		log.Printf("ℹ️  ALLOWED_DISPLAY_PHONE_NUMBER not set, defaulting to: %s", allowedPhoneNumber)
	} else {
		log.Printf("🔒 Only processing webhooks from display phone number: %s", allowedPhoneNumber)
	}

	return &WhatsAppBridge{
		api:                api,
		config:             config,
		activeCalls:        make(map[string]*Call),
		verifyToken:        verifyToken,
		accessToken:        accessToken,
		phoneNumberID:      phoneNumberID,
		allowedPhoneNumber: allowedPhoneNumber,
	}
}

// Start begins the HTTP server
func (b *WhatsAppBridge) Start() {
	router := mux.NewRouter()
	
	// Add logging middleware
	router.Use(b.loggingMiddleware)
	
	// WhatsApp webhook endpoints
	router.HandleFunc("/whatsapp-call", b.handleWebhookVerification).Methods("GET")
	router.HandleFunc("/whatsapp-call", b.handleWebhookEvent).Methods("POST")
	
	// Test endpoints
	router.HandleFunc("/test-call", b.handleTestCall).Methods("POST")
	router.HandleFunc("/status", b.handleStatus).Methods("GET")
	router.HandleFunc("/health", b.handleHealth).Methods("GET")

	// Outbound call endpoint
	router.HandleFunc("/initiate-call", b.handleInitiateCall).Methods("POST")

	// Call permission request endpoint
	router.HandleFunc("/request-call-permission", b.handleRequestCallPermission).Methods("POST")

	// Reminders cron endpoint - called by Supabase cron job
	router.HandleFunc("/check-reminders", b.handleCheckReminders).Methods("POST", "GET")

	// Get port from environment or default
	port := os.Getenv("PORT")
	if port == "" {
		port = "3011"
	}
	
	// Start server
	log.Printf("🚀 Pion WhatsApp Bridge starting on port %s", port)
	log.Printf("📡 Webhook endpoint: /whatsapp-call")
	log.Printf("🧪 Test endpoint: /test-call")
	log.Printf("📊 Status endpoint: /status")
	log.Printf("🔐 Verify token configured: %v", b.verifyToken != "")
	log.Printf("🔑 Access token configured: %v", b.accessToken != "")
	log.Printf("📱 Phone number ID: %s", b.phoneNumberID)
	log.Printf("🔊 Echo mode: %v", os.Getenv("ENABLE_ECHO") == "true")
	
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatal(err)
	}
}

// loggingMiddleware logs all incoming requests
func (b *WhatsAppBridge) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("🌐 %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		log.Printf("📋 Headers: %v", r.Header)
		next.ServeHTTP(w, r)
	})
}

// handleWebhookVerification handles WhatsApp webhook verification
func (b *WhatsAppBridge) handleWebhookVerification(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")
	
	if mode == "subscribe" && token == b.verifyToken {
		log.Println("✅ WhatsApp webhook verified")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(challenge))
		return
	}
	
	log.Println("❌ WhatsApp webhook verification failed")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(map[string]string{"error": "Verification failed"})
}

// handleWebhookEvent handles incoming WhatsApp webhook events
func (b *WhatsAppBridge) handleWebhookEvent(w http.ResponseWriter, r *http.Request) {
	log.Println("📨 POST /whatsapp-call webhook received")
	
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("❌ Failed to read body: %v", err)
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	
	log.Printf("📦 Raw webhook body: %s", string(body))
	
	// WhatsApp Calling API doesn't require signature verification
	// Just log if signature header is present
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature != "" {
		log.Printf("🔏 Signature header present but not required for WhatsApp Calling API")
	}
	
	// Parse webhook data
	var webhook map[string]interface{}
	if err := json.Unmarshal(body, &webhook); err != nil {
		log.Printf("❌ Failed to parse JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	// Log parsed webhook structure
	prettyJSON, _ := json.MarshalIndent(webhook, "", "  ")
	log.Printf("📱 WhatsApp webhook parsed:\n%s", string(prettyJSON))
	
	// Process the webhook asynchronously to return 200 OK immediately
	go b.processWebhook(webhook)
	
	// WhatsApp expects a 200 OK response immediately
	w.WriteHeader(http.StatusOK)
	log.Printf("✉️ Sent 200 OK response to WhatsApp")
}


// processWebhook processes incoming webhook data
func (b *WhatsAppBridge) processWebhook(webhook map[string]interface{}) {
	log.Println("🔍 Processing webhook data...")

	// Log the complete raw webhook for debugging
	webhookJSON, _ := json.MarshalIndent(webhook, "", "  ")
	log.Printf("📦 COMPLETE WEBHOOK BODY:\n%s\n", string(webhookJSON))

	// Parse WhatsApp webhook structure
	entry, ok := webhook["entry"].([]interface{})
	if !ok || len(entry) == 0 {
		log.Println("⚠️ No entry found in webhook")
		return
	}

	log.Printf("📊 Found %d entries", len(entry))
	
	// Get first entry
	firstEntry, ok := entry[0].(map[string]interface{})
	if !ok {
		log.Println("⚠️ Invalid entry format")
		return
	}
	
	// Get changes
	changes, ok := firstEntry["changes"].([]interface{})
	if !ok || len(changes) == 0 {
		log.Println("⚠️ No changes found in entry")
		return
	}
	
	log.Printf("📊 Found %d changes", len(changes))
	
	// Process each change
	for i, change := range changes {
		log.Printf("🔄 Processing change %d", i+1)
		
		changeData, ok := change.(map[string]interface{})
		if !ok {
			log.Printf("⚠️ Invalid change format at index %d", i)
			continue
		}
		
		// Get the value directly - WhatsApp webhook structure has the field type inside value
		if value, ok := changeData["value"].(map[string]interface{}); ok {
			// Security check: Only process webhooks from our specific phone number
			if metadata, ok := value["metadata"].(map[string]interface{}); ok {
				displayPhoneNumber, _ := metadata["display_phone_number"].(string)
				allowedPhoneNumber := "917306356514"

				if displayPhoneNumber != allowedPhoneNumber {
					log.Printf("🚫 Ignoring webhook from unauthorized phone number: %s (expected: %s)",
						displayPhoneNumber, allowedPhoneNumber)
					return
				}
				log.Printf("✅ Phone number verified: %s", displayPhoneNumber)
			} else {
				log.Printf("⚠️ No metadata found in webhook value, skipping security check")
			}

			// Check for event_type field (used for outbound calls)
			if eventType, ok := value["event_type"].(string); ok {
				log.Printf("📞 Found event_type: %s", eventType)
				if eventType == "call.connect" {
					log.Printf("📞 Processing call.connect event (outbound call answer)")
					// This is the webhook with SDP answer for outbound calls
					callID, _ := value["call_id"].(string)
					from, _ := value["from"].(string)

					if session, ok := value["session"].(map[string]interface{}); ok {
						sdpAnswer, _ := session["sdp"].(string)
						sdpType, _ := session["sdp_type"].(string)

						log.Printf("🔍 Session data: sdp_type=%s, sdp_length=%d", sdpType, len(sdpAnswer))

						if sdpType == "answer" && sdpAnswer != "" {
							log.Printf("📥 Received SDP answer for outbound call %s", callID)
							log.Printf("📄 SDP Answer:\n%s", sdpAnswer)
							// Process the answer asynchronously
							go b.handleOutboundCallAnswer(callID, sdpAnswer, from)
						}
					}
				}
			}

			// Check for calls array directly in value (used for inbound calls)
			if calls, ok := value["calls"].([]interface{}); ok && len(calls) > 0 {
				log.Printf("📞 Found %d call events", len(calls))
				for _, call := range calls {
					if callData, ok := call.(map[string]interface{}); ok {
						b.handleCallEvent(callData)
					}
				}
			} else if messages, ok := value["messages"].([]interface{}); ok && len(messages) > 0 {
				log.Printf("💬 Found %d message events", len(messages))
				// Handle messages using the messaging SDK
				go b.handleMessageEvents(webhook)
			} else if statuses, ok := value["statuses"].([]interface{}); ok && len(statuses) > 0 {
				log.Printf("📊 Found %d status events", len(statuses))
				// Check if any are call statuses
				for _, status := range statuses {
					if statusData, ok := status.(map[string]interface{}); ok {
						if statusType, ok := statusData["type"].(string); ok && statusType == "call" {
							log.Printf("📞 Found call status event")
							// Log full status data for outbound calls
							statusJSON, _ := json.MarshalIndent(statusData, "", "  ")
							log.Printf("📋 Full status data:\n%s", string(statusJSON))

							// When we get ACCEPTED, we should expect a connect webhook next
							if callStatus, ok := statusData["status"].(string); ok && callStatus == "ACCEPTED" {
								log.Printf("✅ Call ACCEPTED by user - waiting for connect webhook with SDP answer...")
							}
						}
					}
				}
			} else {
				// Log all fields present in value for debugging
				log.Printf("📋 Value contains fields: %v", func() []string {
					var fields []string
					for k := range value {
						fields = append(fields, k)
					}
					return fields
				}())
			}
		}
		
		// Also check for field attribute (some webhooks have it)
		if field, ok := changeData["field"].(string); ok {
			log.Printf("📌 Field attribute: %s", field)
		}
	}
}

// handleCallEvent processes individual call events from webhooks
func (b *WhatsAppBridge) handleCallEvent(callData map[string]interface{}) {
	// Extract call information
	callID, _ := callData["id"].(string)
	event, _ := callData["event"].(string)
	direction, _ := callData["direction"].(string)
	from, _ := callData["from"].(string)
	to, _ := callData["to"].(string)

	log.Printf("📞 Call event: %s (ID: %s, Direction: %s, From: %s, To: %s)", event, callID, direction, from, to)

	// For debugging, log all fields in the call data
	log.Printf("🔍 Call data fields: %v", func() []string {
		var fields []string
		for k := range callData {
			fields = append(fields, k)
		}
		return fields
	}())
	
	// Log additional call data for debugging
	if status, ok := callData["status"].(string); ok {
		log.Printf("📞 Call status: %s", status)

		// If status is FAILED, log the errors
		if status == "FAILED" {
			if errors, ok := callData["errors"].([]interface{}); ok && len(errors) > 0 {
				log.Printf("❌ Call FAILED with %d errors:", len(errors))
				for i, errObj := range errors {
					if errData, ok := errObj.(map[string]interface{}); ok {
						errJSON, _ := json.MarshalIndent(errData, "  ", "  ")
						log.Printf("❌ Error %d:\n  %s", i+1, string(errJSON))
					}
				}
			} else {
				log.Printf("❌ Call FAILED but no error details available")
			}
		}
	}
	if timestamp, ok := callData["timestamp"].(string); ok {
		log.Printf("📞 Call timestamp: %s", timestamp)
	}
	
	switch event {
	case "connect":
		// Handle incoming call with SDP offer
		if direction == "USER_INITIATED" {
			// Extract SDP from session
			if session, ok := callData["session"].(map[string]interface{}); ok {
				sdpOffer, _ := session["sdp"].(string)
				sdpType, _ := session["sdp_type"].(string)

				if sdpType == "offer" && sdpOffer != "" {
					log.Printf("📥 Received SDP offer for inbound call %s", callID)
					// Process the call asynchronously
					go b.acceptIncomingCall(callID, sdpOffer, from)
				}
			}
		} else if direction == "BUSINESS_INITIATED" {
			// Handle outbound call - user answered with SDP answer
			log.Printf("📥 User answered outbound call %s", callID)
			if session, ok := callData["session"].(map[string]interface{}); ok {
				sdpAnswer, _ := session["sdp"].(string)
				sdpType, _ := session["sdp_type"].(string)

				log.Printf("🔍 Session data: sdp_type=%s, sdp_length=%d", sdpType, len(sdpAnswer))

				if sdpType == "answer" && sdpAnswer != "" {
					log.Printf("📥 Received SDP answer for outbound call %s", callID)
					log.Printf("📄 SDP Answer:\n%s", sdpAnswer)
					// Process the answer asynchronously
					go b.handleOutboundCallAnswer(callID, sdpAnswer, from)
				} else {
					log.Printf("⚠️ Invalid or missing SDP answer: type=%s, present=%v", sdpType, sdpAnswer != "")
				}
			} else {
				log.Printf("⚠️ No session data in connect event for outbound call")
			}
		}
		
	case "terminate":
		// Handle call termination
		b.mu.Lock()
		if call, exists := b.activeCalls[callID]; exists {
			// Close WebRTC connection
			if call.PeerConnection != nil {
				call.PeerConnection.Close()
			}
			// Close OpenAI connection
			if call.OpenAIClient != nil {
				call.OpenAIClient.Close()
				log.Printf("🤖 Closed OpenAI connection for call %s", callID)
			}
			// Log call duration
			callDuration := time.Since(call.StartTime)
			log.Printf("📊 Call %s lasted %v", callID, callDuration)
			
			delete(b.activeCalls, callID)
			log.Printf("☎️ Call terminated and cleaned up: %s", callID)
		} else {
			log.Printf("☎️ Terminate event for unknown call: %s", callID)
		}
		b.mu.Unlock()
		
	case "ringing":
		log.Printf("🔔 Call ringing: %s", callID)
		// WhatsApp is notifying us that the call is ringing
		// We don't need to do anything here, just log it
		
	case "answered":
		log.Printf("📞 Call answered: %s", callID)
		// The call was answered (might be on another device)
		
	default:
		log.Printf("📋 Unhandled call event: %s", event)
		// Log the entire call data for unknown events
		callJSON, _ := json.MarshalIndent(callData, "", "  ")
		log.Printf("📋 Full call data:\n%s", string(callJSON))
	}
}

// handleMessageEvents processes incoming WhatsApp messages
func (b *WhatsAppBridge) handleMessageEvents(webhook map[string]interface{}) {
	log.Println("💬 Processing message events...")

	// Initialize messaging SDK
	wa := NewWhatsAppSDK("", "")
	handler := wa.WebhookHandler()

	// Convert webhook to JSON bytes for parsing
	webhookJSON, err := json.Marshal(webhook)
	if err != nil {
		log.Printf("❌ Failed to marshal webhook: %v", err)
		return
	}

	// Parse webhook data
	if err := handler.Parse(webhookJSON); err != nil {
		log.Printf("❌ Failed to parse message webhook: %v", err)
		return
	}

	// Check for duplicate messages
	if handler.IsDuplicate() {
		log.Printf("🔁 Duplicate message ignored")
		return
	}

	// Get message details
	sender := handler.Sender()
	text := handler.Text()
	msgType := handler.MessageType()
	contactName := handler.ContactName()

	log.Printf("📨 Message from %s (%s): Type=%s, Text=%s", contactName, sender, msgType, text)

	// Handle different message types
	switch msgType {
	case "text":
		b.handleTextMessage(handler, text, sender)
	case "interactive":
		b.handleInteractiveMessage(handler, text, sender)
	case "audio":
		b.handleAudioMessage(handler, sender)
	case "image":
		b.handleImageMessage(handler, sender)
	case "video":
		b.handleVideoMessage(handler, sender)
	default:
		log.Printf("⚠️ Unknown message type: %s", msgType)
	}
}

// handleTextMessage handles incoming text messages using LLM
func (b *WhatsAppBridge) handleTextMessage(handler *WebhookHandler, text, sender string) {
	log.Printf("💬 Handling text message: %s", text)

	// Create LLM handler for this user
	llmHandler := NewLLMTextHandler(sender)

	// Save incoming message to Supabase
	messageID := handler.MessageID()
	contactName := handler.ContactName()
	if err := llmHandler.SaveMessage(text, "inbound", messageID, contactName); err != nil {
		log.Printf("⚠️ Failed to save incoming message: %v", err)
	}

	// Get AI response
	aiResponse, err := llmHandler.GetAIResponse(text)
	if err != nil {
		log.Printf("❌ Failed to get AI response: %v", err)
		// Fallback response if AI fails
		aiResponse = "I'm having trouble thinking right now. Can you try again? 🤔"
	}

	// Send response
	if _, err := handler.ReplyText(aiResponse); err != nil {
		log.Printf("❌ Failed to send response: %v", err)
		return
	}

	// Save outbound message to Supabase
	if err := llmHandler.SaveMessage(aiResponse, "outbound", "", contactName); err != nil {
		log.Printf("⚠️ Failed to save outbound message: %v", err)
	}

	log.Printf("✅ AI conversation completed for %s", sender)
}

// handleInteractiveMessage handles button/list replies
func (b *WhatsAppBridge) handleInteractiveMessage(handler *WebhookHandler, selection, sender string) {
	log.Printf("🔘 User selected: %s", selection)

	switch selection {
	case "Call Me":
		handler.ReplyText("📞 Initiating voice call...")
		// Initiate an outbound call
		go func() {
			callID, err := b.initiateWhatsAppCall(sender, "")
			if err != nil {
				log.Printf("❌ Failed to initiate call: %v", err)
			} else {
				log.Printf("✅ Initiated call: %s", callID)
			}
		}()

	case "Check Status":
		b.handleTextMessage(handler, "status", sender)

	case "Help":
		b.handleTextMessage(handler, "help", sender)

	case "approve_call_permission":
		// User approved call permission
		log.Printf("✅ User %s approved call permission", sender)
		if err := ApproveCallPermission(sender, "express_request"); err != nil {
			log.Printf("❌ Failed to approve call permission: %v", err)
			handler.ReplyText("❌ Sorry, there was an error processing your response. Please try again.")
		} else {
			handler.ReplyText("✅ Thank you! You've granted permission for us to call you. We can now contact you by phone when needed. This permission is valid for 72 hours.")
		}

	case "deny_call_permission":
		// User denied call permission
		log.Printf("🚫 User %s denied call permission", sender)
		handler.ReplyText("👍 No problem! We won't call you. You can change your mind anytime by typing 'allow calls'.")

	default:
		// Unknown button selection - just log it
		log.Printf("⚠️ Unknown button selection: %s", selection)
	}
}

// handleAudioMessage handles incoming audio messages with transcription
func (b *WhatsAppBridge) handleAudioMessage(handler *WebhookHandler, sender string) {
	audioID := handler.AudioID()
	if audioID == "" {
		log.Printf("⚠️ No audio ID found in message")
		return
	}

	log.Printf("🎤 Received audio message: %s from %s", audioID, sender)

	// Get WhatsApp credentials
	token := os.Getenv("WHATSAPP_TOKEN")
	phoneNumberID := os.Getenv("PHONE_NUMBER_ID")

	// Download the audio file
	audioFilePath, err := DownloadAudio(audioID, phoneNumberID, token)
	if err != nil {
		log.Printf("❌ Error downloading audio: %v", err)
		handler.ReplyText("Sorry, I couldn't download your audio message. 🤔")
		return
	}

	// Ensure cleanup
	defer CleanupAudioFile(audioFilePath)

	// Transcribe the audio
	transcription, err := TranscribeAudio(audioFilePath)
	if err != nil {
		log.Printf("❌ Error transcribing audio: %v", err)
		handler.ReplyText("Sorry, I couldn't understand your audio message. Can you try again? 🎤")
		return
	}

	if transcription == "" {
		log.Printf("⚠️ Empty transcription result")
		handler.ReplyText("I couldn't hear anything in your audio. Can you try again? 🎤")
		return
	}

	log.Printf("✅ Transcribed audio: %s", transcription)

	// Get message metadata
	messageID := handler.MessageID()
	contactName := handler.ContactName()

	// Create LLM handler for this user
	llmHandler := NewLLMTextHandler(sender)

	// Save the transcribed message with [Voice] prefix
	voiceMessage := fmt.Sprintf("[Voice]: %s", transcription)
	if err := llmHandler.SaveMessage(voiceMessage, "inbound", messageID, contactName); err != nil {
		log.Printf("⚠️ Failed to save voice message: %v", err)
	}

	// Get AI response for the transcribed text
	aiResponse, err := llmHandler.GetAIResponse(transcription)
	if err != nil {
		log.Printf("❌ Failed to get AI response: %v", err)
		handler.ReplyText("I'm having trouble thinking right now. Can you try again? 🤔")
		return
	}

	// Send the AI response
	if _, err := handler.ReplyText(aiResponse); err != nil {
		log.Printf("❌ Failed to send response: %v", err)
		return
	}

	// Save the outbound response
	if err := llmHandler.SaveMessage(aiResponse, "outbound", "", contactName); err != nil {
		log.Printf("⚠️ Failed to save outbound message: %v", err)
	}

	log.Printf("✅ Voice message processed and response sent")
}

// handleImageMessage handles incoming image messages
func (b *WhatsAppBridge) handleImageMessage(handler *WebhookHandler, sender string) {
	imageID := handler.ImageID()
	if imageID == "" {
		log.Printf("⚠️ No image ID found in message")
		return
	}

	log.Printf("🖼️ Received image message: %s from %s", imageID, sender)

	// Download the image file
	filename := "image_" + imageID + ".jpg"
	savedPath, err := handler.client.DownloadMedia(imageID, filename)
	if err != nil {
		log.Printf("❌ Error downloading image: %v", err)
		handler.ReplyText("Sorry, I couldn't process your image.")
		return
	}

	log.Printf("✅ Image saved to: %s", savedPath)
	handler.ReplyText("📸 Great image! I've received it.")

	// Here you could:
	// 1. Use GPT-4 Vision to analyze the image
	// 2. Extract text using OCR
	// 3. Perform image recognition
}

// handleVideoMessage handles incoming video messages
func (b *WhatsAppBridge) handleVideoMessage(handler *WebhookHandler, sender string) {
	videoID := handler.VideoID()
	if videoID == "" {
		log.Printf("⚠️ No video ID found in message")
		return
	}

	log.Printf("🎥 Received video message: %s from %s", videoID, sender)

	// Download the video file
	filename := "video_" + videoID + ".mp4"
	savedPath, err := handler.client.DownloadMedia(videoID, filename)
	if err != nil {
		log.Printf("❌ Error downloading video: %v", err)
		handler.ReplyText("Sorry, I couldn't process your video.")
		return
	}

	log.Printf("✅ Video saved to: %s", savedPath)
	handler.ReplyText("🎬 Thanks for the video! I've received it.")
}

// acceptIncomingCall handles accepting an incoming WhatsApp call
func (b *WhatsAppBridge) acceptIncomingCall(callID, sdpOffer, callerNumber string) {
	log.Printf("🔔 Processing incoming call %s from %s", callID, callerNumber)
	log.Printf("📋 Call flow: 1) Create PeerConnection → 2) Set SDP → 3) Pre-accept → 4) Accept → 5) Media flow")

	// Grant call permission automatically - user calling us grants implicit permission for callbacks
	if err := GrantCallPermission(callerNumber); err != nil {
		log.Printf("⚠️ Failed to grant call permission for %s: %v", callerNumber, err)
		// Continue anyway - permission tracking is not critical for call handling
	}

	// Check if call is already being processed
	b.mu.Lock()
	if _, exists := b.activeCalls[callID]; exists {
		b.mu.Unlock()
		log.Printf("⚠️ Call %s already being processed, ignoring duplicate", callID)
		return
	}
	// Reserve this call ID immediately to prevent race conditions
	b.activeCalls[callID] = &Call{
		ID:        callID,
		StartTime: time.Now(),
	}
	b.mu.Unlock()
	
	// Create a new PeerConnection
	pc, err := b.api.NewPeerConnection(b.config)
	if err != nil {
		log.Printf("❌ Failed to create peer connection: %v", err)
		b.mu.Lock()
		delete(b.activeCalls, callID)
		b.mu.Unlock()
		return
	}
	
	// Set up ICE connection channel
	iceConnected := make(chan bool, 1)
	
	// Set up handlers
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State for call %s: %s", callID, state.String())
		if state == webrtc.ICEConnectionStateConnected || state == webrtc.ICEConnectionStateCompleted {
			select {
			case iceConnected <- true:
			default:
			}
		}
		// v4: Handle explicit DTLS close (instant disconnect detection)
		if state == webrtc.ICEConnectionStateClosed {
			log.Printf("🔴 Call %s: ICE connection explicitly closed via DTLS", callID)
			// Connection closed gracefully - cleanup will happen in terminate handler
		}
	})
	
	pc.OnICEGatheringStateChange(func(state webrtc.ICEGatheringState) {
		log.Printf("ICE Gathering State for call %s: %s", callID, state.String())
	})
	
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			log.Printf("ICE Candidate for call %s: %s", callID, candidate.String())
		}
	})
	
	// We'll create and add the audio track AFTER setting remote description
	
	// Create the call object early
	call := &Call{
		ID:             callID,
		PeerConnection: pc,
		StartTime:      time.Now(),
	}
	
	// Store the call early so we can access it in OnTrack
	b.mu.Lock()
	b.activeCalls[callID] = call
	b.mu.Unlock()
	
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("🔊 Received audio track for call %s: %s (codec: %s)", callID, track.ID(), track.Codec().MimeType)
		log.Printf("📊 Track details: PayloadType=%d, SSRC=%d, Kind=%s", track.PayloadType(), track.SSRC(), track.Kind().String())
		
		// Verify call exists
		b.mu.Lock()
		_, exists := b.activeCalls[callID]
		b.mu.Unlock()
		
		if !exists {
			log.Printf("❌ Call %s not found in active calls", callID)
			return
		}
		
		// Start reading audio packets and dynamically check for OpenAI client
		go func() {
			packetCount := 0
			totalBytes := 0
			openAIForwardingStarted := false

			for {
				// v4 FIX: Use ReadRTP() to access full packet with headers
				rtpPacket, _, readErr := track.ReadRTP()
				if readErr != nil {
					log.Printf("❌ Error reading audio after %d packets: %v", packetCount, readErr)
					return
				}

				// v4 FIX: Clear extension headers to avoid conflicts between WhatsApp and OpenAI
				// Different endpoints use different extension header IDs, causing audio corruption
				rtpPacket.Extension = false
				rtpPacket.Extensions = nil

				// Marshal back to bytes for forwarding
				rtpBytes, marshalErr := rtpPacket.Marshal()
				if marshalErr != nil {
					log.Printf("❌ Error marshaling RTP packet: %v", marshalErr)
					continue
				}

				packetCount++
				totalBytes += len(rtpBytes)

				// Check for OpenAI client on every packet (it might become available later)
				b.mu.Lock()
				activeCall, exists := b.activeCalls[callID]
				var openAIClient *OpenAIRealtimeClient
				if exists && activeCall != nil {
					openAIClient = activeCall.OpenAIClient
				}
				b.mu.Unlock()

				if openAIClient != nil {
					// OpenAI client is available - forward the packet
					if !openAIForwardingStarted {
						log.Printf("🔄 OpenAI client now available - starting WhatsApp->OpenAI forwarding")
						openAIForwardingStarted = true
					}

					// Forward cleaned RTP packet to OpenAI
					if err := openAIClient.ForwardRTPToOpenAI(rtpBytes); err != nil {
						if packetCount <= 3 { // Only log first few errors
							log.Printf("❌ Error forwarding RTP to OpenAI: %v", err)
						}
					} else if packetCount == 1 || packetCount%100 == 0 {
						if packetCount == 1 {
							log.Printf("✅ First WhatsApp RTP packet forwarded to OpenAI! (cleaned headers)")
						} else {
							log.Printf("📦 Forwarded %d WhatsApp RTP packets (%d KB) to OpenAI",
								packetCount, totalBytes/1024)
						}
					}
				} else {
					// OpenAI client not ready yet - just count packets
					if packetCount%100 == 0 {
						log.Printf("🎤 Received %d audio packets (total: %d bytes) - waiting for OpenAI", packetCount, totalBytes)
					}
				}
			}
		}()
	})
	
	// Clean and validate the SDP
	sdpOffer = strings.TrimSpace(sdpOffer)
	sdpOffer = strings.ReplaceAll(sdpOffer, "\\r\\n", "\r\n")
	sdpOffer = strings.ReplaceAll(sdpOffer, "\\n", "\n")
	
	// Ensure SDP ends with a newline (required by some parsers)
	if !strings.HasSuffix(sdpOffer, "\n") && !strings.HasSuffix(sdpOffer, "\r\n") {
		sdpOffer += "\r\n"
		log.Printf("📝 Added missing newline to SDP")
	}
	
	log.Printf("🔍 SDP Offer (cleaned):\n%s", sdpOffer)
	log.Printf("📏 SDP length: %d bytes", len(sdpOffer))
	
	// Validate SDP starts correctly
	if !strings.HasPrefix(sdpOffer, "v=0") {
		log.Printf("❌ Invalid SDP: doesn't start with v=0")
	}
	
	// Count the number of lines for debugging
	lines := strings.Split(sdpOffer, "\n")
	log.Printf("📊 SDP has %d lines", len(lines))
	
	// Check for common SDP sections
	hasAudio := strings.Contains(sdpOffer, "m=audio")
	hasIceLite := strings.Contains(sdpOffer, "a=ice-lite")
	hasOpus := strings.Contains(sdpOffer, "opus/48000")
	hasTelephoneEvent := strings.Contains(sdpOffer, "telephone-event")
	log.Printf("✓ SDP contains: audio=%v, ice-lite=%v, opus=%v, telephone-event=%v", hasAudio, hasIceLite, hasOpus, hasTelephoneEvent)
	
	// Extract codecs offered by WhatsApp
	if hasAudio {
		lines := strings.Split(sdpOffer, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "m=audio") {
				log.Printf("📊 WhatsApp audio line: %s", strings.TrimSpace(line))
			} else if strings.HasPrefix(line, "a=rtpmap:") {
				log.Printf("📊 WhatsApp codec: %s", strings.TrimSpace(line))
			}
		}
	}
	
	// Try to parse specific problem areas
	if strings.Contains(sdpOffer, "a=extmap:") {
		log.Printf("📡 SDP contains extmap attributes - these might cause parsing issues")
		// Count extmap lines
		extmapCount := strings.Count(sdpOffer, "a=extmap:")
		log.Printf("📡 Found %d extmap attributes", extmapCount)
	}
	
	// Set the remote description (WhatsApp's offer)
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdpOffer,
	}
	
	if err := pc.SetRemoteDescription(offer); err != nil {
		log.Printf("❌ Failed to set remote description: %v", err)
		log.Printf("📋 Error type: %T", err)
		log.Printf("📋 Full error string: %q", err.Error())
		log.Printf("📋 SDP that failed:\n%s", sdpOffer)
		
		// Try parsing the SDP manually to see where it fails
		if sdpBytes := []byte(sdpOffer); len(sdpBytes) > 0 {
			log.Printf("📋 First 50 chars: %q", string(sdpBytes[:min(50, len(sdpBytes))]))
			log.Printf("📋 Last 50 chars: %q", string(sdpBytes[max(0, len(sdpBytes)-50):]))
		}
		
		// Check for specific error patterns
		errStr := err.Error()
		if strings.Contains(errStr, "EOF") {
			log.Printf("💡 EOF error - SDP might be truncated or have parsing issues")
		}
		if strings.Contains(errStr, "extmap") {
			log.Printf("💡 Error related to RTP extensions")
		}
		if strings.Contains(errStr, "codec") {
			log.Printf("💡 Error related to codec registration")
		}
		
		b.mu.Lock()
		delete(b.activeCalls, callID)
		b.mu.Unlock()
		if pc != nil {
			pc.Close()
		}
		return
	}
	
	// Create audio track with WhatsApp's exact codec parameters
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			// Match WhatsApp's exact fmtp line
			SDPFmtpLine: "maxaveragebitrate=20000;maxplaybackrate=16000;minptime=20;sprop-maxcapturerate=16000;useinbandfec=1",
		},
		"audio",
		"bridge-audio",
	)
	if err != nil {
		log.Printf("❌ Failed to create audio track: %v", err)
		b.mu.Lock()
		delete(b.activeCalls, callID)
		b.mu.Unlock()
		if pc != nil {
			pc.Close()
		}
		return
	}

	// Add the track directly to ensure Opus is in the SDP
	rtpSender, err := pc.AddTrack(audioTrack)
	if err != nil {
		log.Printf("❌ Failed to add audio track: %v", err)
		b.mu.Lock()
		delete(b.activeCalls, callID)
		b.mu.Unlock()
		if pc != nil {
			pc.Close()
		}
		return
	}
	
	// Read incoming RTCP packets
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()
	
	// Store the track in the call
	call.AudioTrack = audioTrack
	
	// Log track details
	log.Printf("✅ Added audio track with Opus codec for bidirectional audio")
	
	// Create answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Printf("❌ Failed to create answer: %v", err)
		if pc != nil {
			pc.Close()
		}
		b.mu.Lock()
		delete(b.activeCalls, callID)
		b.mu.Unlock()
		return
	}
	
	// Set local description
	if err := pc.SetLocalDescription(answer); err != nil {
		log.Printf("❌ Failed to set local description: %v", err)
		if pc != nil {
			pc.Close()
		}
		b.mu.Lock()
		delete(b.activeCalls, callID)
		b.mu.Unlock()
		return
	}
	
	// Log the final SDP that will be sent
	log.Printf("📄 Final SDP Answer being sent to WhatsApp (first 500 chars):")
	if len(answer.SDP) > 500 {
		log.Printf("%s...", answer.SDP[:500])
	} else {
		log.Printf("%s", answer.SDP)
	}
	
	// Check if it contains sendrecv
	if strings.Contains(answer.SDP, "a=sendrecv") {
		log.Printf("✅ SDP contains sendrecv - bidirectional audio enabled")
	} else if strings.Contains(answer.SDP, "a=recvonly") {
		log.Printf("❌ WARNING: SDP contains recvonly - only receive audio!")
	} else if strings.Contains(answer.SDP, "a=sendonly") {
		log.Printf("⚠️ WARNING: SDP contains sendonly - only send audio!")
	} else {
		log.Printf("⚠️ WARNING: No explicit direction attribute found in SDP")
		// If no direction is specified, sendrecv is the default, so add it explicitly
		log.Printf("📝 Note: When no direction is specified, sendrecv is the default behavior")
	}
	
	// Also check for audio media line
	if strings.Contains(answer.SDP, "m=audio") {
		// Extract the audio line
		lines := strings.Split(answer.SDP, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "m=audio") {
				log.Printf("📊 Audio media line: %s", strings.TrimSpace(line))
				break
			}
		}
	} else {
		log.Printf("❌ WARNING: No audio media line in SDP!")
	}
	
	// Wait for ICE gathering to complete
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	
	// Wait up to 3 seconds for gathering
	select {
	case <-gatherComplete:
		log.Printf("✅ ICE gathering complete for call %s", callID)
	case <-time.After(3 * time.Second):
		log.Printf("⏱️ ICE gathering timeout for call %s", callID)
	}
	
	// Get the local description with candidates
	localDesc := pc.LocalDescription()
	if localDesc != nil {
		log.Printf("📄 SDP Answer with candidates:\n%s", localDesc.SDP)
		answer.SDP = localDesc.SDP
	} else {
		log.Printf("📄 SDP Answer (no additional candidates):\n%s", answer.SDP)
	}
	
	// The call is already stored with peer connection
	
	// Send pre-accept to WhatsApp API first to establish WebRTC connection
	log.Printf("📞 Sending pre-accept for call %s", callID)
	log.Printf("📄 First 200 chars of answer SDP: %.200s...", answer.SDP)
	if err := b.sendPreAcceptCall(callID, answer.SDP); err != nil {
		log.Printf("❌ Failed to pre-accept call: %v", err)
		b.mu.Lock()
		if pc != nil {
			pc.Close()
		}
		delete(b.activeCalls, callID)
		b.mu.Unlock()
		return
	}
	
	// According to WhatsApp diagram, we should send accept immediately
	// The connection becomes active on first packet OR accept
	log.Printf("📞 Sending accept immediately after pre-accept for call %s", callID)
	if err := b.sendAcceptCall(callID, answer.SDP); err != nil {
		log.Printf("❌ Failed to accept call: %v", err)
		b.mu.Lock()
		if pc != nil {
			pc.Close()
		}
		delete(b.activeCalls, callID)
		b.mu.Unlock()
		return
	}
	
	log.Printf("✅ Call accepted: %s from %s", callID, callerNumber)
	
	// Log the current state
	connectionState := pc.ConnectionState()
	iceState := pc.ICEConnectionState()
	log.Printf("📊 Connection states - PC: %s, ICE: %s", connectionState.String(), iceState.String())
	
	// OpenAI will send audio, so we don't need continuous silence anymore
	log.Printf("🎧 Waiting for OpenAI to send audio...")
	
	// Now that the call is accepted, start media flow
	// Connect to Azure OpenAI Realtime API if configured
	azureKey := os.Getenv("AZURE_OPENAI_API_KEY")

	if azureKey != "" {
		log.Printf("🔵 Azure OpenAI API key found, starting Azure AI integration...")
		// Start Azure integration only after accept succeeds
		go func() {
			// Small delay to ensure everything is ready
			time.Sleep(500 * time.Millisecond)
			b.connectToOpenAIRealtime(callID, pc, azureKey, callerNumber, "") // No reminder for inbound calls
		}()
	} else {
		log.Printf("⚠️ AZURE_OPENAI_API_KEY not set - no AI agent will respond")
		// Play a welcome message only after accept succeeds
		go func() {
			// Small delay to ensure media channel is ready
			time.Sleep(100 * time.Millisecond)
			b.playWelcomeMessage(pc)
		}()
	}
}

// sendPreAcceptCall sends pre-accept to WhatsApp API
func (b *WhatsAppBridge) sendPreAcceptCall(callID, sdpAnswer string) error {
	return b.callWhatsAppAPI("pre_accept", callID, sdpAnswer)
}

// sendAcceptCall sends accept to WhatsApp API
func (b *WhatsAppBridge) sendAcceptCall(callID, sdpAnswer string) error {
	return b.callWhatsAppAPI("accept", callID, sdpAnswer)
}

// callWhatsAppAPI makes API calls to WhatsApp
func (b *WhatsAppBridge) callWhatsAppAPI(action, callID, sdpAnswer string) error {
	if b.accessToken == "" || b.phoneNumberID == "" {
		return fmt.Errorf("WhatsApp credentials not configured")
	}
	
	url := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/calls", b.phoneNumberID)
	
	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"call_id":          callID,
		"action":           action,
		"session": map[string]string{
			"sdp_type": "answer",
			"sdp":      sdpAnswer,
		},
	}
	
	if action == "accept" {
		payload["biz_opaque_callback_data"] = fmt.Sprintf("pion_%d", time.Now().Unix())
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	
	log.Printf("📤 WhatsApp API %s request to %s:", action, url)
	log.Printf("📤 Payload: %s", string(jsonData))
	
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.accessToken)
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	log.Printf("📡 WhatsApp API %s response: Status=%d, Body=%s", action, resp.StatusCode, string(body))
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("WhatsApp API error: %s - %s", resp.Status, string(body))
	}
	
	// Parse response to check if it was truly successful
	var apiResp map[string]interface{}
	if err := json.Unmarshal(body, &apiResp); err == nil {
		if success, ok := apiResp["success"].(bool); ok && success {
			log.Printf("✅ WhatsApp API %s successful for call %s", action, callID)
		} else {
			log.Printf("⚠️ WhatsApp API %s response doesn't confirm success: %v", action, apiResp)
		}
	}
	
	return nil
}

// connectToOpenAIRealtime connects the WhatsApp call to OpenAI's Realtime API
func (b *WhatsAppBridge) connectToOpenAIRealtime(callID string, whatsappPC *webrtc.PeerConnection, apiKey string, phoneNumber string, reminderText string) {
	log.Printf("🤖 Connecting call %s to OpenAI Realtime API (caller: %s)", callID, phoneNumber)

	// Create OpenAI client with phone number for task context and optional reminder
	openAIClient := NewOpenAIRealtimeClient(apiKey, phoneNumber, reminderText)
	
	// Get ephemeral token
	if err := openAIClient.GetEphemeralToken(); err != nil {
		log.Printf("❌ Failed to get OpenAI token: %v", err)
		return
	}
	
	// Connect to OpenAI Realtime API
	if err := openAIClient.ConnectToRealtimeAPI(b.api); err != nil {
		log.Printf("❌ Failed to connect to OpenAI: %v", err)
		return
	}
	
	// Store OpenAI client in call
	b.mu.Lock()
	if call, exists := b.activeCalls[callID]; exists {
		call.OpenAIClient = openAIClient
	}
	b.mu.Unlock()
	
	// Get the audio track we already created
	b.mu.Lock()
	call, exists := b.activeCalls[callID]
	var whatsappAudioTrack *webrtc.TrackLocalStaticRTP
	if exists && call != nil {
		whatsappAudioTrack = call.AudioTrack
	}
	b.mu.Unlock()
	
	if whatsappAudioTrack == nil {
		log.Printf("❌ No audio track found on WhatsApp connection")
		return
	}
	
	log.Printf("✅ Using existing audio track for OpenAI->WhatsApp audio")
	
	// Wait a moment for connections to stabilize
	time.Sleep(500 * time.Millisecond)
	
	// Set up bidirectional audio forwarding between WhatsApp and OpenAI
	log.Printf("🔄 Setting up bidirectional audio forwarding (v3 - no test packets)")
	log.Printf("📊 Call %s: WhatsApp → OpenAI (RTP forwarding)", callID)
	log.Printf("📊 Call %s: OpenAI → WhatsApp (waiting for track)", callID)
	
	// Forward audio from OpenAI to WhatsApp
	go func() {
		// Wait for OpenAI's remote audio track
		log.Printf("⏳ Waiting for OpenAI audio track...")
		var openAITrack *webrtc.TrackRemote
		for i := 0; i < 100; i++ { // Wait up to 10 seconds
			if track := openAIClient.GetRemoteAudioTrack(); track != nil {
				openAITrack = track
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		
		if openAITrack == nil {
			log.Printf("❌ OpenAI audio track not available after 10 seconds")
			return
		}
		
		log.Printf("🔊 Starting OpenAI → WhatsApp audio forwarding")
		
		// Get WhatsApp audio track
		b.mu.Lock()
		activeCall, exists := b.activeCalls[callID]
		var whatsappTrack *webrtc.TrackLocalStaticRTP
		if exists && activeCall != nil {
			whatsappTrack = activeCall.AudioTrack
		}
		b.mu.Unlock()
		
		if whatsappTrack == nil {
			log.Printf("❌ No WhatsApp track for audio forwarding")
			return
		}
		
		// Forward RTP packets from OpenAI to WhatsApp
		packetCount := 0
		lastLogTime := time.Now()

		for {
			// Read the full RTP packet (not just payload)
			rtpPacket, _, readErr := openAITrack.ReadRTP()
			if readErr != nil {
				log.Printf("❌ Error reading OpenAI RTP: %v", readErr)
				return
			}

			// v4 FIX: Clear extension headers before forwarding to WhatsApp
			// Prevents conflicts with WhatsApp's extension header IDs
			rtpPacket.Extension = false
			rtpPacket.Extensions = nil

			// Log first few packets for debugging
			if packetCount < 3 {
				log.Printf("🔍 OpenAI RTP packet %d: PayloadType=%d, SequenceNumber=%d, Timestamp=%d, PayloadSize=%d",
					packetCount, rtpPacket.PayloadType, rtpPacket.SequenceNumber, rtpPacket.Timestamp, len(rtpPacket.Payload))
			}

			// Marshal the RTP packet to bytes
			rtpBytes, marshalErr := rtpPacket.Marshal()
			if marshalErr != nil {
				log.Printf("❌ Error marshaling RTP packet: %v", marshalErr)
				continue
			}

			// Write the complete RTP packet to WhatsApp
			bytesWritten, writeErr := whatsappTrack.Write(rtpBytes)
			if writeErr != nil {
				log.Printf("❌ Error forwarding to WhatsApp (packet %d): %v", packetCount, writeErr)
				return
			}

			if packetCount < 3 {
				log.Printf("✅ Wrote %d bytes to WhatsApp track", bytesWritten)
			}

			packetCount++
			if packetCount == 1 {
				log.Printf("✅ First OpenAI audio packet forwarded to WhatsApp!")
			} else if time.Since(lastLogTime) > 5*time.Second {
				log.Printf("📦 Forwarded %d OpenAI audio packets to WhatsApp", packetCount)
				lastLogTime = time.Now()
			}
		}
	}()
	
	// The OnTrack handler is already set up in acceptIncomingCall
	// It will start forwarding audio once OpenAI client is stored
	log.Printf("🎧 Audio forwarding will start when WhatsApp track arrives")
	
	log.Printf("✅ OpenAI Realtime connection established for call %s", callID)
}

// playWelcomeMessage plays a welcome message or tone
func (b *WhatsAppBridge) playWelcomeMessage(pc *webrtc.PeerConnection) {
	// In a real implementation, you would:
	// 1. Create an audio track
	// 2. Generate or load audio data (welcome message)
	// 3. Send it through the peer connection
	
	// For now, just log that we would play audio
	log.Println("🎵 Would play welcome message to caller")
	
	// Example of how to add an audio track:
	// track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMA}, "audio", "pion")
	// if err == nil {
	//     _, err = pc.AddTrack(track)
	//     // Then write RTP packets to the track
	// }
}

// handleTestCall handles test call requests
func (b *WhatsAppBridge) handleTestCall(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	
	var testData map[string]interface{}
	if err := json.Unmarshal(body, &testData); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	// Check if SDP is provided
	sdpStr, hasSDP := testData["sdp"].(string)
	if hasSDP {
		// Process the SDP and create response
		responseSDP, err := b.processIncomingSDP(sdpStr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		response := map[string]interface{}{
			"status": "ok",
			"message": "Call processed with Pion WebRTC",
			"sdp": responseSDP,
			"call_id": fmt.Sprintf("call_%d", time.Now().Unix()),
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// No SDP provided, return simple OK response with generated SDP
	okSDP := b.generateOKResponseSDP()
	response := map[string]interface{}{
		"status": "ok",
		"message": "Test call processed",
		"sdp": okSDP,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// processIncomingSDP processes incoming SDP and creates a response
func (b *WhatsAppBridge) processIncomingSDP(offerSDP string) (string, error) {
	// Create a new PeerConnection
	pc, err := b.api.NewPeerConnection(b.config)
	if err != nil {
		return "", fmt.Errorf("failed to create peer connection: %v", err)
	}
	
	// Set up handlers
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State: %s", state.String())
		// v4: Handle explicit DTLS close
		if state == webrtc.ICEConnectionStateClosed {
			log.Printf("🔴 Test call: ICE connection explicitly closed")
		}
	})
	
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("📞 Received audio track: %s", track.ID())
		
		// Read RTP packets to detect audio
		go func() {
			buf := make([]byte, 1400)
			for {
				_, _, readErr := track.Read(buf)
				if readErr != nil {
					return
				}
				// Audio detected - in real implementation, we'd process it
				log.Println("🔊 Audio packet received")
			}
		}()
	})
	
	// Set the remote description
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}
	
	if err := pc.SetRemoteDescription(offer); err != nil {
		return "", fmt.Errorf("failed to set remote description: %v", err)
	}
	
	// Create answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("failed to create answer: %v", err)
	}
	
	// Set local description
	if err := pc.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("failed to set local description: %v", err)
	}
	
	// Store the call
	callID := fmt.Sprintf("call_%d", time.Now().Unix())
	b.mu.Lock()
	b.activeCalls[callID] = &Call{
		ID:             callID,
		PeerConnection: pc,
		StartTime:      time.Now(),
	}
	b.mu.Unlock()
	
	// Clean up after timeout
	go func() {
		time.Sleep(5 * time.Minute)
		b.mu.Lock()
		if call, exists := b.activeCalls[callID]; exists {
			if call.PeerConnection != nil {
				call.PeerConnection.Close()
			}
			delete(b.activeCalls, callID)
		}
		b.mu.Unlock()
	}()
	
	return answer.SDP, nil
}


// generateOKResponseSDP generates a simple OK response SDP
func (b *WhatsAppBridge) generateOKResponseSDP() string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf(`v=0
o=- %d %d IN IP4 0.0.0.0
s=Pion WhatsApp Bridge OK Response
c=IN IP4 0.0.0.0
t=0 0
m=audio 20000 RTP/AVP 111 126
a=rtpmap:111 opus/48000/2
a=rtpmap:126 telephone-event/8000
a=sendrecv
a=setup:active
a=ice-ufrag:pion%d
a=ice-pwd:pion%d`, timestamp, timestamp, timestamp, timestamp)
}

// handleStatus returns the bridge status
func (b *WhatsAppBridge) handleStatus(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	activeCallCount := len(b.activeCalls)
	b.mu.Unlock()
	
	status := map[string]interface{}{
		"status":       "running",
		"bridge_type":  "pion",
		"ice_lite":     false, // We respond as full ICE agent to WhatsApp's ice-lite
		"active_calls": activeCallCount,
		"timestamp":    time.Now().Format(time.RFC3339),
		"webhook_ready": true,
		"echo_enabled": os.Getenv("ENABLE_ECHO") == "true",
		"environment": map[string]bool{
			"whatsapp_token_set": b.accessToken != "",
			"phone_number_id_set": b.phoneNumberID != "",
			"verify_token_set": b.verifyToken != "",
		},
		"codec_support": []string{
			"opus/48000/2 (PT:111)",
			"telephone-event/8000 (PT:126)",
		},
		"webhook_endpoint": "/whatsapp-call",
		"railway_url": os.Getenv("RAILWAY_PUBLIC_DOMAIN"),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleHealth returns health check status
func (b *WhatsAppBridge) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// handleRequestCallPermission sends a permission request message to a user
func (b *WhatsAppBridge) handleRequestCallPermission(w http.ResponseWriter, r *http.Request) {
	log.Printf("📞 Received request-call-permission request from %s", r.RemoteAddr)

	var req struct {
		To string `json:"to"` // Phone number to request permission from (without +)
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("❌ Failed to decode request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.To == "" {
		log.Printf("❌ Phone number not provided in request")
		http.Error(w, "Phone number required", http.StatusBadRequest)
		return
	}

	log.Printf("📤 Requesting call permission from %s", req.To)

	// Send permission request message
	if err := SendCallPermissionRequest(req.To); err != nil {
		if err.Error() == "rate limited" {
			log.Printf("🚫 Rate limited: %s", req.To)
			http.Error(w, "Rate limited. You can only send 1 request per 24 hours, 2 per 7 days.", http.StatusTooManyRequests)
			return
		}
		log.Printf("❌ Failed to send permission request: %v", err)
		http.Error(w, fmt.Sprintf("Failed to send permission request: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "sent",
		"message": "Call permission request sent successfully",
		"to":      req.To,
	})

	log.Printf("✅ Successfully sent call permission request to %s", req.To)
}

// handleInitiateCall initiates an outbound call to a WhatsApp user
func (b *WhatsAppBridge) handleInitiateCall(w http.ResponseWriter, r *http.Request) {
	log.Printf("📞 Received initiate-call request from %s", r.RemoteAddr)

	var req struct {
		To           string `json:"to"`            // Phone number to call (without +)
		ReminderID   string `json:"reminder_id"`   // Optional: ID of reminder if this is a reminder call
		ReminderText string `json:"reminder_text"` // Optional: What to remind about
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("❌ Failed to decode request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.To == "" {
		log.Printf("❌ Phone number not provided in request")
		http.Error(w, "Phone number required", http.StatusBadRequest)
		return
	}

	// Check if we have permission to call this number
	permission, err := CheckCallPermission(req.To)
	if err != nil {
		log.Printf("⚠️ Error checking call permission for %s: %v", req.To, err)
		// Continue anyway - if Supabase is down, we don't want to block calls
	} else if permission == nil {
		log.Printf("🚫 No call permission for %s - user has not called us first", req.To)
		http.Error(w, "No call permission from recipient. They must call you first to grant permission.", http.StatusForbidden)
		return
	} else {
		log.Printf("✅ Call permission verified for %s (granted on %s)", req.To, permission.FirstInboundCallAt)
	}

	log.Printf("📞 Initiating outbound call to %s", req.To)

	// Create WebRTC peer connection
	pc, err := b.api.NewPeerConnection(b.config)
	if err != nil {
		log.Printf("❌ Failed to create peer connection: %v", err)
		http.Error(w, "Failed to create connection", http.StatusInternalServerError)
		return
	}

	// Create audio track for sending audio to WhatsApp user
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  "audio/opus",
			ClockRate: 48000,
			Channels:  2,
		},
		"audio",
		"pion-stream",
	)
	if err != nil {
		log.Printf("❌ Failed to create audio track: %v", err)
		http.Error(w, "Failed to create audio track", http.StatusInternalServerError)
		return
	}

	// Add track to peer connection
	_, err = pc.AddTrack(audioTrack)
	if err != nil {
		log.Printf("❌ Failed to add track: %v", err)
		http.Error(w, "Failed to add track", http.StatusInternalServerError)
		return
	}

	// Handle ICE connection state changes
	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("🧊 Outbound call ICE state: %s", connectionState.String())
		// v4: Handle explicit DTLS close
		if connectionState == webrtc.ICEConnectionStateClosed {
			log.Printf("🔴 Outbound call: ICE connection explicitly closed via DTLS")
		}
	})

	// Handle peer connection state changes
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("🔌 Outbound call connection state: %s", s.String())
	})

	// Create SDP offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Printf("❌ Failed to create offer: %v", err)
		http.Error(w, "Failed to create offer", http.StatusInternalServerError)
		return
	}

	// Set local description
	if err := pc.SetLocalDescription(offer); err != nil {
		log.Printf("❌ Failed to set local description: %v", err)
		http.Error(w, "Failed to set local description", http.StatusInternalServerError)
		return
	}

	// Wait for ICE gathering to complete
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	<-gatherComplete

	// Get complete SDP with ICE candidates
	sdpOffer := pc.LocalDescription().SDP

	log.Printf("📤 Sending outbound call request to WhatsApp API")
	log.Printf("📄 SDP Offer length: %d bytes", len(sdpOffer))
	log.Printf("📄 Full SDP Offer:\n%s", sdpOffer)
	log.Printf("=====================================")

	// Call WhatsApp API to initiate call
	callID, err := b.initiateWhatsAppCall(req.To, sdpOffer)
	if err != nil {
		log.Printf("❌ Failed to initiate call: %v", err)
		http.Error(w, fmt.Sprintf("Failed to initiate call: %v", err), http.StatusInternalServerError)
		pc.Close()
		return
	}

	log.Printf("✅ Outbound call initiated: call_id=%s, to=%s", callID, req.To)

	// Store the call
	call := &Call{
		ID:             callID,
		PeerConnection: pc,
		AudioTrack:     audioTrack,
		StartTime:      time.Now(),
		ReminderID:     req.ReminderID,
		ReminderText:   req.ReminderText,
	}

	// Log if this is a reminder call
	if req.ReminderID != "" {
		log.Printf("⏰ This is a reminder call: %s", req.ReminderText)
	}

	b.mu.Lock()
	b.activeCalls[callID] = call
	log.Printf("✅ Stored call in activeCalls map with key: %s", callID)
	log.Printf("📊 Total active calls: %d", len(b.activeCalls))
	b.mu.Unlock()

	// Pre-connect to Azure OpenAI so it's ready when user answers
	azureKey := os.Getenv("AZURE_OPENAI_API_KEY")

	if azureKey != "" {
		log.Printf("🔵 Pre-connecting to Azure OpenAI before user answers...")
		go func() {
			// Connect to Azure OpenAI in background while call is ringing
			// This way Azure is ready immediately when user answers
			b.connectToOpenAIRealtime(callID, pc, azureKey, req.To, req.ReminderText)
			log.Printf("✅ Azure OpenAI pre-connected and ready for call %s", callID)
		}()
	} else {
		log.Printf("⚠️ AZURE_OPENAI_API_KEY not set - no AI agent will respond")
	}

	// Handle incoming audio from user
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("🔊 Received audio track from outbound call: %s (codec: %s)", track.ID(), track.Codec().MimeType)

		// Forward audio to OpenAI when connected
		go func() {
			packetCount := 0
			totalBytes := 0
			openAIForwardingStarted := false

			for {
				// v4 FIX: Use ReadRTP() to access full packet with headers
				rtpPacket, _, readErr := track.ReadRTP()
				if readErr != nil {
					log.Printf("❌ Error reading audio from outbound call after %d packets: %v", packetCount, readErr)
					return
				}

				// v4 FIX: Clear extension headers to avoid conflicts between WhatsApp and OpenAI
				rtpPacket.Extension = false
				rtpPacket.Extensions = nil

				// Marshal back to bytes for forwarding
				rtpBytes, marshalErr := rtpPacket.Marshal()
				if marshalErr != nil {
					log.Printf("❌ Error marshaling outbound call RTP packet: %v", marshalErr)
					continue
				}

				packetCount++
				totalBytes += len(rtpBytes)

				// Check for OpenAI client on every packet (it becomes available after answer is received)
				b.mu.Lock()
				activeCall, exists := b.activeCalls[callID]
				var openAIClient *OpenAIRealtimeClient
				if exists && activeCall != nil {
					openAIClient = activeCall.OpenAIClient
				}
				b.mu.Unlock()

				if openAIClient != nil {
					// OpenAI client is available - forward the packet
					if !openAIForwardingStarted {
						log.Printf("🔄 OpenAI client available - starting outbound call audio forwarding")
						openAIForwardingStarted = true
					}

					// Forward cleaned RTP packet to OpenAI
					if err := openAIClient.ForwardRTPToOpenAI(rtpBytes); err != nil {
						if packetCount <= 3 {
							log.Printf("❌ Error forwarding outbound call RTP to OpenAI: %v", err)
						}
					} else if packetCount == 1 || packetCount%100 == 0 {
						if packetCount == 1 {
							log.Printf("✅ First outbound call RTP packet forwarded to OpenAI! (cleaned headers)")
						} else {
							log.Printf("📦 Forwarded %d outbound call RTP packets (%d KB) to OpenAI",
								packetCount, totalBytes/1024)
						}
					}
				} else {
					// OpenAI client not ready yet - just count packets
					if packetCount%100 == 0 {
						log.Printf("🎤 Received %d outbound call audio packets (waiting for OpenAI)", packetCount)
					}
				}
			}
		}()
	})

	// Respond with call ID
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"call_id": callID,
		"status":  "ringing",
		"to":      req.To,
	})
}

// handleCheckReminders checks for due reminders and initiates calls
func (b *WhatsAppBridge) handleCheckReminders(w http.ResponseWriter, r *http.Request) {
	log.Printf("⏰ Checking for due reminders...")

	// Get all due reminders
	reminders, err := GetDueReminders()
	if err != nil {
		log.Printf("❌ Failed to get due reminders: %v", err)
		http.Error(w, "Failed to check reminders", http.StatusInternalServerError)
		return
	}

	if len(reminders) == 0 {
		log.Printf("✅ No due reminders found")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "success",
			"message": "No due reminders",
			"count":   0,
		})
		return
	}

	log.Printf("📞 Found %d due reminders, initiating calls...", len(reminders))

	calledCount := 0
	failedCount := 0

	for _, reminder := range reminders {
		log.Printf("📞 Calling %s for reminder: %s", reminder.PhoneNumber, reminder.ReminderText)

		// Make the request to initiate call
		req := struct {
			To           string `json:"to"`
			ReminderText string `json:"reminder_text,omitempty"`
			ReminderID   string `json:"reminder_id,omitempty"`
		}{
			To:           reminder.PhoneNumber,
			ReminderText: reminder.ReminderText,
			ReminderID:   reminder.ID,
		}

		jsonData, _ := json.Marshal(req)
		resp, err := http.Post("http://localhost:3011/initiate-call", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("❌ Failed to initiate call for reminder %s: %v", reminder.ID, err)
			failedCount++
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			// Update reminder status to 'called'
			if err := UpdateReminderStatus(reminder.ID, "called", ""); err != nil {
				log.Printf("⚠️ Failed to update reminder status: %v", err)
			} else {
				log.Printf("✅ Reminder call initiated for %s", reminder.PhoneNumber)
				calledCount++
			}
		} else {
			log.Printf("❌ Failed to initiate call, status: %d", resp.StatusCode)
			failedCount++
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("Processed %d reminders", len(reminders)),
		"called":  calledCount,
		"failed":  failedCount,
	})
}

// handleOutboundCallAnswer processes the SDP answer when user accepts outbound call
func (b *WhatsAppBridge) handleOutboundCallAnswer(callID, sdpAnswer, from string) {
	log.Printf("🔄 Processing outbound call answer for %s", callID)

	// Get the call from active calls
	b.mu.Lock()
	log.Printf("🔍 Looking for call_id: %s", callID)
	log.Printf("🔍 Active calls count: %d", len(b.activeCalls))

	// Log all active call IDs for comparison
	activeIDs := []string{}
	for id := range b.activeCalls {
		activeIDs = append(activeIDs, id)
	}
	log.Printf("🔍 Active call IDs: %v", activeIDs)

	call, exists := b.activeCalls[callID]
	b.mu.Unlock()

	if !exists {
		log.Printf("❌ Call %s not found in active calls", callID)
		log.Printf("❌ This means the call_id from webhook doesn't match what we stored")
		return
	}

	// Set remote description (user's SDP answer)
	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sdpAnswer,
	}

	if err := call.PeerConnection.SetRemoteDescription(answer); err != nil {
		log.Printf("❌ Failed to set remote description: %v", err)
		return
	}

	log.Printf("✅ Set remote SDP answer for call %s", callID)
	log.Printf("✅ Outbound call %s connected - media should now flow", callID)
	log.Printf("🎙️ Azure OpenAI should already be connected and ready to respond")
}

// acceptOutboundCall sends the final accept to WhatsApp API for outbound call
func (b *WhatsAppBridge) acceptOutboundCall(callID string) error {
	url := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/calls", b.phoneNumberID)

	// Get the call
	b.mu.Lock()
	call, exists := b.activeCalls[callID]
	b.mu.Unlock()

	if !exists {
		return fmt.Errorf("call %s not found", callID)
	}

	// Get our local SDP
	localSDP := call.PeerConnection.LocalDescription().SDP

	reqBody := map[string]interface{}{
		"messaging_product": "whatsapp",
		"call_id":           callID,
		"action":            "accept",
		"session": map[string]string{
			"sdp_type": "answer",
			"sdp":      localSDP,
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	log.Printf("📤 Sending accept for outbound call %s", callID)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+b.accessToken)
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

	log.Printf("📡 WhatsApp API accept response: Status=%s, Body=%s", resp.Status, string(body))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("accept failed: %s - %s", resp.Status, string(body))
	}

	log.Printf("✅ WhatsApp API accept successful for outbound call %s", callID)
	return nil
}

// initiateWhatsAppCall calls WhatsApp API to initiate an outbound call
func (b *WhatsAppBridge) initiateWhatsAppCall(phoneNumber, sdpOffer string) (string, error) {
	url := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/calls", b.phoneNumberID)

	reqBody := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                phoneNumber,
		"action":            "connect",
		"session": map[string]string{
			"sdp_type": "offer",
			"sdp":      sdpOffer,
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+b.accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	log.Printf("📡 WhatsApp API response: Status=%s, Body=%s", resp.Status, string(body))

	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ WhatsApp API error: Status=%s, Body=%s", resp.Status, string(body))
		return "", fmt.Errorf("WhatsApp API error: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Calls []struct {
			ID string `json:"id"`
		} `json:"calls"`
		Success          bool   `json:"success"`
		MessagingProduct string `json:"messaging_product"`
		Error            *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("❌ Failed to parse response: %v", err)
		return "", err
	}

	if result.Error != nil {
		log.Printf("❌ WhatsApp API returned error: %+v", result.Error)
		return "", fmt.Errorf("WhatsApp API error: %s (code: %d)", result.Error.Message, result.Error.Code)
	}

	if len(result.Calls) == 0 {
		log.Printf("❌ WhatsApp API response has no calls array")
		return "", fmt.Errorf("no call_id in response")
	}

	callID := result.Calls[0].ID
	log.Printf("✅ WhatsApp API returned call_id: %s", callID)
	return callID, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  No .env file found or error loading it")
	} else {
		log.Println("✅ Loaded .env file")
	}

	log.Println("🚀 Starting Pion WhatsApp Bridge v3 - Proper Audio Architecture")
	log.Println("✨ Pure Go implementation with native ice-lite support")
	log.Println("🎯 Direct RTP forwarding: WhatsApp ↔️ OpenAI")

	bridge := NewWhatsAppBridge()
	bridge.Start()
}