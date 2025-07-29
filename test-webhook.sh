#!/bin/bash

# Test webhook script to simulate WhatsApp call events

WEBHOOK_URL="${1:-http://localhost:3000}/whatsapp-call"

echo "🧪 Testing WhatsApp webhook at: $WEBHOOK_URL"
echo ""

# Test 1: Simple message webhook
echo "Test 1: Simple message webhook"
curl -X POST "$WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  -d '{
    "object": "whatsapp_business_account",
    "entry": [{
      "id": "123456789",
      "changes": [{
        "value": {
          "messaging_product": "whatsapp",
          "metadata": {
            "display_phone_number": "15551234567",
            "phone_number_id": "106382542433515"
          },
          "messages": [{
            "from": "15559876543",
            "id": "wamid.test123",
            "timestamp": "1234567890",
            "type": "text",
            "text": {
              "body": "Test message"
            }
          }]
        },
        "field": "messages"
      }]
    }]
  }'

echo -e "\n\n"

# Test 2: Call webhook (USER_INITIATED)
echo "Test 2: Incoming call webhook"
curl -X POST "$WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  -d '{
    "object": "whatsapp_business_account",
    "entry": [{
      "id": "123456789",
      "changes": [{
        "value": {
          "messaging_product": "whatsapp",
          "metadata": {
            "display_phone_number": "15551234567",
            "phone_number_id": "106382542433515"
          },
          "calls": [{
            "id": "wacid.TEST_CALL_ID_123",
            "to": "15551234567",
            "from": "15559876543",
            "event": "connect",
            "timestamp": "1234567890",
            "direction": "USER_INITIATED",
            "session": {
              "sdp_type": "offer",
              "sdp": "v=0\r\no=- 7602563789789945080 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE audio\r\na=msid-semantic: WMS 6932bc1c-db1a-4abe-b437-0c4168be8a13\r\na=ice-lite\r\nm=audio 40012 UDP/TLS/RTP/SAVPF 111 126\r\nc=IN IP4 31.13.65.60\r\na=rtcp:9 IN IP4 0.0.0.0\r\na=candidate:1972637320 1 udp 2113937151 31.13.65.60 40012 typ host generation 0 network-cost 50 ufrag 6k2qP1R6kBfI/2\r\na=ice-ufrag:6k2qP1R6kBfI/2\r\na=ice-pwd:UApvJw3NcwFRDvIMKdM0vWCdlXah25E9\r\na=fingerprint:sha-256 1B:B6:6B:40:A5:0B:8C:75:0D:8C:CB:90:2F:99:74:1E:26:45:AE:AF:45:C1:51:60:8F:73:C9:2D:10:6D:8A:88\r\na=setup:actpass\r\na=mid:audio\r\na=extmap:1 urn:ietf:params:rtp-hdrext:ssrc-audio-level\r\na=sendrecv\r\na=rtcp-mux\r\na=rtpmap:111 opus/48000/2\r\na=rtcp-fb:111 transport-cc\r\na=fmtp:111 minptime=10;useinbandfec=1\r\na=rtpmap:126 telephone-event/8000\r\na=ssrc:4208138518 cname:gAXq2V9TKltrnapv"
            }
          }]
        },
        "field": "calls"
      }]
    }]
  }'

echo -e "\n\n"

# Test 3: Call status webhook
echo "Test 3: Call status webhook (ringing)"
curl -X POST "$WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  -d '{
    "object": "whatsapp_business_account",
    "entry": [{
      "id": "123456789",
      "changes": [{
        "value": {
          "messaging_product": "whatsapp",
          "metadata": {
            "display_phone_number": "15551234567",
            "phone_number_id": "106382542433515"
          },
          "statuses": [{
            "id": "wacid.TEST_CALL_ID_123",
            "timestamp": "1234567890",
            "type": "call",
            "status": "RINGING",
            "recipient_id": "15559876543"
          }]
        },
        "field": "calls"
      }]
    }]
  }'

echo -e "\n\n"
echo "✅ Tests complete. Check your logs!"