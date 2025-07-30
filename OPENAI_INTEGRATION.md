# OpenAI Realtime API Integration

This document describes how WhatsApp calls are connected to OpenAI's Realtime API for AI-powered voice conversations.

## Architecture Overview

```
WhatsApp Caller → WebRTC → Pion Bridge → WebRTC → OpenAI Realtime API
                     ↑                        ↓
                  Audio In                 AI Response
```

## Key Components

### 1. Dual WebRTC Connections
- **WhatsApp Connection**: Receives calls via webhook, passive ICE mode
- **OpenAI Connection**: Active WebRTC client connecting to OpenAI's servers

### 2. Audio Bridging
- **WhatsApp → OpenAI**: RTP packets forwarded directly
- **OpenAI → WhatsApp**: Audio track bridged between connections

### 3. Data Channel Communication
- Channel name: `oai-events`
- Handles session configuration and real-time events
- Supports text/audio responses and transcriptions

## Connection Flow

1. **WhatsApp Call Received**
   - Webhook delivers SDP offer
   - Bridge accepts call and establishes WebRTC connection

2. **OpenAI Connection Setup**
   - Fetch ephemeral token using API key
   - Create WebRTC offer with audio track
   - Exchange SDP with OpenAI endpoint
   - Open data channel for events

3. **Audio Bridging**
   - Forward WhatsApp audio to OpenAI in real-time
   - Stream OpenAI responses back to caller
   - Handle transcriptions and events via data channel

## Configuration

Set the `OPENAI_API_KEY` environment variable to enable AI voice assistant:

```bash
export OPENAI_API_KEY=your_api_key
```

## Technical Details

### OpenAI WebRTC Configuration
- **Endpoint**: `https://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-12-17`
- **Audio Format**: Opus codec for both directions
- **Data Channel**: JSON events for session control

### Voice Settings
- **Model**: GPT-4o Realtime Preview
- **Voice**: Alloy (configurable)
- **Turn Detection**: Server VAD with 200ms silence threshold
- **Transcription**: Whisper model for input audio

### Event Types Handled
- `session.created`: Connection established
- `session.updated`: Configuration applied
- `response.audio.delta`: Audio chunks from AI
- `response.text.delta`: Text responses
- `response.audio_transcript.delta`: Transcription updates

## Implementation Notes

1. **Ephemeral Tokens**: Valid for 1 minute, fetched per call
2. **Audio Forwarding**: Direct RTP packet forwarding (no transcoding)
3. **Connection Timing**: 500ms delay after accept for stability
4. **Error Handling**: Graceful fallback if OpenAI unavailable

## Testing

1. Set your OpenAI API key
2. Make a WhatsApp call to your business number
3. Speak to the AI assistant
4. Check logs for transcriptions and events

## Troubleshooting

- **No AI Response**: Check API key and ephemeral token generation
- **Audio Issues**: Verify both WebRTC connections are established
- **Connection Failures**: Check network and STUN server accessibility