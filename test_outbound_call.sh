#!/bin/bash

# Test script to initiate an outbound WhatsApp call

# Phone number to call (without +)
PHONE_NUMBER="919885842349"

# Server endpoint
SERVER_URL="http://localhost:3011/initiate-call"

echo "📞 Initiating outbound call to $PHONE_NUMBER..."
echo ""

curl -X POST "$SERVER_URL" \
  -H "Content-Type: application/json" \
  -d "{\"to\": \"$PHONE_NUMBER\"}" \
  | jq '.'

echo ""
echo "✅ Call initiated! Check server logs for status."
echo "📱 The user should receive a call on WhatsApp..."
