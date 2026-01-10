# Pion WhatsApp Bridge

Turn a WhatsApp Business number into a real-time voice endpoint that you can fully control from Go – or plug straight into OpenAI Realtime to get an AI agent that answers WhatsApp calls.

Behind the scenes it:

- Receives WhatsApp voice call webhooks
- Negotiates a WebRTC call using Pion
- Bridges the audio to your logic (or OpenAI Realtime)
- Sends audio back to the caller in real time

---

## What you can use it for

- **AI phone agent on WhatsApp**  
  Pipe the call into OpenAI Realtime: transcribe, reason, and respond with natural voice.

- **Programmable call flows**  
  Build IVRs, bots, or custom logic in Go while still using WhatsApp’s calling UI.

- **Audio-first experiments**  
  Capture raw audio from real users on WhatsApp without touching Twilio/SIP stacks.

---

## What makes it different

- **Pure Go, single binary**  
  No external media server, no plugin system – just `go build` and run.

- **Designed specifically for WhatsApp calling**  
  Uses Pion with native `ice-lite` and SDP control to match WhatsApp’s non-standard WebRTC behavior.

- **First-class AI bridge**  
  Built to maintain *two* live WebRTC legs:  
  WhatsApp ↔ Pion ↔ OpenAI Realtime, with data channels and audio forwarding already wired in.

- **Deployment friendly**  
  Works as a normal HTTP service; scripts and docs included for tunneling (ngrok) and container deployment (Azure, etc.).

---

## How it works (high level)

1. A user calls your WhatsApp Business number.
2. WhatsApp sends a webhook with the call event + SDP offer.
3. The bridge uses Pion to create a WebRTC peer connection and SDP answer.
4. It calls the WhatsApp API to accept the call with that answer.
5. Media flows over WebRTC:
   - Inbound: caller → Pion (→ optional AI / processing)
   - Outbound: your audio / AI audio → caller
6. If `OPENAI_API_KEY` is set, audio is streamed to OpenAI Realtime and responses are spoken back automatically.

---

## Quick start

1. Clone the repo and set environment variables:

   - `WHATSAPP_TOKEN` – WhatsApp access token  
   - `PHONE_NUMBER_ID` – WhatsApp phone number ID  
   - `VERIFY_TOKEN` – webhook verification token  
   - `OPENAI_API_KEY` – (optional) enables AI assistant  
   - `PORT` – HTTP port (default `3000`)

2. Run the deployment script:

   ```bash
   ./deploy.sh