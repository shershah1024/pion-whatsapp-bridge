#!/bin/bash

# Pion WhatsApp Bridge Test Script
echo "🧪 Pion WhatsApp Bridge Test Suite"
echo "=================================="
echo ""

# Check if service is running
if ! curl -s http://localhost:3000/health > /dev/null 2>&1; then
    echo "❌ Bridge service is not running"
    echo "💡 Please start it first with: ./deploy.sh"
    exit 1
fi

echo "✅ Bridge service detected"
echo ""

# Function to pretty print JSON
pretty_json() {
    python3 -m json.tool 2>/dev/null || cat
}

# Test 1: Health Check
echo "📋 Test 1: Health Check"
echo "----------------------"
curl -s http://localhost:3000/health | pretty_json
echo ""

# Test 2: Status Check
echo "📋 Test 2: Status Check"
echo "----------------------"
curl -s http://localhost:3000/status | pretty_json
echo ""

# Test 3: Webhook Verification
echo "📋 Test 3: Webhook Verification"
echo "------------------------------"
VERIFY_RESPONSE=$(curl -s 'http://localhost:3000/whatsapp-call?hub.mode=subscribe&hub.verify_token=whatsapp_bridge_token&hub.challenge=test123')
if [ "$VERIFY_RESPONSE" = "test123" ]; then
    echo "✅ Webhook verification: PASSED"
else
    echo "❌ Webhook verification: FAILED"
    echo "Response: $VERIFY_RESPONSE"
fi
echo ""

# Test 4: Simple Test Call
echo "📋 Test 4: Simple Test Call (no SDP)"
echo "-----------------------------------"
curl -s -X POST http://localhost:3000/test-call \
  -H 'Content-Type: application/json' \
  -d '{"test": "call"}' | pretty_json
echo ""

# Test 5: Test Call with WhatsApp-like SDP
echo "📋 Test 5: Test Call with WhatsApp-like SDP"
echo "------------------------------------------"
cat > /tmp/test-sdp.json << 'EOF'
{
  "sdp": "v=0\no=whatsapp 123456789 123456789 IN IP4 192.168.1.100\ns=WhatsApp Voice Call\nc=IN IP4 192.168.1.100\nt=0 0\nm=audio 12345 RTP/AVP 8 0\na=rtpmap:8 PCMA/8000\na=rtpmap:0 PCMU/8000\na=sendrecv\na=ice-ufrag:4aFv\na=ice-pwd:by4GZGG1lw+040DWA6hXM5Bz\na=ice-lite"
}
EOF

curl -s -X POST http://localhost:3000/test-call \
  -H 'Content-Type: application/json' \
  -d @/tmp/test-sdp.json | pretty_json
echo ""

# Test 6: WhatsApp Webhook Event Simulation
echo "📋 Test 6: WhatsApp Webhook Event Simulation"
echo "-------------------------------------------"
cat > /tmp/webhook-event.json << 'EOF'
{
  "entry": [{
    "id": "ENTRY_ID",
    "changes": [{
      "value": {
        "messaging_product": "whatsapp",
        "metadata": {
          "display_phone_number": "15550551234",
          "phone_number_id": "PHONE_NUMBER_ID"
        },
        "messages": [{
          "from": "15550559999",
          "id": "wamid.ID",
          "timestamp": "1669233778",
          "type": "audio",
          "audio": {
            "id": "AUDIO_ID",
            "mime_type": "audio/opus"
          }
        }]
      },
      "field": "messages"
    }]
  }]
}
EOF

echo "Sending WhatsApp webhook event..."
curl -s -X POST http://localhost:3000/whatsapp-call \
  -H 'Content-Type: application/json' \
  -d @/tmp/webhook-event.json | pretty_json
echo ""

# Cleanup
rm -f /tmp/test-sdp.json /tmp/webhook-event.json

# Summary
echo "📊 Test Summary"
echo "==============="
echo "✅ All tests completed"
echo ""
echo "💡 Next steps:"
echo "  1. Check the logs for audio detection messages"
echo "  2. Verify ice-lite mode is active in the SDP responses"
echo "  3. Test with actual WhatsApp Business API"
echo ""