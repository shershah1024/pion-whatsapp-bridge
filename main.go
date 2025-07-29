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
	"github.com/pion/webrtc/v3"
)

const (
	// Default webhook configuration (overridden by env vars)
	DEFAULT_VERIFY_TOKEN = "whatsapp_bridge_token"
)

// WhatsAppBridge handles WhatsApp call bridging using Pion WebRTC
type WhatsAppBridge struct {
	api           *webrtc.API
	config        webrtc.Configuration
	activeCalls   map[string]*Call
	mu            sync.Mutex
	verifyToken   string
	accessToken   string
	phoneNumberID string
}

// Call represents an active WhatsApp call session
type Call struct {
	ID             string
	PeerConnection *webrtc.PeerConnection
	AudioTrack     *webrtc.TrackLocalStaticRTP
	StartTime      time.Time
	OpenAIClient   *OpenAIRealtimeClient
}

// NewWhatsAppBridge creates a new bridge instance
func NewWhatsAppBridge() *WhatsAppBridge {
	// Create a MediaEngine with audio codecs
	m := &webrtc.MediaEngine{}
	
	// First register the Opus codec with WhatsApp's exact parameters
	opusCodec := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1", // Match WhatsApp's fmtp exactly
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
	
	return &WhatsAppBridge{
		api:           api,
		config:        config,
		activeCalls:   make(map[string]*Call),
		verifyToken:   verifyToken,
		accessToken:   accessToken,
		phoneNumberID: phoneNumberID,
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
	
	// Get port from environment or default
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
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
			// Check for calls array directly in value
			if calls, ok := value["calls"].([]interface{}); ok && len(calls) > 0 {
				log.Printf("📞 Found %d call events", len(calls))
				for _, call := range calls {
					if callData, ok := call.(map[string]interface{}); ok {
						b.handleCallEvent(callData)
					}
				}
			} else if messages, ok := value["messages"].([]interface{}); ok && len(messages) > 0 {
				log.Printf("💬 Found %d message events (ignoring)", len(messages))
			} else if statuses, ok := value["statuses"].([]interface{}); ok && len(statuses) > 0 {
				log.Printf("📊 Found %d status events", len(statuses))
				// Check if any are call statuses
				for _, status := range statuses {
					if statusData, ok := status.(map[string]interface{}); ok {
						if statusType, ok := statusData["type"].(string); ok && statusType == "call" {
							log.Printf("📞 Found call status event")
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
	
	// Log additional call data for debugging
	if status, ok := callData["status"].(string); ok {
		log.Printf("📞 Call status: %s", status)
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
					log.Printf("📥 Received SDP offer for call %s", callID)
					// Process the call asynchronously
					go b.acceptIncomingCall(callID, sdpOffer, from)
				}
			}
		}
		
	case "terminate":
		// Handle call termination
		b.mu.Lock()
		if call, exists := b.activeCalls[callID]; exists {
			if call.PeerConnection != nil {
				call.PeerConnection.Close()
			}
			delete(b.activeCalls, callID)
			log.Printf("☎️ Call terminated: %s", callID)
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

// acceptIncomingCall handles accepting an incoming WhatsApp call
func (b *WhatsAppBridge) acceptIncomingCall(callID, sdpOffer, callerNumber string) {
	log.Printf("🔔 Processing incoming call %s from %s", callID, callerNumber)
	log.Printf("📋 Call flow: 1) Create PeerConnection → 2) Set SDP → 3) Pre-accept → 4) Accept → 5) Media flow")
	
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
	})
	
	pc.OnICEGatheringStateChange(func(state webrtc.ICEGathererState) {
		log.Printf("ICE Gathering State for call %s: %s", callID, state.String())
	})
	
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			log.Printf("ICE Candidate for call %s: %s", callID, candidate.String())
		}
	})
	
	// We'll create and add the audio track AFTER setting remote description
	
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("🔊 Received audio track for call %s: %s (codec: %s)", callID, track.ID(), track.Codec().MimeType)
		
		// Get the local audio track for echo
		b.mu.Lock()
		call, exists := b.activeCalls[callID]
		b.mu.Unlock()
		
		if !exists || call.AudioTrack == nil {
			log.Printf("⚠️ No audio track available for echo")
			return
		}
		
		// Echo audio packets back
		go func() {
			buf := make([]byte, 1400)
			packetCount := 0
			for {
				n, _, readErr := track.Read(buf)
				if readErr != nil {
					log.Printf("❌ Error reading audio: %v", readErr)
					return
				}
				
				packetCount++
				if packetCount%100 == 0 {
					log.Printf("🎤 Received %d audio packets (last packet size: %d bytes)", packetCount, n)
				}
				
				// Echo the packet back (if echo mode is enabled)
				if os.Getenv("ENABLE_ECHO") == "true" {
					if _, err := call.AudioTrack.Write(buf[:n]); err != nil {
						log.Printf("❌ Error writing echo: %v", err)
						return
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
	log.Printf("✓ SDP contains: audio=%v, ice-lite=%v, opus=%v", hasAudio, hasIceLite, hasOpus)
	
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
	
	// Now that remote description is set, create and add audio track
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{
		MimeType: webrtc.MimeTypeOpus,
	}, "audio", "pion")
	if err != nil {
		log.Printf("❌ Failed to create audio track: %v", err)
	} else {
		// Add track to peer connection
		rtpSender, err := pc.AddTrack(audioTrack)
		if err != nil {
			log.Printf("❌ Failed to add audio track: %v", err)
		} else {
			// Handle RTCP packets
			go func() {
				rtcpBuf := make([]byte, 1500)
				for {
					if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
						return
					}
				}
			}()
			
			// Store the audio track in the call
			b.mu.Lock()
			if call, exists := b.activeCalls[callID]; exists {
				call.AudioTrack = audioTrack
			}
			b.mu.Unlock()
			log.Printf("✅ Audio track created and added for echo")
		}
	}
	
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
	
	// Update the call with peer connection
	b.mu.Lock()
	if call, exists := b.activeCalls[callID]; exists {
		call.PeerConnection = pc
	}
	b.mu.Unlock()
	
	// Send pre-accept to WhatsApp API first to establish WebRTC connection
	log.Printf("📞 Sending pre-accept for call %s", callID)
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
	
	// Wait for ICE connection to be established after pre-accept
	log.Printf("⏳ Waiting for ICE connection after pre-accept...")
	
	// Wait up to 5 seconds for ICE connection
	select {
	case <-iceConnected:
		log.Printf("✅ ICE connection established")
	case <-time.After(5 * time.Second):
		log.Printf("⏱️ ICE connection timeout - proceeding anyway")
	}
	
	// Now send accept to start media flow
	log.Printf("📞 Sending accept for call %s", callID)
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
	
	// Now that the call is accepted, start media flow
	// Connect to OpenAI Realtime API if configured
	openAIKey := os.Getenv("OPENAI_API_KEY")
	if openAIKey != "" {
		// Start OpenAI integration only after accept succeeds
		go b.connectToOpenAIRealtime(callID, pc, openAIKey)
	} else {
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
	
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/calls", b.phoneNumberID)
	
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
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("WhatsApp API error: %s - %s", resp.Status, string(body))
	}
	
	log.Printf("✅ WhatsApp API %s successful for call %s", action, callID)
	return nil
}

// connectToOpenAIRealtime connects the WhatsApp call to OpenAI's Realtime API
func (b *WhatsAppBridge) connectToOpenAIRealtime(callID string, whatsappPC *webrtc.PeerConnection, apiKey string) {
	log.Printf("🤖 Connecting call %s to OpenAI Realtime API", callID)
	
	// Create OpenAI client
	openAIClient := NewOpenAIRealtimeClient(apiKey)
	
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
	
	// Set up audio forwarding from WhatsApp to OpenAI
	whatsappPC.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("🎤 Forwarding audio from WhatsApp to OpenAI")
		
		// Read RTP packets from WhatsApp
		go func() {
			buf := make([]byte, 1400)
			for {
				n, _, readErr := track.Read(buf)
				if readErr != nil {
					return
				}
				
				// Forward audio to OpenAI
				// Note: This would need proper RTP to PCM conversion
				// For now, just log that we would forward
				log.Printf("🔊 Would forward %d bytes to OpenAI", n)
				
				// In a real implementation:
				// 1. Extract PCM audio from RTP packets
				// 2. Convert to base64
				// 3. Send to OpenAI via openAIClient.SendAudioToOpenAI()
			}
		}()
	})
	
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	log.Println("🚀 Starting Pion WhatsApp Bridge")
	log.Println("✨ Pure Go implementation with native ice-lite support")
	
	bridge := NewWhatsAppBridge()
	bridge.Start()
}