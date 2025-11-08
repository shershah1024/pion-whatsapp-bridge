# WhatsApp Messaging SDK for Go

A comprehensive Go SDK for handling WhatsApp messages, inspired by the Python SDK in horizon-german-whatsapp-bot. This SDK provides easy-to-use functions for sending and receiving various types of WhatsApp messages.

**✨ Fully integrated with the existing WhatsApp Call Bridge!** The same `/whatsapp-call` webhook endpoint handles both voice calls AND text messages.

## Features

- ✅ **Send Messages**: Text, images, audio, video, documents
- ✅ **Interactive Messages**: Buttons, lists, menus
- ✅ **Template Messages**: Send pre-approved template messages
- ✅ **Media Download**: Download audio, images, and videos from users
- ✅ **Webhook Handling**: Parse incoming messages with helper methods (unified with call webhooks)
- ✅ **Duplicate Detection**: Automatically track and ignore duplicate messages
- ✅ **Thread-Safe**: Concurrent-safe message processing
- ✅ **Integrated Commands**: Text messages can trigger voice calls and vice versa

## Quick Start

### 1. Environment Variables

Set these environment variables:

```bash
export WHATSAPP_TOKEN="your_whatsapp_api_token"
export PHONE_NUMBER_ID="your_phone_number_id"
```

### 2. Initialize SDK

```go
// Uses environment variables automatically
wa := NewWhatsAppSDK("", "")

// Or provide credentials explicitly
wa := NewWhatsAppSDK("your_token", "your_phone_id")
```

### 3. Send a Message

```go
// Send simple text
wa.QuickSend("+1234567890", "Hello World!")

// Send image
wa.Client.SendImage("+1234567890", "https://example.com/image.jpg")

// Send audio
wa.Client.SendAudio("+1234567890", "https://example.com/audio.mp3")
```

## Sending Messages

### Text Messages

```go
resp, err := wa.Client.SendText("+1234567890", "Hello! This is a test message.")
if err != nil {
    log.Printf("Error: %v", err)
}
```

### Media Messages

```go
// Image
wa.Client.SendImage("+1234567890", "https://example.com/image.jpg")

// Audio
wa.Client.SendAudio("+1234567890", "https://example.com/audio.mp3")

// Video
wa.Client.SendVideo("+1234567890", "https://example.com/video.mp4")
```

### Interactive Button Messages

```go
// Create buttons (max 3)
buttons := []Button{
    NewButton("btn1", "Option 1"),
    NewButton("btn2", "Option 2"),
    NewButton("btn3", "Option 3"),
}

// Send button message
wa.Client.SendButtons(
    "+1234567890",
    "Please choose an option:",
    buttons,
    nil, // no header
)
```

### Button Message with Media Header

```go
// Create buttons
buttons := []Button{
    NewButton("learn", "Learn"),
    NewButton("practice", "Practice"),
}

// Create media header
header := &MediaHeader{
    Type: "image",
    URL:  "https://example.com/header.jpg",
}

// Send with header
wa.Client.SendButtons(
    "+1234567890",
    "Choose your path:",
    buttons,
    header,
)
```

### Quick Menu (Helper Method)

```go
// Send menu with up to 3 options
options := map[string]string{
    "beginner":     "Beginner",
    "intermediate": "Intermediate",
    "advanced":     "Advanced",
}

wa.SendMenu("+1234567890", "Choose your level:", options)
```

### Template Messages

```go
// Send pre-approved template
wa.Client.SendTemplate(
    "+1234567890",
    "welcome_template",  // template name
    "en",                // language code
    nil,                 // parameters (optional)
)
```

## Receiving Messages (Webhook Handling)

### Unified Webhook Endpoint

**The messaging SDK is already integrated into the existing `/whatsapp-call` webhook!**

The same endpoint that handles voice calls also handles text messages. When you configure your WhatsApp webhook URL, point it to:

```
https://your-domain.com/whatsapp-call
```

This single webhook handles:
- ✅ Voice call events (connect, terminate, ringing, etc.)
- ✅ Text messages
- ✅ Interactive messages (buttons, lists)
- ✅ Media messages (audio, images, videos)
- ✅ Status updates

The bridge automatically routes each event type to the appropriate handler based on the webhook payload structure.

