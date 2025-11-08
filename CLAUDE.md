# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A pure Go WebRTC bridge for handling WhatsApp voice calls using Pion WebRTC. This system receives WhatsApp call webhooks, establishes WebRTC connections, and enables real-time voice communication between WhatsApp users and AI voice assistants (OpenAI/Azure OpenAI Realtime API).

**Why Pion over Janus Gateway:**
- Native ice-lite support built-in
- Pure Go with single binary deployment (no C dependencies)
- Direct programmatic control over SDP and RTP
- Better suited for custom/non-standard WebRTC implementations

## Development Commands

### Building and Running
```bash
# Build the application
go build -o pion-whatsapp-bridge

# Run locally
go run main.go

# Deploy (builds, starts server, creates ngrok tunnel)
./deploy.sh

# Test webhook verification
./test-webhook.sh

# Test outbound call initiation
./test_outbound_call.sh
```

### Testing
```bash
# Test webhook verification endpoint
curl 'http://localhost:3011/whatsapp-call?hub.mode=subscribe&hub.verify_token=whatsapp_bridge_token&hub.challenge=test123'

# Test with SDP offer
./test.sh

# Check status endpoint
curl http://localhost:3011/status

# Health check
curl http://localhost:3011/health
```

## Architecture

### Core Components

**main.go** - Main application with:
- `WhatsAppBridge` - Core bridge managing WebRTC connections and WhatsApp API calls
- `Call` - Represents an active call session with peer connection, audio track, and optional OpenAI client
- Inbound call flow: Webhook → Accept → WebRTC connection → Audio forwarding
- Outbound call flow: Create offer → WhatsApp API → User answers → Set remote SDP → Audio forwarding

**openai_realtime.go** - OpenAI Realtime API integration:
- `OpenAIRealtimeClient` - Manages WebRTC connection to OpenAI/Azure OpenAI
- Dual WebRTC setup: One connection to WhatsApp, one to OpenAI
- Bidirectional audio forwarding via RTP packets
- Data channel for Realtime API events (session config, transcriptions, function calls)
- Weather function implementation as example tool

**audio_processor.go** - Audio processing utilities (if present)

### WebRTC Configuration Details

**Critical WhatsApp-Specific Requirements:**
1. **ICE Mode**: WhatsApp uses `ice-lite` in their offer (passive mode), but WE must be a full ICE agent (active mode)
2. **DTLS Role**: Set `DTLSRoleClient` (active) in answer when WhatsApp uses `setup:actpass`
3. **Codec**: Opus 48kHz stereo with exact fmtp parameters matching WhatsApp's SDP
4. **Extensions**: Must register RTP header extensions that WhatsApp uses:
   - `urn:ietf:params:rtp-hdrext:ssrc-audio-level`
   - `http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time`
   - `http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01`

**MediaEngine Setup (main.go:47-94):**
```go
// Register Opus with WhatsApp's exact parameters
opusCodec := webrtc.RTPCodecParameters{
    RTPCodecCapability: webrtc.RTPCodecCapability{
        MimeType:    webrtc.MimeTypeOpus,
        ClockRate:   48000,
        Channels:    2,
        SDPFmtpLine: "maxaveragebitrate=20000;maxplaybackrate=16000;minptime=20;sprop-maxcapturerate=16000;useinbandfec=1",
    },
    PayloadType: 111,
}
```

