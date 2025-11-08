package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// MessageType represents the type of WhatsApp message
type MessageType string

const (
	MessageTypeText        MessageType = "text"
	MessageTypeImage       MessageType = "image"
	MessageTypeAudio       MessageType = "audio"
	MessageTypeVideo       MessageType = "video"
	MessageTypeDocument    MessageType = "document"
	MessageTypeInteractive MessageType = "interactive"
	MessageTypeTemplate    MessageType = "template"
)

// Config holds WhatsApp API configuration
type Config struct {
	Token      string
	PhoneID    string
	APIVersion string
	BaseURL    string
}

// NewConfig creates a new Config with defaults from environment variables
func NewConfig() *Config {
	token := os.Getenv("WHATSAPP_TOKEN")
	if token == "" {
		token = os.Getenv("TOKEN")
	}

	phoneID := os.Getenv("PHONE_NUMBER_ID")
	if phoneID == "" {
		phoneID = os.Getenv("WHATSAPP_PHONE_ID")
	}
	if phoneID == "" {
		phoneID = os.Getenv("PHONE_ID")
	}

	apiVersion := "v21.0"
	baseURL := fmt.Sprintf("https://graph.facebook.com/%s/%s/messages", apiVersion, phoneID)

	return &Config{
		Token:      token,
		PhoneID:    phoneID,
		APIVersion: apiVersion,
		BaseURL:    baseURL,
	}
}

// Button represents an interactive button
type Button struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// NewButton creates a new button with default type "reply"
func NewButton(id, title string) Button {
	return Button{
		ID:    id,
		Title: title,
		Type:  "reply",
	}
}

// MediaHeader represents a media header for interactive messages
type MediaHeader struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
	ID   string `json:"id,omitempty"`
}

// WhatsAppClient handles sending messages to WhatsApp
type WhatsAppClient struct {
	config       *Config
	httpClient   *http.Client
	processedIDs map[string]bool
	mu           sync.RWMutex
}

// NewWhatsAppClient creates a new WhatsApp client
func NewWhatsAppClient(config *Config) *WhatsAppClient {
	if config == nil {
		config = NewConfig()
	}

	return &WhatsAppClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		processedIDs: make(map[string]bool),
	}
}

