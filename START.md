# Server Startup Commands

## 1. Start Cloudflare Tunnel

```bash
cloudflared tunnel --config /Users/imaginedemo/projects/whatsapp-call-research/pion-whatsapp-bridge/cloudflared-config.yml run fbc9f22a-6dac-4be1-8f61-48c9dac102f3
```

Or run in background:

```bash
cloudflared tunnel --config /Users/imaginedemo/projects/whatsapp-call-research/pion-whatsapp-bridge/cloudflared-config.yml run fbc9f22a-6dac-4be1-8f61-48c9dac102f3 &
```

## 2. Start Go Server

```bash
go run *.go
```

Or using specific files:

```bash
go run audio_processor.go main.go openai_realtime.go
```

## Server Info

- **Local Port**: 3011
- **Public URL**: https://whatsapp-bridge.tslfiles.org
- **Webhook**: https://whatsapp-bridge.tslfiles.org/whatsapp-call
- **Health Check**: https://whatsapp-bridge.tslfiles.org/health
- **Initiate Outbound Call**: POST to http://localhost:3011/initiate-call

## 3. Initiate Outbound Call (Optional)

To call a WhatsApp user (requires permission):

```bash
./test_outbound_call.sh
```

Or manually:

```bash
curl -X POST http://localhost:3011/initiate-call \
  -H "Content-Type: application/json" \
  -d '{"to": "919885842349"}'
```

## Troubleshooting

Kill processes if port is in use:

```bash
# Kill Go server
pkill -f "go run"

# Kill port 3011
lsof -ti:3011 | xargs kill -9

# Kill cloudflared
pkill cloudflared
```
https://whatsapp-bridge.agreeablehill-44d96eb3.eastus.azurecontainerapps.io/whatsapp-call

 https://whatsapp-bridge.tslfiles.org/whatsapp-call

## Webhook URLs
- **Production**: https://whatsapp-bridge.tslfiles.org/whatsapp-call
- **Azure**: https://whatsapp-bridge.agreeablehill-44d96eb3.eastus.azurecontainerapps.io/whatsapp-call


cloudflared tunnel --config /Users/imaginedemo/projects/whatsapp-call-research/pion-whatsapp-bridge/cloudflared-config.yml run
  fbc9f22a-6dac-4be1-8f61-48c9dac102f3 &

  