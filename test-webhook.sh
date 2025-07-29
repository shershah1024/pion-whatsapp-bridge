#!/bin/bash

# Test webhook script to simulate WhatsApp call events

WEBHOOK_URL="${1:-http://localhost:3000/whatsapp-call}"

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
              "sdp": "v=0\r\no=- 1234567890 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=ice-lite\r\nm=audio 9999 UDP/TLS/RTP/SAVPF 8\r\nc=IN IP4 192.168.1.100\r\na=rtpmap:8 PCMA/8000\r\na=sendrecv"
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