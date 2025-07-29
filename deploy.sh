#!/bin/bash

# Pion WhatsApp Bridge Deployment Script
echo "🚀 Pion WhatsApp Bridge Deployment"
echo "==================================="
echo ""

# Check prerequisites
echo "🔍 Checking prerequisites..."

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed"
    echo "📦 Please install Go from https://golang.org/dl/"
    echo ""
    echo "On macOS with Homebrew:"
    echo "  brew install go"
    echo ""
    exit 1
fi

# Check if ngrok is installed
if ! command -v ngrok &> /dev/null; then
    echo "❌ ngrok is not installed"
    echo "📦 Please install ngrok from https://ngrok.com/"
    echo ""
    echo "On macOS with Homebrew:"
    echo "  brew install ngrok"
    echo ""
    exit 1
fi

echo "✅ All prerequisites met"
echo "  Go version: $(go version)"
echo ""

# Function to cleanup on exit
cleanup() {
    echo -e "\n🛑 Shutting down services..."
    if [ ! -z "$BRIDGE_PID" ]; then
        echo "🌉 Stopping Pion Bridge..."
        kill $BRIDGE_PID 2>/dev/null
    fi
    if [ ! -z "$NGROK_PID" ]; then
        echo "🚇 Stopping ngrok tunnel..."
        kill $NGROK_PID 2>/dev/null
    fi
    echo "✅ All services stopped"
    exit 0
}

# Set up signal handlers
trap cleanup SIGINT SIGTERM

# Step 1: Build the Go application
echo "🔨 Building Pion WhatsApp Bridge..."
go mod download
go build -o pion-whatsapp-bridge .

if [ ! -f "./pion-whatsapp-bridge" ]; then
    echo "❌ Failed to build the application"
    exit 1
fi

echo "✅ Build successful"
echo ""

# Step 2: Start the Pion Bridge
echo "🌉 Starting Pion WhatsApp Bridge..."
./pion-whatsapp-bridge &
BRIDGE_PID=$!

# Wait for bridge to start
echo "⏳ Waiting for bridge to initialize..."
sleep 3

# Check if bridge is running
if ! kill -0 $BRIDGE_PID 2>/dev/null; then
    echo "❌ Failed to start Pion Bridge"
    exit 1
fi

# Verify bridge is responding
if ! curl -s http://localhost:3000/health > /dev/null 2>&1; then
    echo "❌ Bridge service not responding on port 3000"
    kill $BRIDGE_PID 2>/dev/null
    exit 1
fi

echo "✅ Pion Bridge started successfully"
echo ""

# Step 3: Start ngrok tunnel
echo "🚇 Creating public tunnel with ngrok..."
ngrok http 3000 --log=stdout &
NGROK_PID=$!

echo "⏳ Waiting for ngrok tunnel to establish..."
sleep 4

# Get the public URL from ngrok API
PUBLIC_URL=$(curl -s http://localhost:4040/api/tunnels | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    tunnel = data['tunnels'][0]
    print(tunnel['public_url'])
except:
    print('ERROR')
" 2>/dev/null)

if [ "$PUBLIC_URL" = "ERROR" ] || [ -z "$PUBLIC_URL" ]; then
    echo "❌ Failed to get ngrok public URL"
    echo "💡 Trying alternative method..."
    
    # Alternative: check ngrok web interface
    sleep 2
    PUBLIC_URL=$(curl -s http://localhost:4040/api/tunnels | grep -o 'https://[^"]*\.ngrok\.io' | head -1)
    
    if [ -z "$PUBLIC_URL" ]; then
        echo "❌ Could not establish public tunnel"
        echo "💡 Please check ngrok authentication"
        kill $BRIDGE_PID $NGROK_PID 2>/dev/null
        exit 1
    fi
fi

echo "✅ Public tunnel established"
echo ""

# Step 4: Test the system
echo "🧪 Testing the system..."

# Test bridge status
BRIDGE_STATUS=$(curl -s http://localhost:3000/status | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    print('OK' if data.get('status') == 'running' else 'FAIL')
except:
    print('FAIL')
" 2>/dev/null)

if [ "$BRIDGE_STATUS" != "OK" ]; then
    echo "❌ Bridge status check failed"
    kill $BRIDGE_PID $NGROK_PID 2>/dev/null
    exit 1
fi

# Test webhook verification
VERIFY_TEST=$(curl -s "$PUBLIC_URL/whatsapp-call?hub.mode=subscribe&hub.verify_token=whatsapp_bridge_token&hub.challenge=test123")

if [ "$VERIFY_TEST" != "test123" ]; then
    echo "❌ Webhook verification failed"
    echo "Response: $VERIFY_TEST"
    kill $BRIDGE_PID $NGROK_PID 2>/dev/null
    exit 1
fi

echo "✅ All system tests passed!"
echo ""

# Display deployment information
echo "🎉 DEPLOYMENT COMPLETE!"
echo "======================="
echo ""
echo "📡 Services Running:"
echo "  • Pion Bridge:       http://localhost:3000"
echo "  • Public Endpoint:   $PUBLIC_URL"
echo ""
echo "📱 WhatsApp Business API Configuration:"
echo "  Webhook URL:         $PUBLIC_URL/whatsapp-call"
echo "  Verify Token:        whatsapp_bridge_token"
echo "  HTTP Method:         POST"
echo ""
echo "🔍 Monitoring URLs:"
echo "  • Bridge Status:     $PUBLIC_URL/status"
echo "  • Health Check:      $PUBLIC_URL/health"
echo "  • ngrok Dashboard:   http://localhost:4040"
echo ""
echo "🧪 Test Commands:"
echo ""
echo "# Test webhook verification:"
echo "curl '$PUBLIC_URL/whatsapp-call?hub.mode=subscribe&hub.verify_token=whatsapp_bridge_token&hub.challenge=test123'"
echo ""
echo "# Test call with SDP:"
echo "curl -X POST '$PUBLIC_URL/test-call' \\"
echo "  -H 'Content-Type: application/json' \\"
echo "  -d '{\"sdp\":\"v=0\\no=whatsapp 0 0 IN IP4 192.168.1.100\\ns=Test\\nc=IN IP4 192.168.1.100\\nt=0 0\\nm=audio 12345 RTP/AVP 8\\na=rtpmap:8 PCMA/8000\\na=sendrecv\"}'"
echo ""
echo "✨ Advantages of Pion implementation:"
echo "  • Native ice-lite support"
echo "  • Single Go binary - no complex dependencies"
echo "  • Direct SDP control for WhatsApp compatibility"
echo "  • Built-in audio detection"
echo "  • Simpler deployment"
echo ""
echo "🚀 System is ready for WhatsApp Business API integration!"
echo ""
echo "💡 Keep this terminal open to maintain the services"
echo "   Press Ctrl+C to stop all services"
echo ""

# Keep services running
wait