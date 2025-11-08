package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// ExampleSendingMessages demonstrates how to send various types of messages
func ExampleSendingMessages() {
	// Initialize SDK (uses environment variables WHATSAPP_TOKEN and PHONE_NUMBER_ID)
	wa := NewWhatsAppSDK("", "")

	// Send simple text message
	resp, err := wa.QuickSend("+1234567890", "Hello World!")
	if err != nil {
		log.Printf("Error sending text: %v", err)
	} else {
		log.Printf("Text sent: %+v", resp)
	}

	// Send image
	resp, err = wa.Client.SendImage("+1234567890", "https://example.com/image.jpg")
	if err != nil {
		log.Printf("Error sending image: %v", err)
	} else {
		log.Printf("Image sent: %+v", resp)
	}

	// Send audio
	resp, err = wa.Client.SendAudio("+1234567890", "https://example.com/audio.mp3")
	if err != nil {
		log.Printf("Error sending audio: %v", err)
	} else {
		log.Printf("Audio sent: %+v", resp)
	}

	// Send menu with buttons (max 3 buttons)
	options := map[string]string{
		"beginner":     "Beginner",
		"intermediate": "Intermediate",
		"advanced":     "Advanced",
	}
	resp, err = wa.SendMenu("+1234567890", "Choose your level:", options)
	if err != nil {
		log.Printf("Error sending menu: %v", err)
	} else {
		log.Printf("Menu sent: %+v", resp)
	}

	// Send media menu with image header
	resp, err = wa.SendMediaMenu(
		"+1234567890",
		"Check out our courses:",
		"https://example.com/header.jpg",
		"image",
		options,
	)
	if err != nil {
		log.Printf("Error sending media menu: %v", err)
	} else {
		log.Printf("Media menu sent: %+v", resp)
	}

	// Send template message
	resp, err = wa.Client.SendTemplate(
		"+1234567890",
		"welcome_message",
		"en",
		nil,
	)
	if err != nil {
		log.Printf("Error sending template: %v", err)
	} else {
		log.Printf("Template sent: %+v", resp)
	}
}

// ExampleWebhookHandler demonstrates how to handle incoming messages
func ExampleWebhookHandler(w http.ResponseWriter, r *http.Request) {
	// Initialize SDK
	wa := NewWhatsAppSDK("", "")

	// Parse webhook data
	handler := wa.WebhookHandler()

	var webhookData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&webhookData); err != nil {
		log.Printf("Error decoding webhook: %v", err)
		http.Error(w, "Invalid webhook data", http.StatusBadRequest)
		return
	}

	// Convert to JSON bytes for parsing
	jsonData, _ := json.Marshal(webhookData)
	if err := handler.Parse(jsonData); err != nil {
		log.Printf("Error parsing webhook: %v", err)
		http.Error(w, "Error parsing webhook", http.StatusBadRequest)
		return
	}

	// Check for duplicate messages
	if handler.IsDuplicate() {
		log.Printf("🔁 Duplicate message ignored")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "duplicate"})
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
		handleTextMessage(handler, text)
	case "interactive":
		handleInteractiveMessage(handler, text)
	case "audio":
		handleAudioMessage(handler)
	case "image":
		handleImageMessage(handler)
	case "video":
		handleVideoMessage(handler)
	default:
		log.Printf("Unknown message type: %s", msgType)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "processed"})
}

// handleTextMessage handles incoming text messages
func handleTextMessage(handler *WebhookHandler, text string) {
	switch text {
	case "help", "Help", "HELP":
		handler.ReplyText("Welcome! How can I help you today?\n\n" +
			"Commands:\n" +
			"- menu: View options\n" +
			"- help: Show this message\n" +
			"- call: Start a voice call")

	case "menu", "Menu", "MENU":
		buttons := []Button{
			NewButton("learn", "Learn"),
			NewButton("practice", "Practice"),
			NewButton("test", "Take Test"),
		}
		handler.ReplyButtons("What would you like to do?", buttons, nil)

	case "call", "Call", "CALL":
		handler.ReplyText("Voice call feature coming soon!")

	default:
		// Echo back the message
		handler.ReplyText("You said: " + text)
	}
}