**SettingEngine Configuration (main.go:96-104):**
- Do NOT use `SetLite(true)` - WhatsApp is ice-lite, we must be full ICE
- Set `SetAnsweringDTLSRole(webrtc.DTLSRoleClient)` to be active/client
- Use UDP only (WhatsApp doesn't use TCP)
- Configure STUN servers for ICE candidate gathering

### Call Flow Diagrams

**Inbound Call (User → Business):**
```
1. User initiates call
2. WhatsApp → Webhook (POST /whatsapp-call) with SDP offer
3. Create PeerConnection, set remote description
4. Create SDP answer with ice-lite removed, setup:active
5. Wait for ICE gathering complete
6. Send pre_accept to WhatsApp API with SDP answer
7. Send accept immediately after (no delay between pre_accept and accept)
8. WebRTC connection established
9. Connect to OpenAI (if AZURE_OPENAI_API_KEY set)
10. Bidirectional audio: WhatsApp ↔ OpenAI
```

**Outbound Call (Business → User):**
```
1. POST /initiate-call with phone number
2. Create PeerConnection with audio track
3. Create SDP offer
4. WhatsApp API connect action with SDP offer
5. User's phone rings
6. User accepts → Webhook with SDP answer
7. Set remote description (user's answer)
8. OpenAI already pre-connected (started during ring)
9. Bidirectional audio: WhatsApp ↔ OpenAI
```

### Audio Forwarding Architecture

**WhatsApp → OpenAI (main.go:559-627):**
- OnTrack handler receives WhatsApp audio
- Forwards raw RTP packets to OpenAI via `ForwardRTPToOpenAI()`
- No transcoding - direct Opus packet forwarding

**OpenAI → WhatsApp (main.go:1040-1117):**
- Wait for OpenAI remote audio track
- Read RTP packets using `ReadRTP()` to get full packet structure
- Marshal to bytes and write to WhatsApp track
- Logs every 5 seconds to avoid spam

**Key Pattern:**
```go
// Reading from track (preserves RTP structure)
rtpPacket, _, err := track.ReadRTP()
rtpBytes, _ := rtpPacket.Marshal()
whatsappTrack.Write(rtpBytes)

// NOT just reading payload - we need full RTP packet
```

## Environment Variables

**Required:**
- `WHATSAPP_TOKEN` - WhatsApp Cloud API access token
- `PHONE_NUMBER_ID` - WhatsApp Business phone number ID
- `VERIFY_TOKEN` - Webhook verification token (default: "whatsapp_bridge_token")

**Optional:**
- `AZURE_OPENAI_API_KEY` - Azure OpenAI API key (enables AI voice assistant)
- `AZURE_OPENAI_ENDPOINT` - Azure OpenAI endpoint URL
- `AZURE_OPENAI_DEPLOYMENT` - Azure OpenAI deployment name
- `OPENAI_API_KEY` - OpenAI API key (alternative to Azure)
- `ENABLE_ECHO` - Set to "true" to echo audio back to caller (testing)
- `PORT` - Server port (default: 3011)

## API Endpoints

- `GET /whatsapp-call` - Webhook verification (responds with challenge)
- `POST /whatsapp-call` - Webhook events (call events, status updates)
- `POST /test-call` - Test endpoint with SDP support
- `POST /initiate-call` - Initiate outbound call (body: `{"to": "14085551234"}`)
- `GET /status` - Bridge status and active calls count
- `GET /health` - Health check

## WhatsApp API Integration

**API Base URL:** `https://graph.facebook.com/v21.0/{PHONE_NUMBER_ID}/calls`

**Inbound Call Actions:**
1. `pre_accept` - Establish WebRTC connection with SDP answer
2. `accept` - Start media flow (sent immediately after pre_accept)

**Outbound Call Actions:**
1. `connect` - Initiate call with SDP offer
2. `accept` - Finalize connection after user answers
3. `terminate` - End call

**Webhook Event Types:**
- `connect` - Call connected (inbound: has offer, outbound: has answer)
- `terminate` - Call ended
- `ringing` - Outbound call is ringing
- `answered` - Call answered on another device
- Status updates with ACCEPTED, FAILED, etc.

## Azure OpenAI Realtime API Integration

**Session Creation:**
- Fetch ephemeral token from `/openai/realtimeapi/sessions` endpoint
- Uses `api-key` header (not Bearer token) for session creation
- Bearer token used for WebRTC endpoint

**WebRTC Connection:**
- Endpoint: `https://eastus2.realtimeapi-preview.ai.azure.com/v1/realtimertc?model={deployment}`
- POST with SDP offer as `application/sdp`
- Response contains SDP answer

**Configuration:**
- Voice: "alloy"
- Model: From `AZURE_OPENAI_DEPLOYMENT`
- Turn detection: Server VAD (threshold 0.5, silence 100ms)
- Input transcription: Whisper model
- Instructions: Weather assistant in English only
- Tools: `get_weather` function

**Data Channel Events:**
- `session.created` - Triggers immediate greeting
- `session.updated` - Configuration applied
- `response.output_audio.delta` - Audio from AI
- `input_audio_buffer.speech_started/stopped` - VAD events
- `conversation.item.input_audio_transcription.completed` - Transcription results
- `response.function_call_arguments.done` - Function call requests

## Common Issues and Solutions

### Issue: Audio not flowing bidirectionally
**Problem:** WhatsApp doesn't send audio until it receives valid audio first
**Solution:** Ensure audio track is added before creating answer, use `AddTrack()` not just transceiver

### Issue: Codec mismatch errors
**Problem:** SDP parameters don't match WhatsApp's requirements
**Solution:** Use exact fmtp line from WhatsApp's offer (main.go:57)

### Issue: ICE connection fails
**Problem:** Using ice-lite in answer or wrong DTLS role
**Solution:** Be full ICE agent, use STUN servers, set DTLSRoleClient

### Issue: Call disconnects after ~20 seconds
**Problem:** WhatsApp received no valid audio and times out
**Solution:** Verify OpenAI audio is being forwarded correctly, check RTP packet marshaling

### Issue: SDP parsing fails
**Problem:** Missing RTP header extension registrations
**Solution:** Register all extensions WhatsApp uses (main.go:82-93)

## Deployment

**Railway (Production):**
1. Push to GitHub
2. Deploy from Railway dashboard
3. Configure environment variables
4. Get permanent HTTPS URL from Railway
5. Configure WhatsApp webhook URL

**Local Development:**
1. Run `./deploy.sh` to start server and ngrok tunnel
2. Configure WhatsApp webhook with ngrok URL
3. Make test call to your WhatsApp Business number

## Important File References

- **WEBRTC_FLOW.md** - Detailed WebRTC connection flow and common mistakes
- **OPENAI_INTEGRATION.md** - OpenAI Realtime API integration details
- **OUTBOUND_CALLS_GUIDE.md** - Complete guide for business-initiated calls
- **CURRENT_STATUS.md** - Latest debugging status and known issues
- **TROUBLESHOOTING.md** - Common problems and solutions

## Code Patterns to Follow

### Error Handling for WhatsApp API Calls
Always log full request/response for debugging:
```go
log.Printf("📤 WhatsApp API request: %s", jsonData)
resp, err := client.Do(req)
body, _ := io.ReadAll(resp.Body)
log.Printf("📡 WhatsApp API response: Status=%d, Body=%s", resp.StatusCode, body)
```

### SDP Processing
Clean and validate SDP before parsing:
```go
sdpOffer = strings.TrimSpace(sdpOffer)
sdpOffer = strings.ReplaceAll(sdpOffer, "\\r\\n", "\r\n")
// Ensure ends with newline
if !strings.HasSuffix(sdpOffer, "\n") {
    sdpOffer += "\r\n"
}
```

### Call State Management
Use mutex for concurrent access to active calls map:
```go
b.mu.Lock()
b.activeCalls[callID] = call
b.mu.Unlock()
```

### Audio Track Creation
Match codec parameters exactly:
```go
audioTrack, err := webrtc.NewTrackLocalStaticRTP(
    webrtc.RTPCodecCapability{
        MimeType:    webrtc.MimeTypeOpus,
        ClockRate:   48000,
        Channels:    2,
        SDPFmtpLine: "maxaveragebitrate=20000;maxplaybackrate=16000;minptime=20;sprop-maxcapturerate=16000;useinbandfec=1",
    },
    "audio",
    "bridge-audio",
)
```

## Testing Strategy

1. **Webhook Verification:** Test GET endpoint with verification parameters
2. **SDP Processing:** Use `/test-call` endpoint with sample SDP
3. **Inbound Calls:** Make WhatsApp call and verify logs show connection
4. **Outbound Calls:** Use `/initiate-call` and check webhook reception
5. **Audio Quality:** Listen to actual call and check for latency/quality issues
6. **OpenAI Integration:** Verify transcriptions and AI responses in logs
7. **Error Scenarios:** Test reject, timeout, network issues

## Performance Considerations

- ICE gathering timeout: 3 seconds (main.go:833-839)
- HTTP client timeout: 10 seconds for API calls
- RTCP buffer reading: Non-blocking goroutine per sender
- Audio packet logging: Only first 3 packets + every 100th to reduce noise
- OpenAI connection: Pre-connect during outbound call ring to reduce latency
- Webhook processing: Asynchronous to return 200 OK immediately

## Security Notes

- All audio uses DTLS-SRTP encryption
- Webhook signature verification available but not required for Calling API
- Never log full access tokens (log first 20 chars only)
- Clean up peer connections on call termination to prevent resource leaks