### Parse Incoming Messages

```go
func HandleIncomingMessage(w http.ResponseWriter, r *http.Request) {
    wa := NewWhatsAppSDK("", "")
    handler := wa.WebhookHandler()

    // Read webhook data
    var webhookData map[string]interface{}
    json.NewDecoder(r.Body).Decode(&webhookData)

    // Parse
    jsonData, _ := json.Marshal(webhookData)
    handler.Parse(jsonData)

    // Check for duplicates
    if handler.IsDuplicate() {
        w.WriteHeader(http.StatusOK)
        return
    }

    // Get message details
    sender := handler.Sender()           // Phone number
    text := handler.Text()               // Message text
    msgType := handler.MessageType()     // "text", "audio", "image", etc.
    contactName := handler.ContactName() // User's name

    // Handle the message
    switch msgType {
    case "text":
        handleTextMessage(handler, text)
    case "audio":
        handleAudioMessage(handler)
    case "image":
        handleImageMessage(handler)
    }

    w.WriteHeader(http.StatusOK)
}
```

### Helper Methods for Webhooks

```go
handler := wa.WebhookHandler()
handler.Parse(webhookData)

// Extract information
sender := handler.Sender()           // "+1234567890"
text := handler.Text()               // Message text or button selection
msgType := handler.MessageType()     // "text", "audio", "image", "video", "interactive"
contactName := handler.ContactName() // User's display name
audioID := handler.AudioID()         // Media ID for audio
imageID := handler.ImageID()         // Media ID for image
videoID := handler.VideoID()         // Media ID for video
displayNum := handler.DisplayNumber() // Business number

// Reply methods
handler.ReplyText("Thanks for your message!")
handler.ReplyImage("https://example.com/image.jpg")
handler.ReplyAudio("https://example.com/audio.mp3")

// Reply with buttons
buttons := []Button{NewButton("yes", "Yes"), NewButton("no", "No")}
handler.ReplyButtons("Continue?", buttons, nil)
```

## Downloading Media

### Download Audio/Image/Video

```go
func handleAudioMessage(handler *WebhookHandler) {
    audioID := handler.AudioID()
    if audioID == "" {
        return
    }

    // Download the file
    filename := "audio_" + audioID + ".ogg"
    savedPath, err := handler.client.DownloadMedia(audioID, filename)
    if err != nil {
        log.Printf("Error: %v", err)
        return
    }

    log.Printf("Saved to: %s", savedPath)
    handler.ReplyText("Thanks for the audio!")

    // Now you can:
    // - Transcribe with OpenAI Whisper
    // - Process for voice commands
    // - Store in database
}
```

## Message Types and Structures

### Supported Message Types

```go
const (
    MessageTypeText        MessageType = "text"
    MessageTypeImage       MessageType = "image"
    MessageTypeAudio       MessageType = "audio"
    MessageTypeVideo       MessageType = "video"
    MessageTypeDocument    MessageType = "document"
    MessageTypeInteractive MessageType = "interactive"
    MessageTypeTemplate    MessageType = "template"
)
```

### Webhook Message Structure

```go
type WebhookMessage struct {
    From        string              // Sender's phone number
    ID          string              // Unique message ID
    Timestamp   string              // Message timestamp
    Type        string              // Message type
    Text        *WebhookText        // Text content (if text message)
    Interactive *WebhookInteractive // Button/list reply (if interactive)
    Audio       *WebhookAudio       // Audio metadata (if audio)
    Image       *WebhookImage       // Image metadata (if image)
    Video       *WebhookVideo       // Video metadata (if video)
}
```

## Integration with Existing Bridge

### Already Integrated! ✅

The messaging SDK is **already fully integrated** into `main.go`!

The existing `/whatsapp-call` webhook endpoint automatically:
1. Detects message events in incoming webhooks
2. Routes them to `handleMessageEvents()`
3. Processes text, audio, image, video messages
4. Handles interactive buttons
5. Downloads media files
6. Sends appropriate replies

### How It Works

In `main.go`, the `processWebhook()` function checks the webhook structure:

