package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

const (
	// WhatsApp webhook configuration
	VERIFY_TOKEN   = "whatsapp_bridge_token"
	WEBHOOK_SECRET = "your_webhook_secret" // Set via env var WHATSAPP_WEBHOOK_SECRET
)

// WhatsAppBridge handles WhatsApp call bridging using Pion WebRTC
type WhatsAppBridge struct {
	api           *webrtc.API
	config        webrtc.Configuration
	activeCalls   map[string]*Call
	mu            sync.Mutex
	webhookSecret string
	accessToken   string
	phoneNumberID string
}

// Call represents an active WhatsApp call session
type Call struct {
	ID           string
	PeerConnection *webrtc.PeerConnection
	AudioTrack   *webrtc.TrackLocalStaticRTP
	StartTime    time.Time
}

// NewWhatsAppBridge creates a new bridge instance
func NewWhatsAppBridge() *WhatsAppBridge {
	// Create a MediaEngine with only audio codecs
	m := &webrtc.MediaEngine{}
	
	// Register PCMA (G.711 A-law) - WhatsApp's preferred codec
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypePCMA,
			ClockRate:   8000,
			Channels:    1,
		},
		PayloadType: 8,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		log.Fatal(err)
	}
	
	// Register PCMU (G.711 μ-law) as fallback
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypePCMU,
			ClockRate:   8000,
			Channels:    1,
		},
		PayloadType: 0,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		log.Fatal(err)
	}
	
	// Create a SettingEngine and enable ice-lite mode
	s := webrtc.SettingEngine{}
	s.SetLite(true) // Enable ice-lite mode for WhatsApp compatibility
	s.SetAnsweringDTLSRole(webrtc.DTLSRoleServer)
	
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
	
	// Configure ICE servers (none needed for ice-lite)
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{},
	}
	
	webhookSecret := os.Getenv("WHATSAPP_WEBHOOK_SECRET")
	if webhookSecret == "" {
		webhookSecret = WEBHOOK_SECRET
	}
	
	accessToken := os.Getenv("WHATSAPP_ACCESS_TOKEN")
	if accessToken == "" {
		log.Println("⚠️  WHATSAPP_ACCESS_TOKEN not set - API calls will fail")
	}
	
	phoneNumberID := os.Getenv("WHATSAPP_PHONE_NUMBER_ID")
	if phoneNumberID == "" {
		log.Println("⚠️  WHATSAPP_PHONE_NUMBER_ID not set - API calls will fail")
	}
	
	return &WhatsAppBridge{
		api:           api,
		config:        config,
		activeCalls:   make(map[string]*Call),
		webhookSecret: webhookSecret,
		accessToken:   accessToken,
		phoneNumberID: phoneNumberID,
	}
}

// Start begins the HTTP server
func (b *WhatsAppBridge) Start() {
	router := mux.NewRouter()
	
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
	
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatal(err)
	}
}

// handleWebhookVerification handles WhatsApp webhook verification
func (b *WhatsAppBridge) handleWebhookVerification(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")
	
	if mode == "subscribe" && token == VERIFY_TOKEN {
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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	
	// Verify webhook signature if present
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature != "" && !b.verifySignature(body, signature) {
		http.Error(w, "Invalid signature", http.StatusForbidden)
		return
	}
	
	// Parse webhook data
	var webhook map[string]interface{}
	if err := json.Unmarshal(body, &webhook); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	log.Printf("📱 WhatsApp webhook received: %s", string(body))
	
	// Process the webhook
	response := b.processWebhook(webhook)
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// verifySignature verifies the webhook signature
func (b *WhatsAppBridge) verifySignature(body []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(b.webhookSecret))
	mac.Write(body)
	expectedMAC := mac.Sum(nil)
	expectedSignature := "sha256=" + hex.EncodeToString(expectedMAC)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// processWebhook processes incoming webhook data
func (b *WhatsAppBridge) processWebhook(webhook map[string]interface{}) map[string]interface{} {
	// Parse WhatsApp webhook structure
	entry, ok := webhook["entry"].([]interface{})
	if !ok || len(entry) == 0 {
		return map[string]interface{}{"status": "ok", "message": "No entry found"}
	}
	
	// Get first entry
	firstEntry, ok := entry[0].(map[string]interface{})
	if !ok {
		return map[string]interface{}{"status": "ok", "message": "Invalid entry format"}
	}
	
	// Get changes
	changes, ok := firstEntry["changes"].([]interface{})
	if !ok || len(changes) == 0 {
		return map[string]interface{}{"status": "ok", "message": "No changes found"}
	}
	
	// Process each change
	for _, change := range changes {
		changeData, ok := change.(map[string]interface{})
		if !ok {
			continue
		}
		
		// Check if it's a calls field
		if field, ok := changeData["field"].(string); ok && field == "calls" {
			value, ok := changeData["value"].(map[string]interface{})
			if !ok {
				continue
			}
			
			// Process call events
			if calls, ok := value["calls"].([]interface{}); ok && len(calls) > 0 {
				for _, call := range calls {
					b.handleCallEvent(call.(map[string]interface{}))
				}
			}
		}
	}
	
	// Always return OK to acknowledge webhook receipt
	return map[string]interface{}{
		"status": "ok",
		"message": "Webhook processed",
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
			call.PeerConnection.Close()
			delete(b.activeCalls, callID)
			log.Printf("☎️ Call terminated: %s", callID)
		}
		b.mu.Unlock()
		
	default:
		log.Printf("📋 Unhandled call event: %s", event)
	}
}

// acceptIncomingCall handles accepting an incoming WhatsApp call
func (b *WhatsAppBridge) acceptIncomingCall(callID, sdpOffer, callerNumber string) {
	// Create a new PeerConnection
	pc, err := b.api.NewPeerConnection(b.config)
	if err != nil {
		log.Printf("❌ Failed to create peer connection: %v", err)
		return
	}
	
	// Set up handlers
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State for call %s: %s", callID, state.String())
	})
	
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("🔊 Received audio track for call %s: %s", callID, track.ID())
		
		// Process audio packets
		go func() {
			buf := make([]byte, 1400)
			for {
				_, _, readErr := track.Read(buf)
				if readErr != nil {
					return
				}
				// Audio packet received - you can process it here
				// For now, we just acknowledge we're receiving audio
			}
		}()
	})
	
	// Set the remote description (WhatsApp's offer)
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdpOffer,
	}
	
	if err := pc.SetRemoteDescription(offer); err != nil {
		log.Printf("❌ Failed to set remote description: %v", err)
		pc.Close()
		return
	}
	
	// Create answer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Printf("❌ Failed to create answer: %v", err)
		pc.Close()
		return
	}
	
	// Set local description
	if err := pc.SetLocalDescription(answer); err != nil {
		log.Printf("❌ Failed to set local description: %v", err)
		pc.Close()
		return
	}
	
	// Store the call
	b.mu.Lock()
	b.activeCalls[callID] = &Call{
		ID:             callID,
		PeerConnection: pc,
		StartTime:      time.Now(),
	}
	b.mu.Unlock()
	
	// Send pre-accept to WhatsApp API for faster connection
	if err := b.sendPreAcceptCall(callID, answer.SDP); err != nil {
		log.Printf("❌ Failed to pre-accept call: %v", err)
	}
	
	// Send accept to WhatsApp API
	if err := b.sendAcceptCall(callID, answer.SDP); err != nil {
		log.Printf("❌ Failed to accept call: %v", err)
		b.mu.Lock()
		pc.Close()
		delete(b.activeCalls, callID)
		b.mu.Unlock()
		return
	}
	
	log.Printf("✅ Call accepted: %s from %s", callID, callerNumber)
	
	// Play a simple audio tone or message
	// In a real implementation, you'd connect to your audio processing system here
	go b.playWelcomeMessage(pc)
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
			call.PeerConnection.Close()
			delete(b.activeCalls, callID)
		}
		b.mu.Unlock()
	}()
	
	return b.modifySDPForWhatsApp(answer.SDP), nil
}

