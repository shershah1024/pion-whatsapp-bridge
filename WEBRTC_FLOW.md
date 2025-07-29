# WhatsApp WebRTC Call Flow

## Key Insights

WhatsApp uses a specific WebRTC configuration that requires careful handling:

### WhatsApp's SDP Offer
- **Uses `a=ice-lite`** - WhatsApp operates in passive ICE mode
- **Uses `a=setup:actpass`** - Can be either active or passive for DTLS
- **Includes candidates** - But in ice-lite mode, it won't initiate connections

### Our SDP Answer Must
- **NOT include `a=ice-lite`** - We must be a full ICE agent
- **Use `a=setup:active`** - We must be the active DTLS side
- **Include our candidates** - We initiate the connection to WhatsApp

## Correct Configuration

```go
// 1. Don't use ice-lite mode
s := webrtc.SettingEngine{}
// s.SetLite(true) // DON'T DO THIS!
s.SetAnsweringDTLSRole(webrtc.DTLSRoleClient) // Be active/client

// 2. Use STUN servers (we need candidates)
config := webrtc.Configuration{
    ICEServers: []webrtc.ICEServer{
        {URLs: []string{"stun:stun.l.google.com:19302"}},
    },
}

// 3. Wait for ICE gathering to complete
gatherComplete := webrtc.GatheringCompletePromise(pc)
select {
case <-gatherComplete:
    // ICE gathering complete
case <-time.After(3 * time.Second):
    // Timeout
}
```

## Call Flow

1. **WhatsApp sends webhook** with SDP offer (ice-lite, setup:actpass)
2. **We create peer connection** as full ICE agent
3. **Add audio track** for bidirectional audio
4. **Set remote description** with WhatsApp's offer
5. **Create answer** (no ice-lite, setup:active)
6. **Wait for ICE gathering** to get our candidates
7. **Send pre_accept** to establish WebRTC connection
8. **Wait 500ms** to ensure connection is ready
9. **Send accept** to start media flow
10. **Audio flows** bidirectionally

## Common Mistakes

❌ **Using ice-lite in answer** - Connection won't establish
❌ **Using setup:passive** - DTLS negotiation fails
❌ **Not waiting for ICE gathering** - No candidates in answer
❌ **No STUN servers** - Can't gather public candidates
❌ **Modifying the SDP** - Breaks compatibility

## Debugging

Enable these logs to debug connection issues:
- ICE connection state changes
- ICE gathering state changes
- ICE candidates
- Full SDP answer

Look for:
- ✅ ICE gathering complete
- ✅ Multiple ICE candidates
- ✅ ICE connection state: connected
- ✅ Audio packets being received