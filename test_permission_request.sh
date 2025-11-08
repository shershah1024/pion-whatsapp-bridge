#!/bin/bash

# Test script for WhatsApp Call Permission Request system
# Usage: ./test_permission_request.sh <phone_number>

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default to localhost
BASE_URL="${BASE_URL:-http://localhost:3011}"

if [ -z "$1" ]; then
    echo -e "${RED}❌ Error: Phone number required${NC}"
    echo "Usage: $0 <phone_number>"
    echo "Example: $0 14085551234"
    exit 1
fi

PHONE_NUMBER=$1

echo -e "${YELLOW}📞 WhatsApp Call Permission Request Test${NC}"
echo "========================================"
echo "Phone Number: $PHONE_NUMBER"
echo "Server: $BASE_URL"
echo ""

# Test 1: Request permission
echo -e "${YELLOW}Test 1: Requesting call permission...${NC}"
RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST "$BASE_URL/request-call-permission" \
  -H "Content-Type: application/json" \
  -d "{\"to\": \"$PHONE_NUMBER\"}")

HTTP_CODE=$(echo "$RESPONSE" | grep "HTTP_CODE:" | cut -d':' -f2)
BODY=$(echo "$RESPONSE" | sed '/HTTP_CODE:/d')

if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✅ Success! Permission request sent${NC}"
    echo "$BODY" | jq '.' 2>/dev/null || echo "$BODY"
    echo ""
    echo -e "${YELLOW}📱 Check the user's WhatsApp for the permission request message${NC}"
    echo "   They should see:"
    echo "   📞 Would you like to receive voice calls from us?"
    echo "   [✅ Yes, you can call me] [❌ No, thanks]"
    echo ""
elif [ "$HTTP_CODE" = "429" ]; then
    echo -e "${RED}🚫 Rate Limited${NC}"
    echo "$BODY"
    echo ""
    echo -e "${YELLOW}Rate limits: 1 request per 24 hours, 2 requests per 7 days${NC}"
    exit 1
else
    echo -e "${RED}❌ Failed (HTTP $HTTP_CODE)${NC}"
    echo "$BODY"
    exit 1
fi

# Wait for user to approve
echo -e "${YELLOW}Waiting for user approval...${NC}"
echo "Press ENTER after the user clicks '✅ Yes, you can call me'"
read

# Test 2: Try to initiate call
echo ""
echo -e "${YELLOW}Test 2: Attempting to initiate call...${NC}"
RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST "$BASE_URL/initiate-call" \
  -H "Content-Type: application/json" \
  -d "{\"to\": \"$PHONE_NUMBER\"}")

HTTP_CODE=$(echo "$RESPONSE" | grep "HTTP_CODE:" | cut -d':' -f2)
BODY=$(echo "$RESPONSE" | sed '/HTTP_CODE:/d')

if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✅ Success! Call initiated${NC}"
    echo "$BODY" | jq '.' 2>/dev/null || echo "$BODY"
    echo ""
    echo -e "${GREEN}🎉 Permission system is working correctly!${NC}"
elif [ "$HTTP_CODE" = "403" ]; then
    echo -e "${RED}🚫 Forbidden - No permission${NC}"
    echo "$BODY"
    echo ""
    echo -e "${YELLOW}Possible reasons:${NC}"
    echo "  1. User hasn't approved the permission request yet"
    echo "  2. Permission has expired (72 hours)"
    echo "  3. Permission was revoked"
else
    echo -e "${RED}❌ Failed (HTTP $HTTP_CODE)${NC}"
    echo "$BODY"
fi

echo ""
echo "========================================"
echo -e "${YELLOW}Test complete!${NC}"