// modifySDPForWhatsApp modifies the SDP for WhatsApp compatibility
func (b *WhatsAppBridge) modifySDPForWhatsApp(sdpStr string) string {
	// Parse the SDP
	sessionDescription := &sdp.SessionDescription{}
	if err := sessionDescription.Unmarshal([]byte(sdpStr)); err != nil {
		log.Printf("Failed to parse SDP: %v", err)
		return sdpStr
	}
	
	// Modify for WhatsApp compatibility
	// - Ensure ice-lite attribute is present
	// - Set connection to passive mode
	// - Remove unnecessary attributes
	
	var modifiedSDP strings.Builder
	modifiedSDP.WriteString("v=0\r\n")
	modifiedSDP.WriteString(fmt.Sprintf("o=- %d %d IN IP4 0.0.0.0\r\n", time.Now().Unix(), time.Now().Unix()))
	modifiedSDP.WriteString("s=Pion WhatsApp Bridge\r\n")
	modifiedSDP.WriteString("c=IN IP4 0.0.0.0\r\n")
	modifiedSDP.WriteString("t=0 0\r\n")
	modifiedSDP.WriteString("a=ice-lite\r\n")
	
	// Add media sections
	for _, media := range sessionDescription.MediaDescriptions {
		if media.MediaName.Media == "audio" {
			modifiedSDP.WriteString(fmt.Sprintf("m=audio %d RTP/AVP 8 0\r\n", media.MediaName.Port))
			modifiedSDP.WriteString("a=rtpmap:8 PCMA/8000\r\n")
			modifiedSDP.WriteString("a=rtpmap:0 PCMU/8000\r\n")
			modifiedSDP.WriteString("a=sendrecv\r\n")
			modifiedSDP.WriteString("a=setup:passive\r\n")
			
			// Add ICE credentials if present
			for _, attr := range media.Attributes {
				if attr.Key == "ice-ufrag" || attr.Key == "ice-pwd" {
					modifiedSDP.WriteString(fmt.Sprintf("a=%s:%s\r\n", attr.Key, attr.Value))
				}
			}
		}
	}
	
	return modifiedSDP.String()
}

// generateOKResponseSDP generates a simple OK response SDP
func (b *WhatsAppBridge) generateOKResponseSDP() string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf(`v=0
o=- %d %d IN IP4 0.0.0.0
s=Pion WhatsApp Bridge OK Response
c=IN IP4 0.0.0.0
t=0 0
a=ice-lite
m=audio 20000 RTP/AVP 8 0
a=rtpmap:8 PCMA/8000
a=rtpmap:0 PCMU/8000
a=sendrecv
a=setup:passive
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
		"ice_lite":     true,
		"active_calls": activeCallCount,
		"timestamp":    time.Now().Format(time.RFC3339),
		"webhook_ready": true,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleHealth returns health check status
func (b *WhatsAppBridge) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func main() {
	log.Println("🚀 Starting Pion WhatsApp Bridge")
	log.Println("✨ Pure Go implementation with native ice-lite support")
	
	bridge := NewWhatsAppBridge()
	bridge.Start()
}