```go
// Detects call events
if calls, ok := value["calls"].([]interface{}); ok && len(calls) > 0 {
    // Handle voice calls
    for _, call := range calls {
        b.handleCallEvent(callData)
    }
} else if messages, ok := value["messages"].([]interface{}); ok && len(messages) > 0 {
    // Handle text/media messages using SDK
    go b.handleMessageEvents(webhook)
}
```

### Built-in Commands

The integration includes these commands out of the box:

| Command | Response |
|---------|----------|
| `help`, `hi`, `hello` | Shows welcome message with available commands |
| `menu` | Shows interactive button menu |
| `call` | Instructions to start a voice call |
| `status` | Shows bridge status and active call count |
| Any other text | Echoes back the message |

### Button Actions

When users click buttons, these actions are triggered:

| Button | Action |
|--------|--------|
| "Call Me" | Initiates an outbound voice call to the user |
| "Check Status" | Shows current bridge status |
| "Help" | Shows help message |

## Example Use Cases

### 1. Language Learning Bot

```go
func handleTextMessage(handler *WebhookHandler, text string) {
    switch text {
    case "start":
        buttons := []Button{
            NewButton("a1", "A1 - Beginner"),
            NewButton("b1", "B1 - Intermediate"),
            NewButton("c1", "C1 - Advanced"),
        }
        handler.ReplyButtons("Choose your level:", buttons, nil)

    case "practice":
        // Send audio exercise
        handler.ReplyAudio("https://example.com/exercises/lesson1.mp3")
        handler.ReplyText("Listen and repeat!")

    default:
        handler.ReplyText("Send 'start' to begin learning!")
    }
}
```

### 2. Voice Message Processing

```go
func handleAudioMessage(handler *WebhookHandler) {
    audioID := handler.AudioID()

    // Download audio
    filename := "audio_" + audioID + ".ogg"
    savedPath, _ := handler.client.DownloadMedia(audioID, filename)

    // Transcribe using OpenAI Whisper
    transcription := transcribeAudio(savedPath)

    // Send transcription back
    handler.ReplyText("You said: " + transcription)

    // Process the command
    processVoiceCommand(handler, transcription)
}
```

### 3. Image Analysis

```go
func handleImageMessage(handler *WebhookHandler) {
    imageID := handler.ImageID()

    // Download image
    filename := "image_" + imageID + ".jpg"
    savedPath, _ := handler.client.DownloadMedia(imageID, filename)

    // Analyze using GPT-4 Vision
    description := analyzeImageWithGPT4(savedPath)

    handler.ReplyText("I see: " + description)
}
```

## API Reference

### WhatsAppSDK

| Method | Description |
|--------|-------------|
| `NewWhatsAppSDK(token, phoneID)` | Create new SDK instance |
| `QuickSend(to, text)` | Send quick text message |
| `SendMenu(to, body, options)` | Send button menu (max 3 options) |
| `SendMediaMenu(to, body, mediaURL, mediaType, options)` | Send menu with media header |
| `WebhookHandler()` | Create webhook handler |

### WhatsAppClient

| Method | Description |
|--------|-------------|
| `SendText(to, text)` | Send text message |
| `SendImage(to, url)` | Send image message |
| `SendAudio(to, url)` | Send audio message |
| `SendVideo(to, url)` | Send video message |
| `SendButtons(to, body, buttons, header)` | Send interactive buttons |
| `SendTemplate(to, name, lang, params)` | Send template message |
| `DownloadMedia(mediaID, filename)` | Download media file |

### WebhookHandler

| Method | Description |
|--------|-------------|
| `Parse(webhookData)` | Parse webhook JSON |
| `Sender()` | Get sender phone number |
| `Text()` | Get message text or button selection |
| `MessageType()` | Get message type |
| `AudioID()` | Get audio media ID |
| `ImageID()` | Get image media ID |
| `VideoID()` | Get video media ID |
| `ContactName()` | Get sender's name |
| `IsDuplicate()` | Check if message is duplicate |
| `ReplyText(text)` | Reply with text |
| `ReplyImage(url)` | Reply with image |
| `ReplyAudio(url)` | Reply with audio |
| `ReplyButtons(body, buttons, header)` | Reply with buttons |

## Testing

### Test Sending Messages

```bash
# Build and run (SDK is already integrated into main.go)
go build -o pion-whatsapp-bridge
./pion-whatsapp-bridge

# Or use the deploy script
./deploy.sh
```

