# WhatsApp Cloud API - Business-Initiated Calls Guide

## Table of Contents
- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Call Flow](#call-flow)
- [API Endpoints](#api-endpoints)
- [SDP Protocol](#sdp-protocol)
- [Implementation Steps](#implementation-steps)
- [Webhooks](#webhooks)
- [Code Examples](#code-examples)
- [Best Practices](#best-practices)
- [Limitations](#limitations)

---

## Overview

Business-initiated calls (outbound calls) allow your WhatsApp Business Account to initiate voice calls to WhatsApp users. This is the reverse of the current implementation where users call your business.

### Key Difference
- **Inbound Calls** (Current): User initiates → Your system receives webhook → Accept call
- **Outbound Calls** (New): Your system initiates → Send API request → User receives call

---

## Prerequisites

### 1. Enable Calling APIs
Ensure calling APIs are enabled on your WhatsApp Business phone number.

### 2. Subscribe to Webhooks
You must subscribe to the `calls` webhook field to receive call events.

```bash
# Check current webhook subscriptions
curl -X GET "https://graph.facebook.com/v21.0/<APP_ID>/subscriptions" \
  -H "Authorization: Bearer <APP_ACCESS_TOKEN>"

# Subscribe to calls webhook
curl -X POST "https://graph.facebook.com/v21.0/<APP_ID>/subscriptions" \
  -H "Authorization: Bearer <APP_ACCESS_TOKEN>" \
  -d "object=whatsapp_business_account" \
  -d "callback_url=https://your-webhook-url.com/whatsapp-call" \
  -d "fields=calls" \
  -d "verify_token=your_verify_token"
```

### 3. Obtain User Permission
You **must** obtain the WhatsApp user's permission before calling them. Two methods:

#### Method 1: Call Permission Request Message
Send a message template requesting permission to call.

#### Method 2: Enable Callback Permission Status
Enable `callback_permission_status` in your call settings to automatically track when users grant permission.

---

## Call Flow

### Business-Initiated Call Flow

```
┌─────────────┐                ┌──────────────┐                ┌─────────────┐
│ Your System │                │   WhatsApp   │                │  WhatsApp   │
│   (Pion)    │                │  Cloud API   │                │    User     │
└──────┬──────┘                └──────┬───────┘                └──────┬──────┘
       │                              │                               │
       │ 1. POST /calls (connect)     │                               │
       │ with SDP offer               │                               │
       ├─────────────────────────────>│                               │
       │                              │                               │
       │ 2. 200 OK                    │                               │
       │ with call_id                 │                               │
       │<─────────────────────────────┤                               │
       │                              │                               │
       │                              │ 3. Ring user's phone          │
       │                              ├──────────────────────────────>│
       │                              │                               │
       │                              │ 4. User accepts/rejects       │
       │                              │<──────────────────────────────┤
       │                              │                               │
       │ 5. Webhook: call.connect     │                               │
       │ with SDP answer              │                               │
       │<─────────────────────────────┤                               │
       │                              │                               │
       │ 6. POST /calls (accept)      │                               │
       │ with SDP answer              │                               │
       ├─────────────────────────────>│                               │
       │                              │                               │
       │                              │ 7. Establish RTP connection   │
       │                              │<─────────────────────────────>│
       │                              │                               │
       │ 8. Audio flows (RTP)         │                               │
       │<════════════════════════════════════════════════════════════>│
       │                              │                               │
```

---

## API Endpoints

### Base URL
```
https://graph.facebook.com/v21.0/<PHONE_NUMBER_ID>/calls
```

### 1. Initiate Call (Connect)

**Endpoint:** `POST /<PHONE_NUMBER_ID>/calls`

**Headers:**
```
Authorization: Bearer <ACCESS_TOKEN>
Content-Type: application/json
```

**Request Body:**
```json
{
  "messaging_product": "whatsapp",
  "to": "14085551234",
  "action": "connect",
  "session": {
    "sdp_type": "offer",
    "sdp": "v=0\r\no=- 123456 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\n..."
  }
}
```

**Parameters:**
- `messaging_product`: Always "whatsapp"
- `to`: Phone number to call (E.164 format without +)
- `action`: "connect" to initiate call
- `session.sdp_type`: "offer" (you're offering to connect)
- `session.sdp`: RFC 8866 compliant SDP offer

**Response:**
```json
{
  "call_id": "wacid.HBgYMTQwODU1NTEyMzQVAgASGCA4RTNERTg5QjYxNDkxNjJFQ0U3NkNBQTZFNjU5REQcGAw5MTczMDYzNTY1MTQVAgAVFAA",
  "messaging_product": "whatsapp"
}
```

### 2. Accept Call (After User Accepts)

**Endpoint:** `POST /<PHONE_NUMBER_ID>/calls`

**Request Body:**
```json
{
  "messaging_product": "whatsapp",
  "call_id": "wacid.HBgYMTQwODU1NTEyMzQVAgASGCA4RTNERTg5QjYxNDkxNjJFQ0U3NkNBQTZFNjU5REQcGAw5MTczMDYzNTY1MTQVAgAVFAA",
  "action": "accept",
  "session": {
    "sdp_type": "answer",
    "sdp": "v=0\r\no=- 123456 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\n..."
  }
}
```

**Parameters:**
- `call_id`: The call_id returned from connect request
- `action`: "accept" to finalize connection
- `session.sdp_type`: "answer" (responding to user's answer)
- `session.sdp`: Your final SDP answer

### 3. Terminate Call

**Endpoint:** `POST /<PHONE_NUMBER_ID>/calls`

**Request Body:**
```json
{
  "messaging_product": "whatsapp",
  "call_id": "wacid.HBgYMTQwODU1NTEyMzQVAgASGCA4RTNERTg5QjYxNDkxNjJFQ0U3NkNBQTZFNjU5REQcGAw5MTczMDYzNTY1MTQVAgAVFAA",
  "action": "terminate"
}
```

---

## SDP Protocol

### What is SDP?

Session Description Protocol (SDP) is a text-based format (RFC 8866) used to describe multimedia communication sessions. It negotiates:
- Media types (audio/video)
- Codecs (Opus for audio)
- Network addresses and ports
- Encryption keys
- ICE candidates for NAT traversal

### SDP Structure

```
v=0                                          # Protocol version
o=- 8777056337507125844 1759734982 IN IP4 0.0.0.0  # Origin
s=-                                          # Session name
t=0 0                                        # Timing
a=group:BUNDLE 0                             # Bundle audio
a=extmap-allow-mixed                         # Extensions
a=msid-semantic: WMS pion-stream             # Media stream ID
m=audio 9 UDP/TLS/RTP/SAVPF 111              # Media description
c=IN IP4 0.0.0.0                             # Connection info
a=rtcp:9 IN IP4 0.0.0.0                      # RTCP port
a=ice-ufrag:cloRuCCQaPtDYlxC                 # ICE username fragment
a=ice-pwd:AtKMBUKyeXipfbJjySeKakHWUcOxysKA  # ICE password
a=ice-options:trickle                        # ICE trickling
a=fingerprint:sha-256 XX:XX:XX...            # DTLS fingerprint
a=setup:actpass                              # DTLS setup role
a=mid:0                                      # Media ID
a=sendrecv                                   # Media direction
a=rtcp-mux                                   # Multiplex RTP/RTCP
a=rtpmap:111 opus/48000/2                    # Opus codec, 48kHz, stereo
a=fmtp:111 minptime=10;useinbandfec=1        # Opus parameters
a=ssrc:44200254 cname:pion-track             # Synchronization source
a=candidate:...                              # ICE candidates
```

### Key SDP Components

#### 1. Session Level
```
v=0                     # Version (always 0)
o=- <id> <version> IN IP4 <address>  # Origin
s=-                     # Session name
t=0 0                   # Time (0 0 = permanent session)
```

#### 2. Media Level (Audio)
```
m=audio <port> UDP/TLS/RTP/SAVPF <codec-id>
c=IN IP4 <ip-address>
a=rtpmap:<codec-id> opus/48000/2
```

#### 3. ICE Candidates
```
a=ice-ufrag:<username-fragment>
a=ice-pwd:<password>
a=candidate:<foundation> <component> <protocol> <priority> <ip> <port> typ <type>
```

Example:
```
a=candidate:1 1 UDP 2130706431 192.168.1.100 54321 typ host
```

#### 4. DTLS/SRTP Security
```
a=fingerprint:sha-256 <hash>
a=setup:actpass
```

### WhatsApp SDP Requirements

1. **Codec:** Opus (payload type 111)
   ```
   a=rtpmap:111 opus/48000/2
   a=fmtp:111 minptime=10;useinbandfec=1
   ```

2. **ICE:** Must include ICE candidates
   ```
   a=ice-ufrag:...
   a=ice-pwd:...
   a=candidate:...
   ```

3. **DTLS:** Required for secure RTP
   ```
   a=fingerprint:sha-256 ...
   a=setup:actpass
   ```

4. **Direction:** Typically `sendrecv` for two-way audio
   ```
   a=sendrecv
   ```

---

## Implementation Steps

### Step 1: Create WebRTC Peer Connection

```go
// Create peer connection configuration
config := webrtc.Configuration{
    ICEServers: []webrtc.ICEServer{
        {
            URLs: []string{"stun:stun.l.google.com:19302"},
        },
    },
}

// Create peer connection
pc, err := webrtc.NewPeerConnection(config)
if err != nil {
    log.Fatal(err)
}
```

### Step 2: Add Audio Track

```go
// Create audio track
audioTrack, err := webrtc.NewTrackLocalStaticRTP(
    webrtc.RTPCodecCapability{
        MimeType:  "audio/opus",
        ClockRate: 48000,
        Channels:  2,
    },
    "audio",
    "pion-stream",
)

// Add track to peer connection
_, err = pc.AddTrack(audioTrack)
```

### Step 3: Create SDP Offer

```go
// Create offer
offer, err := pc.CreateOffer(nil)
if err != nil {
    log.Fatal(err)
}

// Set local description
err = pc.SetLocalDescription(offer)
if err != nil {
    log.Fatal(err)
}

// Wait for ICE gathering to complete
<-webrtc.GatheringCompletePromise(pc)

// Get complete SDP with ICE candidates
sdpOffer := pc.LocalDescription().SDP
```

### Step 4: Initiate Call via WhatsApp API

```go
type ConnectCallRequest struct {
    MessagingProduct string `json:"messaging_product"`
    To               string `json:"to"`
    Action           string `json:"action"`
    Session          struct {
        SDPType string `json:"sdp_type"`
        SDP     string `json:"sdp"`
    } `json:"session"`
}

func initiateCall(phoneNumber, sdpOffer string) (string, error) {
    req := ConnectCallRequest{
        MessagingProduct: "whatsapp",
        To:               phoneNumber,
        Action:           "connect",
    }
    req.Session.SDPType = "offer"
    req.Session.SDP = sdpOffer

    jsonData, _ := json.Marshal(req)

    url := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/calls",
        os.Getenv("PHONE_NUMBER_ID"))

    httpReq, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
    httpReq.Header.Set("Authorization", "Bearer "+os.Getenv("WHATSAPP_TOKEN"))
    httpReq.Header.Set("Content-Type", "application/json")

    client := &http.Client{}
    resp, err := client.Do(httpReq)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        CallID string `json:"call_id"`
    }
    json.NewDecoder(resp.Body).Decode(&result)

    return result.CallID, nil
}
```

### Step 5: Handle Call Connect Webhook

```go
func handleCallWebhook(w http.ResponseWriter, r *http.Request) {
    var webhook struct {
        Entry []struct {
            Changes []struct {
                Value struct {
                    CallID  string `json:"call_id"`
                    From    string `json:"from"`
                    Session struct {
                        SDPType string `json:"sdp_type"`
                        SDP     string `json:"sdp"`
                    } `json:"session"`
                } `json:"value"`
            } `json:"changes"`
        } `json:"entry"`
    }

    json.NewDecoder(r.Body).Decode(&webhook)

    // Extract SDP answer from user
    userSDP := webhook.Entry[0].Changes[0].Value.Session.SDP
    callID := webhook.Entry[0].Changes[0].Value.CallID

    // Set remote description
    answer := webrtc.SessionDescription{
        Type: webrtc.SDPTypeAnswer,
        SDP:  userSDP,
    }
    err := pc.SetRemoteDescription(answer)

    // Accept the call with your final SDP
    acceptCall(callID, pc.LocalDescription().SDP)
}
```

### Step 6: Accept Call

```go
func acceptCall(callID, sdpAnswer string) error {
    req := map[string]interface{}{
        "messaging_product": "whatsapp",
        "call_id":           callID,
        "action":            "accept",
        "session": map[string]string{
            "sdp_type": "answer",
            "sdp":      sdpAnswer,
        },
    }

    jsonData, _ := json.Marshal(req)

    url := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/calls",
        os.Getenv("PHONE_NUMBER_ID"))

    httpReq, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
    httpReq.Header.Set("Authorization", "Bearer "+os.Getenv("WHATSAPP_TOKEN"))
    httpReq.Header.Set("Content-Type", "application/json")

    client := &http.Client{}
    resp, err := client.Do(httpReq)

    return err
}
```

### Step 7: Handle Audio (RTP)

Once the call is accepted, audio flows via RTP just like inbound calls:

```go
// Send audio to user
pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
    // Receive audio from WhatsApp user
})

// Write audio to track
audioTrack.Write(rtpPacket)
```

---

## Webhooks

### 1. Call Connect Webhook

Sent when user accepts the call.

```json
{
  "object": "whatsapp_business_account",
  "entry": [
    {
      "id": "WHATSAPP_BUSINESS_ACCOUNT_ID",
      "changes": [
        {
          "value": {
            "messaging_product": "whatsapp",
            "call_id": "wacid.HBgYMTQwODU1NTEyMzQVAgASGCA4RTNERTg5QjYxNDkxNjJFQ0U3NkNBQTZFNjU5REQcGAw5MTczMDYzNTY1MTQVAgAVFAA",
            "from": "14085551234",
            "session": {
              "sdp_type": "answer",
              "sdp": "v=0\r\no=- 123456 2 IN IP4 ..."
            },
            "event_type": "call.connect"
          },
          "field": "calls"
        }
      ]
    }
  ]
}
```

### 2. Call Status Webhook

Tracks call status changes (ringing, accepted, rejected).

```json
{
  "value": {
    "call_id": "wacid.XXX",
    "from": "14085551234",
    "status": "accepted",
    "event_type": "call.status"
  }
}
```

Possible statuses:
- `ringing`: Call is ringing
- `accepted`: User accepted call
- `rejected`: User rejected call
- `busy`: User is busy
- `no_answer`: User didn't answer

### 3. Call Terminate Webhook

Sent when call ends.

```json
{
  "value": {
    "call_id": "wacid.XXX",
    "from": "14085551234",
    "duration": 120,
    "reason": "user_hangup",
    "event_type": "call.terminate"
  }
}
```

---

## Code Examples

### Complete Outbound Call Implementation

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "time"

    "github.com/pion/webrtc/v3"
)

type OutboundCall struct {
    pc         *webrtc.PeerConnection
    audioTrack *webrtc.TrackLocalStaticRTP
    callID     string
}

func NewOutboundCall() (*OutboundCall, error) {
    // Create peer connection
    config := webrtc.Configuration{
        ICEServers: []webrtc.ICEServer{
            {URLs: []string{"stun:stun.l.google.com:19302"}},
        },
    }

    pc, err := webrtc.NewPeerConnection(config)
    if err != nil {
        return nil, err
    }

    // Create audio track
    audioTrack, err := webrtc.NewTrackLocalStaticRTP(
        webrtc.RTPCodecCapability{
            MimeType:  "audio/opus",
            ClockRate: 48000,
            Channels:  2,
        },
        "audio",
        "pion-stream",
    )
    if err != nil {
        return nil, err
    }

    _, err = pc.AddTrack(audioTrack)
    if err != nil {
        return nil, err
    }

    return &OutboundCall{
        pc:         pc,
        audioTrack: audioTrack,
    }, nil
}

func (c *OutboundCall) Dial(phoneNumber string) error {
    // Create offer
    offer, err := c.pc.CreateOffer(nil)
    if err != nil {
        return err
    }

    // Set local description
    if err := c.pc.SetLocalDescription(offer); err != nil {
        return err
    }

    // Wait for ICE gathering
    gatherComplete := webrtc.GatheringCompletePromise(c.pc)
    <-gatherComplete

    // Get SDP offer
    sdpOffer := c.pc.LocalDescription().SDP

    // Initiate call via WhatsApp API
    callID, err := c.initiateCallAPI(phoneNumber, sdpOffer)
    if err != nil {
        return err
    }

    c.callID = callID
    log.Printf("✅ Call initiated to %s, call_id: %s", phoneNumber, callID)

    return nil
}

func (c *OutboundCall) initiateCallAPI(phoneNumber, sdpOffer string) (string, error) {
    reqBody := map[string]interface{}{
        "messaging_product": "whatsapp",
        "to":                phoneNumber,
        "action":            "connect",
        "session": map[string]string{
            "sdp_type": "offer",
            "sdp":      sdpOffer,
        },
    }

    jsonData, _ := json.Marshal(reqBody)

    url := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/calls",
        os.Getenv("WHATSAPP_PHONE_NUMBER_ID"))

    req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
    req.Header.Set("Authorization", "Bearer "+os.Getenv("WHATSAPP_TOKEN"))
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("API error: %s - %s", resp.Status, string(body))
    }

    var result struct {
        CallID string `json:"call_id"`
    }
    json.NewDecoder(resp.Body).Decode(&result)

    return result.CallID, nil
}

func (c *OutboundCall) HandleUserAnswer(userSDP string) error {
    answer := webrtc.SessionDescription{
        Type: webrtc.SDPTypeAnswer,
        SDP:  userSDP,
    }

    if err := c.pc.SetRemoteDescription(answer); err != nil {
        return err
    }

    // Send accept to WhatsApp API
    return c.acceptCallAPI()
}

func (c *OutboundCall) acceptCallAPI() error {
    reqBody := map[string]interface{}{
        "messaging_product": "whatsapp",
        "call_id":           c.callID,
        "action":            "accept",
        "session": map[string]string{
            "sdp_type": "answer",
            "sdp":      c.pc.LocalDescription().SDP,
        },
    }

    jsonData, _ := json.Marshal(reqBody)

    url := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/calls",
        os.Getenv("WHATSAPP_PHONE_NUMBER_ID"))

    req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
    req.Header.Set("Authorization", "Bearer "+os.Getenv("WHATSAPP_TOKEN"))
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("accept error: %s - %s", resp.Status, string(body))
    }

    log.Printf("✅ Call accepted: %s", c.callID)
    return nil
}

func (c *OutboundCall) Terminate() error {
    reqBody := map[string]interface{}{
        "messaging_product": "whatsapp",
        "call_id":           c.callID,
        "action":            "terminate",
    }

    jsonData, _ := json.Marshal(reqBody)

    url := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/calls",
        os.Getenv("WHATSAPP_PHONE_NUMBER_ID"))

    req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
    req.Header.Set("Authorization", "Bearer "+os.Getenv("WHATSAPP_TOKEN"))
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    c.pc.Close()
    return nil
}

// Usage example
func main() {
    // Create outbound call
    call, err := NewOutboundCall()
    if err != nil {
        log.Fatal(err)
    }

    // Dial user
    if err := call.Dial("14085551234"); err != nil {
        log.Fatal(err)
    }

    // Wait for webhook with user's SDP answer
    // (In real implementation, this comes from webhook handler)
    userSDP := "..." // From webhook

    if err := call.HandleUserAnswer(userSDP); err != nil {
        log.Fatal(err)
    }

    // Call is now connected, audio flows via RTP

    // Later, terminate call
    time.Sleep(30 * time.Second)
    call.Terminate()
}
```

---

## Best Practices

### 1. Permission Management
- Always obtain explicit permission before calling users
- Track permission status per user
- Respect opt-outs immediately

### 2. SDP Generation
- Use a reliable WebRTC library (like Pion)
- Always include ICE candidates
- Wait for ICE gathering to complete before sending offer
- Use STUN/TURN servers for NAT traversal

### 3. Error Handling
```go
// Handle all possible call states
switch status {
case "rejected":
    log.Printf("User rejected call")
    // Clean up resources
case "busy":
    log.Printf("User is busy")
    // Maybe retry later
case "no_answer":
    log.Printf("User didn't answer")
    // Track metrics
}
```

### 4. Resource Cleanup
```go
defer func() {
    if pc != nil {
        pc.Close()
    }
}()
```

### 5. Webhook Verification
```go
// Verify webhook authenticity
func verifyWebhook(signature, body string) bool {
    mac := hmac.New(sha256.New, []byte(APP_SECRET))
    mac.Write([]byte(body))
    expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(signature), []byte(expected))
}
```

### 6. Rate Limiting
- Respect WhatsApp's rate limits
- Implement exponential backoff for retries
- Monitor webhook delivery success

### 7. Logging
```go
log.Printf("📞 Outbound call initiated: to=%s, call_id=%s", phoneNumber, callID)
log.Printf("✅ User answered: call_id=%s, duration=%d", callID, duration)
log.Printf("❌ Call failed: call_id=%s, reason=%s", callID, reason)
```

---

## Limitations

### Technical Limitations
1. **Codec:** Only Opus codec supported
2. **Media:** Audio only (no video)
3. **NAT:** Requires ICE/STUN/TURN for NAT traversal
4. **Encryption:** DTLS-SRTP mandatory

### Business Limitations
1. **Permission Required:** Must have user's permission
2. **Rate Limits:** WhatsApp enforces rate limits
3. **Quality of Service:** Network dependent
4. **Availability:** User must be online on WhatsApp

### API Limitations
1. **No Call Waiting:** Can't place user on hold
2. **No Conference:** No multi-party calls
3. **No Recording:** No built-in recording API
4. **No DTMF:** No dual-tone multi-frequency support

---

## Comparison: Inbound vs Outbound

| Feature | Inbound (Current) | Outbound (New) |
|---------|-------------------|----------------|
| **Initiator** | WhatsApp User | Your Business |
| **First SDP** | Receive offer from user | Send offer to user |
| **API Flow** | pre_accept → accept | connect → accept |
| **Permission** | Implicit (user calls) | Explicit required |
| **Webhooks** | Single webhook | Multiple (status, connect, terminate) |
| **Use Case** | Customer support | Proactive outreach, reminders |

---

## Next Steps

1. **Enable Outbound Calling**
   - Contact WhatsApp support to enable business-initiated calls
   - Update webhook subscriptions

2. **Implement Permission System**
   - Create message templates for permission requests
   - Track user permissions in database

3. **Extend Current Code**
   - Add outbound call functions to `main.go`
   - Create webhook handlers for new events
   - Implement call queueing/scheduling

4. **Test Thoroughly**
   - Test with different phone numbers
   - Test call rejection scenarios
   - Test network failure scenarios
   - Monitor audio quality

5. **Monitor & Optimize**
   - Track call success rates
   - Monitor audio quality metrics
   - Optimize ICE candidate selection
   - Implement analytics

---

## References

- [WhatsApp Cloud API - Business Initiated Calls](https://developers.facebook.com/docs/whatsapp/cloud-api/calling/business-initiated-calls)
- [WhatsApp Cloud API - SDP Reference](https://developers.facebook.com/docs/whatsapp/cloud-api/calling/reference#sdp-overview-and-sample-sdp-structures)
- [RFC 8866 - SDP](https://datatracker.ietf.org/doc/html/rfc8866)
- [WebRTC Specification](https://www.w3.org/TR/webrtc/)
- [Pion WebRTC Documentation](https://github.com/pion/webrtc)
