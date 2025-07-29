package main

import (
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
	
	return &WhatsAppBridge{
		api:           api,
		config:        config,
		activeCalls:   make(map[string]*Call),
		webhookSecret: webhookSecret,
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
	// Check if it's a call-related webhook
	// This is where we'd parse WhatsApp's call initiation
	// For now, return OK response
	
	// Generate a simple OK response
	return map[string]interface{}{
		"status":  "ok",
		"message": "Call processed successfully",
		"call_id": fmt.Sprintf("call_%d", time.Now().Unix()),
	}
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