// request makes an HTTP request to the WhatsApp API
func (c *WhatsAppClient) request(method, url string, body interface{}) (map[string]interface{}, error) {
	if url == "" {
		url = c.config.BaseURL
	}

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
		log.Printf("📤 WhatsApp Messaging API request: %s", string(jsonData))
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.config.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("📡 WhatsApp Messaging API response: Status=%d, Body=%s", resp.StatusCode, string(respBody))

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.StatusCode != 200 {
		return result, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	return result, nil
}

// Send sends a message to WhatsApp
func (c *WhatsAppClient) Send(to string, msgType MessageType, content map[string]interface{}) (map[string]interface{}, error) {
	data := map[string]interface{}{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to,
		"type":              string(msgType),
	}

	switch msgType {
	case MessageTypeText:
		data["text"] = map[string]interface{}{
			"body":        content["text"],
			"preview_url": true,
		}
	case MessageTypeImage:
		data["image"] = map[string]interface{}{
			"link": content["url"],
		}
	case MessageTypeAudio:
		data["audio"] = map[string]interface{}{
			"link": content["url"],
		}
	case MessageTypeVideo:
		data["video"] = map[string]interface{}{
			"link": content["url"],
		}
	case MessageTypeDocument:
		data["document"] = map[string]interface{}{
			"link": content["url"],
		}
	case MessageTypeInteractive:
		data["interactive"] = c.buildInteractive(content)
	case MessageTypeTemplate:
		data["template"] = content
	}

	return c.request("POST", "", data)
}

// buildInteractive builds an interactive message payload
func (c *WhatsAppClient) buildInteractive(content map[string]interface{}) map[string]interface{} {
	interactive := map[string]interface{}{
		"type": "button",
		"body": map[string]interface{}{
			"text": content["body"],
		},
	}

	if header, ok := content["header"].(*MediaHeader); ok && header != nil {
		headerData := map[string]interface{}{
			"type": header.Type,
		}
		if header.URL != "" {
			headerData[header.Type] = map[string]string{"link": header.URL}
		} else if header.ID != "" {
			headerData[header.Type] = map[string]string{"id": header.ID}
		}
		interactive["header"] = headerData
	}

	if buttons, ok := content["buttons"].([]Button); ok && len(buttons) > 0 {
		buttonList := make([]map[string]interface{}, len(buttons))
		for i, btn := range buttons {
			buttonList[i] = map[string]interface{}{
				"type": btn.Type,
				"reply": map[string]string{
					"id":    btn.ID,
					"title": btn.Title,
				},
			}
		}
		interactive["action"] = map[string]interface{}{
			"buttons": buttonList,
		}
	}

	return interactive
}

// SendText sends a text message
func (c *WhatsAppClient) SendText(to, text string) (map[string]interface{}, error) {
	return c.Send(to, MessageTypeText, map[string]interface{}{
		"text": text,
	})
}

// SendImage sends an image message
func (c *WhatsAppClient) SendImage(to, url string) (map[string]interface{}, error) {
	return c.Send(to, MessageTypeImage, map[string]interface{}{
		"url": url,
	})
}

// SendAudio sends an audio message
func (c *WhatsAppClient) SendAudio(to, url string) (map[string]interface{}, error) {
	return c.Send(to, MessageTypeAudio, map[string]interface{}{
		"url": url,
	})
}

// SendVideo sends a video message
func (c *WhatsAppClient) SendVideo(to, url string) (map[string]interface{}, error) {
	return c.Send(to, MessageTypeVideo, map[string]interface{}{
		"url": url,
	})
}

// SendButtons sends an interactive button message
func (c *WhatsAppClient) SendButtons(to, body string, buttons []Button, header *MediaHeader) (map[string]interface{}, error) {
	content := map[string]interface{}{
		"body":    body,
		"buttons": buttons,
	}
	if header != nil {
		content["header"] = header
	}
	return c.Send(to, MessageTypeInteractive, content)
}

// SendTemplate sends a template message
func (c *WhatsAppClient) SendTemplate(to, templateName, language string, params []interface{}) (map[string]interface{}, error) {
	templateData := map[string]interface{}{
		"name": templateName,
		"language": map[string]string{
			"code": language,
		},
	}
	if params != nil {
		templateData["components"] = params
	}
	return c.Send(to, MessageTypeTemplate, templateData)
}

// DownloadMedia downloads media from WhatsApp
func (c *WhatsAppClient) DownloadMedia(mediaID, filename string) (string, error) {
	url := fmt.Sprintf("https://graph.facebook.com/%s/%s", c.config.APIVersion, mediaID)

	mediaInfo, err := c.request("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get media info: %w", err)
	}

	mediaURL, ok := mediaInfo["url"].(string)
	if !ok {
		return "", fmt.Errorf("media URL not found in response")
	}

	req, err := http.NewRequest("GET", mediaURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.config.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download media: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read media data: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return filename, nil
}

// IsProcessed checks if a message ID has been processed
func (c *WhatsAppClient) IsProcessed(messageID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.processedIDs[messageID]
}

// MarkProcessed marks a message ID as processed
func (c *WhatsAppClient) MarkProcessed(messageID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.processedIDs[messageID] = true
}

// Webhook data structures

// WebhookText represents text content in a webhook
type WebhookText struct {
	Body string `json:"body"`
}

// WebhookButtonReply represents a button reply
type WebhookButtonReply struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// WebhookListReply represents a list reply
type WebhookListReply struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// WebhookInteractive represents interactive message data
type WebhookInteractive struct {
	Type        string                `json:"type"`
	ButtonReply *WebhookButtonReply   `json:"button_reply,omitempty"`
	ListReply   *WebhookListReply     `json:"list_reply,omitempty"`
}

// WebhookAudio represents audio message data
type WebhookAudio struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	Voice    bool   `json:"voice,omitempty"`
}

// WebhookImage represents image message data
type WebhookImage struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256,omitempty"`
}

// WebhookVideo represents video message data
type WebhookVideo struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256,omitempty"`
}

