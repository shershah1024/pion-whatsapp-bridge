# Azure Quick Start Guide

This guide provides step-by-step instructions to deploy the WhatsApp Voice Bridge to Azure Container Apps.

## Prerequisites

1. **Azure CLI installed**
   ```bash
   # macOS
   brew install azure-cli

   # Windows
   # Download from: https://aka.ms/installazurecliwindows

   # Linux
   curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
   ```

2. **Docker installed and running**

3. **Azure account** with an active subscription

4. **Environment variables ready:**
   - Azure OpenAI API Key
   - Azure OpenAI Endpoint
   - Azure OpenAI Deployment name
   - Supabase URL
   - Supabase Anon Key
   - WhatsApp Phone Number ID
   - WhatsApp Access Token

## Automated Deployment (Recommended)

### Option 1: One-Command Deployment

```bash
# Login to Azure first
az login

# Run the deployment script
./deploy-azure.sh
```

The script will:
- ✅ Create all Azure resources
- ✅ Build and push Docker image
- ✅ Set up Key Vault with your secrets
- ✅ Deploy to Container Apps
- ✅ Configure auto-scaling
- ✅ Provide you with the public URL

**Time to deploy:** ~15-20 minutes

### Option 2: Step-by-Step Manual Deployment

If you prefer manual control, follow the detailed steps in `AZURE_DEPLOYMENT.md`.

## Post-Deployment Steps

### 1. Test Your Deployment

```bash
# Get your Container App URL from the deployment output
CONTAINER_APP_URL="<your-url-from-deployment-output>"

# Test health endpoint
curl https://$CONTAINER_APP_URL/health

# Expected response:
# {"status":"ok"}
```

### 2. Configure Custom Domain with Cloudflare (Optional but Recommended)

#### Add Domain to Azure
```bash
az containerapp hostname add \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg \
  --hostname whatsapp-bridge.tslfiles.org
```

#### Configure Cloudflare DNS

1. Login to **Cloudflare Dashboard**
2. Go to your domain (e.g., `tslfiles.org`)
3. **DNS → Records → Add record:**
   ```
   Type: CNAME
   Name: whatsapp-bridge
   Target: <your-azure-container-app-url>
   Proxy: Enabled (orange cloud ☁️)
   TTL: Auto
   ```

4. **Security Settings (Optional):**
   - **SSL/TLS** → Set to "Full (strict)"
   - **Security → Rate Limiting** → 100 req/min per IP
   - **Security → WAF** → Enable for DDoS protection

5. **Verify DNS:**
   ```bash
   dig whatsapp-bridge.tslfiles.org
   # Should show Cloudflare IPs (proxied)
   ```

### 3. Update WhatsApp Webhook

1. Go to **Meta for Developers** → Your App → WhatsApp → Configuration
2. Update webhook URL:
   ```
   URL: https://whatsapp-bridge.tslfiles.org/whatsapp-call
   Verify Token: whatsapp_bridge_token
   ```
3. Subscribe to webhook fields:
   - ✅ messages
   - ✅ calls
   - ✅ message_status

### 4. Test End-to-End

#### Test Inbound Call
1. Call your WhatsApp Business number
2. Should hear AI voice greeting
3. Test conversation with the AI

#### Test Outbound Call
```bash
curl -X POST https://whatsapp-bridge.tslfiles.org/initiate-call \
  -H "Content-Type: application/json" \
  -d '{"to": "14085551234"}'
```

#### Test Text Messaging
Send a text message to your WhatsApp Business number and check for AI response.

## Monitoring and Logs

### View Real-Time Logs
```bash
az containerapp logs show \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg \
  --follow
```

### View Metrics
```bash
# Open Azure Portal
az containerapp browse \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg
```

Go to **Monitoring → Metrics** to view:
- Request count
- Response time
- CPU/Memory usage
- Replica count (auto-scaling)

## Updating Your Deployment

### Update Code
```bash
# Make code changes, then rebuild and push
az acr login --name whatsappbridgeacr

docker build -t whatsappbridgeacr.azurecr.io/whatsapp-bridge:latest .
docker push whatsappbridgeacr.azurecr.io/whatsapp-bridge:latest

# Update Container App
az containerapp update \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg \
  --image whatsappbridgeacr.azurecr.io/whatsapp-bridge:latest
```

### Update Secrets
```bash
# Update a secret in Key Vault
az keyvault secret set \
  --vault-name whatsapp-bridge-kv \
  --name azure-openai-api-key \
  --value "NEW_KEY"

# Restart app to pick up new secret
az containerapp revision restart \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg
```

## Cost Management

**Expected Monthly Cost:** ~$50-70

| Component | Monthly Cost |
|-----------|--------------|
| Container Apps | ~$30-50 |
| Container Registry | ~$5 |
| Key Vault | ~$3 |
| Application Insights | ~$10 |
| Bandwidth | ~$10 |

### Cost Optimization Tips
- Container Apps uses consumption pricing (pay only for usage)
- Scales to zero when idle (if configured)
- Use Basic tier ACR instead of Standard/Premium
- Monitor costs in Azure Portal → Cost Management

## Troubleshooting

### Container App Not Starting
```bash
# Check logs
az containerapp logs tail \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg

# Check revision status
az containerapp revision list \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg
```

### Key Vault Access Issues
```bash
# Verify managed identity has access
az keyvault show \
  --name whatsapp-bridge-kv \
  --query properties.accessPolicies
```

### Test Container Locally
```bash
# Pull and run image locally to debug
docker run -p 3011:3011 \
  -e AZURE_OPENAI_API_KEY="xxx" \
  -e AZURE_OPENAI_ENDPOINT="xxx" \
  whatsappbridgeacr.azurecr.io/whatsapp-bridge:latest
```

## Migration from Local/Cloudflare Tunnel

### What Changes?
- **Before:** Local machine + Cloudflare Tunnel (cloudflared daemon)
- **After:** Azure Container Apps + Cloudflare Proxy (DNS only)

### Stop Old Tunnel (After Migration)
```bash
# No longer needed - Azure Container Apps are already public
sudo systemctl stop cloudflared
sudo systemctl disable cloudflared

# Keep config files for reference, but they're no longer used
```

### Benefits of New Setup
- ✅ No local machine running 24/7
- ✅ Auto-scaling (1-10 replicas based on load)
- ✅ Enterprise security with Key Vault
- ✅ DDoS protection via Cloudflare
- ✅ Built-in monitoring and logging
- ✅ Zero-downtime deployments

## Support

- **Azure Issues:** Check logs with `az containerapp logs`
- **WhatsApp Issues:** Verify webhook configuration
- **OpenAI Issues:** Check API key and endpoint in Key Vault
- **Cloudflare Issues:** Verify DNS settings and proxy status

For detailed architecture and advanced configurations, see `AZURE_DEPLOYMENT.md`.