// handleInteractiveMessage handles button/list replies
func handleInteractiveMessage(handler *WebhookHandler, selection string) {
	log.Printf("User selected: %s", selection)

	switch selection {
	case "Learn":
		handler.ReplyText("Great! Let's start learning. What topic are you interested in?")
	case "Practice":
		handler.ReplyText("Time to practice! I'll send you some exercises.")
	case "Take Test":
		handler.ReplyText("Starting your test now. Good luck!")
	default:
		handler.ReplyText("Got your selection: " + selection)
	}
}

// handleAudioMessage handles incoming audio messages
func handleAudioMessage(handler *WebhookHandler) {
	audioID := handler.AudioID()
	if audioID == "" {
		log.Printf("No audio ID found")
		return
	}

	log.Printf("📢 Received audio message: %s", audioID)

	// Download the audio file
	filename := "audio_" + audioID + ".ogg"
	savedPath, err := handler.client.DownloadMedia(audioID, filename)
	if err != nil {
		log.Printf("Error downloading audio: %v", err)
		handler.ReplyText("Sorry, I couldn't process your audio message.")
		return
	}

	log.Printf("✅ Audio saved to: %s", savedPath)
	handler.ReplyText("Thanks for the audio message! I've received it.")

	// Here you could:
	// 1. Transcribe the audio using OpenAI Whisper
	// 2. Process the audio for voice commands
	// 3. Store it for later playback
}

// handleImageMessage handles incoming image messages
func handleImageMessage(handler *WebhookHandler) {
	imageID := handler.ImageID()
	if imageID == "" {
		log.Printf("No image ID found")
		return
	}

	log.Printf("🖼️ Received image message: %s", imageID)

	// Download the image file
	filename := "image_" + imageID + ".jpg"
	savedPath, err := handler.client.DownloadMedia(imageID, filename)
	if err != nil {
		log.Printf("Error downloading image: %v", err)
		handler.ReplyText("Sorry, I couldn't process your image.")
		return
	}

	log.Printf("✅ Image saved to: %s", savedPath)
	handler.ReplyText("Great image! I've received it.")

	// Here you could:
	// 1. Use GPT-4 Vision to analyze the image
	// 2. Extract text using OCR
	// 3. Perform image recognition
}

// handleVideoMessage handles incoming video messages
func handleVideoMessage(handler *WebhookHandler) {
	videoID := handler.VideoID()
	if videoID == "" {
		log.Printf("No video ID found")
		return
	}

	log.Printf("🎥 Received video message: %s", videoID)

	// Download the video file
	filename := "video_" + videoID + ".mp4"
	savedPath, err := handler.client.DownloadMedia(videoID, filename)
	if err != nil {
		log.Printf("Error downloading video: %v", err)
		handler.ReplyText("Sorry, I couldn't process your video.")
		return
	}

	log.Printf("✅ Video saved to: %s", savedPath)
	handler.ReplyText("Thanks for the video! I've received it.")
}

// Example of integrating with existing webhook endpoint in main.go
func AddMessagingWebhookExample() {
	// In your main.go, you can add a new route or extend the existing /whatsapp-call endpoint
	// to handle both call and message webhooks

	http.HandleFunc("/whatsapp-message", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			// Webhook verification (same as calls)
			mode := r.URL.Query().Get("hub.mode")
			verifyToken := r.URL.Query().Get("hub.verify_token")
			challenge := r.URL.Query().Get("hub.challenge")

			expectedToken := "whatsapp_bridge_token" // Use same or different token

			if mode == "subscribe" && verifyToken == expectedToken {
				log.Printf("✅ Message webhook verified")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(challenge))
				return
			}

			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if r.Method == "POST" {
			// Handle incoming messages
			ExampleWebhookHandler(w, r)
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})
}