// WebhookMessage represents a message in the webhook
type WebhookMessage struct {
	From        string              `json:"from"`
	ID          string              `json:"id"`
	Timestamp   string              `json:"timestamp"`
	Type        string              `json:"type"`
	Text        *WebhookText        `json:"text,omitempty"`
	Interactive *WebhookInteractive `json:"interactive,omitempty"`
	Audio       *WebhookAudio       `json:"audio,omitempty"`
	Image       *WebhookImage       `json:"image,omitempty"`
	Video       *WebhookVideo       `json:"video,omitempty"`
}

// WebhookContact represents contact information
type WebhookContact struct {
	WaID    string                 `json:"wa_id"`
	Profile map[string]interface{} `json:"profile,omitempty"`
}

// WebhookMetadata represents metadata in the webhook
type WebhookMetadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

// WebhookValue represents the value in a change
type WebhookValue struct {
	MessagingProduct string            `json:"messaging_product"`
	Metadata         WebhookMetadata   `json:"metadata"`
	Contacts         []WebhookContact  `json:"contacts,omitempty"`
	Messages         []WebhookMessage  `json:"messages,omitempty"`
}

// WebhookChange represents a change in the webhook
type WebhookChange struct {
	Value WebhookValue `json:"value"`
	Field string       `json:"field"`
}

// WebhookEntry represents an entry in the webhook
type WebhookEntry struct {
	ID      string          `json:"id"`
	Changes []WebhookChange `json:"changes"`
}

// WebhookData represents the complete webhook payload
type WebhookData struct {
	Object string         `json:"object"`
	Entry  []WebhookEntry `json:"entry"`
}

