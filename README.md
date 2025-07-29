# Pion WhatsApp Bridge

A pure Go implementation for bridging WhatsApp calls using Pion WebRTC with native ice-lite support.

## Why Pion?

After exploring Janus Gateway, we switched to Pion for several compelling reasons:

1. **Native ice-lite Support** - Built-in support for WhatsApp's passive ICE mode
2. **Pure Go** - Single binary deployment, no C dependencies
3. **Direct Control** - Full programmatic control over SDP and RTP
4. **Simpler Architecture** - No plugin system needed
5. **Better for Custom Protocols** - Perfect for non-standard WebRTC implementations

## Features

- ✅ WhatsApp Business API webhook integration
- ✅ ice-lite mode for WhatsApp compatibility
- ✅ Audio detection and OK response system
- ✅ SDP negotiation for WhatsApp's format
- ✅ ngrok integration for public access
- ✅ Simple deployment with single binary

## Prerequisites

1. **Go** (1.21 or later)
   ```bash
   # macOS
   brew install go
   
   # Or download from https://golang.org/dl/
   ```

2. **ngrok** (for public tunnel)
   ```bash
   # macOS
   brew install ngrok
   
   # Or download from https://ngrok.com/
   ```

## Quick Start

```bash
# Deploy the complete system
./deploy.sh
```

This will:
1. Build the Go application
2. Start the Pion WebRTC bridge
3. Create a public ngrok tunnel
4. Provide WhatsApp configuration details

## Architecture

```
WhatsApp Call → Internet → ngrok → Pion Bridge → WebRTC Processing
                                        ↓
                                 Audio Detection
                                        ↓
                                  OK Response
```

## How It Works

1. **WhatsApp Webhook** - Receives call initiation from WhatsApp Business API
2. **SDP Processing** - Parses WhatsApp's SDP and creates compatible response
3. **ice-lite Mode** - Acts as passive ICE endpoint (required by WhatsApp)
4. **Audio Detection** - Monitors incoming RTP packets
5. **OK Response** - Sends acknowledgment when audio is detected

## API Endpoints

- `GET /whatsapp-call` - Webhook verification
- `POST /whatsapp-call` - Webhook events
- `POST /test-call` - Test endpoint with SDP support
- `GET /status` - Bridge status
- `GET /health` - Health check

## Configuration

Environment variables:
- `WHATSAPP_WEBHOOK_SECRET` - Webhook signature secret
- `PORT` - Server port (default: 3000)

## Testing

```bash
# Test webhook verification
curl 'https://your-url.ngrok.io/whatsapp-call?hub.mode=subscribe&hub.verify_token=whatsapp_bridge_token&hub.challenge=test123'

# Test with SDP
curl -X POST 'https://your-url.ngrok.io/test-call' \
  -H 'Content-Type: application/json' \
  -d '{"sdp":"v=0\no=whatsapp 0 0 IN IP4 192.168.1.100\ns=Test\nc=IN IP4 192.168.1.100\nt=0 0\nm=audio 12345 RTP/AVP 8\na=rtpmap:8 PCMA/8000\na=sendrecv"}'
```

## WhatsApp Integration

Configure in WhatsApp Business API:
- **Webhook URL**: `https://your-url.ngrok.io/whatsapp-call`
- **Verify Token**: `whatsapp_bridge_token`
- **Webhook Fields**: Select voice/call events

## Code Structure

- `main.go` - Complete implementation including:
  - WebRTC setup with ice-lite
  - WhatsApp webhook handlers
  - SDP negotiation
  - Audio track handling
  - HTTP server

## Next Steps

1. Configure webhook in WhatsApp Business API
2. Test with actual WhatsApp calls
3. Add OpenAI Realtime API for AI responses
4. Deploy with proper domain and SSL

## Advantages Over Janus

- **No Build Complexity** - Just `go build`, no C dependencies
- **Native ice-lite** - Built into Pion, not a configuration option
- **Direct Go Code** - No plugin architecture to navigate
- **Better Error Handling** - Go's error handling vs C callbacks
- **Easier Debugging** - Single process, clear stack traces
- **Modern Codebase** - Designed for programmatic use

## Production Deployment - Railway

[![Deploy on Railway](https://railway.app/button.svg)](https://railway.app/new/template?template=https://github.com/YOUR_USERNAME/pion-whatsapp-bridge)

### Quick Deploy Steps:

1. **Push to GitHub**:
   ```bash
   git init
   git add .
   git commit -m "Initial commit: Pion WhatsApp Bridge"
   git remote add origin https://github.com/YOUR_USERNAME/pion-whatsapp-bridge.git
   git push -u origin main
   ```

2. **Deploy on Railway**:
   - Go to [railway.app](https://railway.app)
   - Click "New Project" → "Deploy from GitHub repo"
   - Select your repository
   - Railway auto-detects Go and deploys

3. **Get Your URL**:
   - In Railway dashboard → Settings → Domains
   - Click "Generate Domain"
   - You'll get: `https://your-app.up.railway.app`

4. **Configure WhatsApp**:
   - Webhook URL: `https://your-app.up.railway.app/whatsapp-call`
   - Verify Token: `whatsapp_bridge_token`

### Why Railway?
- ✅ **Permanent HTTPS URL** - No more changing ngrok URLs
- ✅ **Zero Config** - Automatic Go detection and build
- ✅ **Free Tier** - $5/month credit, perfect for testing
- ✅ **Auto Deploy** - Push to git, auto deploys
- ✅ **Built-in Monitoring** - Logs and metrics included

See [DEPLOY.md](DEPLOY.md) for detailed instructions.