### Test Message Reception

1. **Send a text message** to your WhatsApp Business number:
   ```
   User: help
   Bot: 👋 Welcome to WhatsApp Voice Bridge!
        Commands:
        • help - Show this message
        • menu - View options
        • call - Start a voice call
        • status - Check bridge status
   ```

2. **Try the menu**:
   ```
   User: menu
   Bot: [Shows 3 buttons: "Call Me", "Check Status", "Help"]
   ```

3. **Send a voice message** - it will be downloaded and acknowledged

4. **Send an image** - it will be downloaded and acknowledged

### Test Webhook Setup

```bash
# For local testing, use ngrok:
ngrok http 3011

# Configure webhook in WhatsApp Business API Dashboard:
# URL: https://your-ngrok-url.ngrok.io/whatsapp-call
# Verify Token: whatsapp_bridge_token
# Subscribe to: messages, calls
```

## Configuration

### WhatsApp Business API Setup

1. **Get credentials** from Meta Developer Dashboard
2. **Set webhook URL**: `https://your-domain.com/whatsapp-call` (same as calls!)
3. **Subscribe to**: Both `messages` AND `calls` fields
4. **Verify token**: Use same token as in code (default: `whatsapp_bridge_token`)

### Environment Variables

```bash
# Required
export WHATSAPP_TOKEN="EAAxxxxx"           # Your access token
export PHONE_NUMBER_ID="123456789"         # Your phone number ID

# Optional
export VERIFY_TOKEN="whatsapp_bridge_token"  # Webhook verification token
```

## Common Patterns

### Command Handler

```go
func handleTextMessage(handler *WebhookHandler, text string) {
    commands := map[string]func(){
        "help": func() {
            handler.ReplyText("Available commands: help, menu, call")
        },
        "menu": func() {
            buttons := []Button{
                NewButton("opt1", "Option 1"),
                NewButton("opt2", "Option 2"),
            }
            handler.ReplyButtons("Choose:", buttons, nil)
        },
        "call": func() {
            handler.ReplyText("Starting voice call...")
            // Integrate with existing call functionality
        },
    }

    if cmd, exists := commands[text]; exists {
        cmd()
    } else {
        handler.ReplyText("Unknown command. Try 'help'")
    }
}
```

### State Machine for Conversations

```go
type UserState struct {
    State string
    Data  map[string]interface{}
}

var userStates = make(map[string]*UserState)

func handleConversation(handler *WebhookHandler, text string) {
    sender := handler.Sender()
    state, exists := userStates[sender]

    if !exists {
        state = &UserState{State: "start", Data: make(map[string]interface{})}
        userStates[sender] = state
    }

    switch state.State {
    case "start":
        handler.ReplyText("What's your name?")
        state.State = "awaiting_name"

    case "awaiting_name":
        state.Data["name"] = text
        handler.ReplyText("Nice to meet you, " + text + "! What's your age?")
        state.State = "awaiting_age"

    case "awaiting_age":
        state.Data["age"] = text
        handler.ReplyText("Great! All set up.")
        state.State = "complete"
    }
}
```

## Comparison with Python SDK

| Feature | Python SDK | Go SDK |
|---------|-----------|--------|
| Send Text | ✅ `send_text()` | ✅ `SendText()` |
| Send Media | ✅ `send_image/audio()` | ✅ `SendImage/Audio()` |
| Send Buttons | ✅ `send_buttons()` | ✅ `SendButtons()` |
| Templates | ✅ `send_template()` | ✅ `SendTemplate()` |
| Download Media | ✅ `download_media()` | ✅ `DownloadMedia()` |
| Webhook Parsing | ✅ `WebhookHandler` | ✅ `WebhookHandler` |
| Duplicate Detection | ✅ `is_duplicate()` | ✅ `IsDuplicate()` |
| Async/Await | ✅ Python asyncio | 🔄 Goroutines |
| Type Safety | ⚠️ Dynamic | ✅ Strongly typed |

## Files

- **whatsapp_sdk.go** - Main SDK implementation
- **whatsapp_sdk_example.go** - Usage examples
- **MESSAGING_SDK.md** - This documentation

## Contributing

See the main `CLAUDE.md` for contribution guidelines.

## License

Same as the main project.