// WebhookHandler handles incoming webhook messages
type WebhookHandler struct {
	client *WhatsAppClient
	data   *WebhookData
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(client *WhatsAppClient) *WebhookHandler {
	return &WebhookHandler{
		client: client,
	}
}

// Parse parses webhook data
func (h *WebhookHandler) Parse(webhookData []byte) error {
	h.data = &WebhookData{}
	if err := json.Unmarshal(webhookData, h.data); err != nil {
		return fmt.Errorf("failed to parse webhook data: %w", err)
	}
	return nil
}

// Message returns the first message from the webhook
func (h *WebhookHandler) Message() *WebhookMessage {
	if h.data == nil || len(h.data.Entry) == 0 {
		return nil
	}
	entry := h.data.Entry[0]
	if len(entry.Changes) == 0 {
		return nil
	}
	change := entry.Changes[0]
	if len(change.Value.Messages) == 0 {
		return nil
	}
	return &change.Value.Messages[0]
}

// Sender returns the sender's phone number
func (h *WebhookHandler) Sender() string {
	msg := h.Message()
	if msg == nil {
		return ""
	}
	return msg.From
}

// Text returns the text content of the message
func (h *WebhookHandler) Text() string {
	msg := h.Message()
	if msg == nil {
		return ""
	}

	if msg.Text != nil {
		return msg.Text.Body
	}

	if msg.Interactive != nil {
		if msg.Interactive.ButtonReply != nil {
			return msg.Interactive.ButtonReply.Title
		}
		if msg.Interactive.ListReply != nil {
			return msg.Interactive.ListReply.Title
		}
	}

	return ""
}

// MessageType returns the type of the message
func (h *WebhookHandler) MessageType() string {
	msg := h.Message()
	if msg == nil {
		return ""
	}
	return msg.Type
}

// AudioID returns the audio media ID
func (h *WebhookHandler) AudioID() string {
	msg := h.Message()
	if msg == nil || msg.Audio == nil {
		return ""
	}
	return msg.Audio.ID
}

// ImageID returns the image media ID
func (h *WebhookHandler) ImageID() string {
	msg := h.Message()
	if msg == nil || msg.Image == nil {
		return ""
	}
	return msg.Image.ID
}

// VideoID returns the video media ID
func (h *WebhookHandler) VideoID() string {
	msg := h.Message()
	if msg == nil || msg.Video == nil {
		return ""
	}
	return msg.Video.ID
}

// MessageID returns the unique message ID
func (h *WebhookHandler) MessageID() string {
	msg := h.Message()
	if msg == nil {
		return ""
	}
	return msg.ID
}

// DisplayNumber returns the display phone number
func (h *WebhookHandler) DisplayNumber() string {
	if h.data == nil || len(h.data.Entry) == 0 {
		return ""
	}
	entry := h.data.Entry[0]
	if len(entry.Changes) == 0 {
		return ""
	}
	return entry.Changes[0].Value.Metadata.DisplayPhoneNumber
}

// ContactName returns the contact's name from profile
func (h *WebhookHandler) ContactName() string {
	if h.data == nil || len(h.data.Entry) == 0 {
		return ""
	}
	entry := h.data.Entry[0]
	if len(entry.Changes) == 0 {
		return ""
	}
	contacts := entry.Changes[0].Value.Contacts
	if len(contacts) == 0 || contacts[0].Profile == nil {
		return ""
	}
	if name, ok := contacts[0].Profile["name"].(string); ok {
		return name
	}
	return ""
}

// IsDuplicate checks if this message has already been processed
func (h *WebhookHandler) IsDuplicate() bool {
	msg := h.Message()
	if msg == nil {
		return false
	}

	if h.client.IsProcessed(msg.ID) {
		return true
	}

	h.client.MarkProcessed(msg.ID)
	return false
}

// ReplyText sends a text reply to the sender
func (h *WebhookHandler) ReplyText(text string) (map[string]interface{}, error) {
	return h.client.SendText(h.Sender(), text)
}

// ReplyImage sends an image reply to the sender
func (h *WebhookHandler) ReplyImage(url string) (map[string]interface{}, error) {
	return h.client.SendImage(h.Sender(), url)
}

// ReplyAudio sends an audio reply to the sender
func (h *WebhookHandler) ReplyAudio(url string) (map[string]interface{}, error) {
	return h.client.SendAudio(h.Sender(), url)
}

// ReplyButtons sends an interactive button reply to the sender
func (h *WebhookHandler) ReplyButtons(body string, buttons []Button, header *MediaHeader) (map[string]interface{}, error) {
	return h.client.SendButtons(h.Sender(), body, buttons, header)
}

// WhatsAppSDK is the main SDK entry point
type WhatsAppSDK struct {
	Client *WhatsAppClient
}

// NewWhatsAppSDK creates a new WhatsApp SDK instance
func NewWhatsAppSDK(token, phoneID string) *WhatsAppSDK {
	config := NewConfig()
	if token != "" {
		config.Token = token
	}
	if phoneID != "" {
		config.PhoneID = phoneID
		config.BaseURL = fmt.Sprintf("https://graph.facebook.com/%s/%s/messages", config.APIVersion, phoneID)
	}

	return &WhatsAppSDK{
		Client: NewWhatsAppClient(config),
	}
}

// WebhookHandler creates a new webhook handler
func (sdk *WhatsAppSDK) WebhookHandler() *WebhookHandler {
	return NewWebhookHandler(sdk.Client)
}

// QuickSend sends a quick text message
func (sdk *WhatsAppSDK) QuickSend(to, text string) (map[string]interface{}, error) {
	return sdk.Client.SendText(to, text)
}

// SendMenu sends a message with button options
func (sdk *WhatsAppSDK) SendMenu(to, body string, options map[string]string) (map[string]interface{}, error) {
	buttons := make([]Button, 0, 3)
	count := 0
	for id, title := range options {
		if count >= 3 {
			break
		}
		buttons = append(buttons, NewButton(id, title))
		count++
	}
	return sdk.Client.SendButtons(to, body, buttons, nil)
}

// SendMediaMenu sends a message with media header and button options
func (sdk *WhatsAppSDK) SendMediaMenu(to, body, mediaURL, mediaType string, options map[string]string) (map[string]interface{}, error) {
	buttons := make([]Button, 0, 3)
	count := 0
	for id, title := range options {
		if count >= 3 {
			break
		}
		buttons = append(buttons, NewButton(id, title))
		count++
	}
	header := &MediaHeader{
		Type: mediaType,
		URL:  mediaURL,
	}
	return sdk.Client.SendButtons(to, body, buttons, header)
}
