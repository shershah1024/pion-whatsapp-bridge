# Azure Deployment Guide

Complete guide for deploying the WhatsApp Voice Bridge on Azure.

## 🎯 Recommended Architecture

**Best Option:** Azure Container Apps + Cloudflare Proxy (serverless, auto-scaling, DDoS protection)

```
WhatsApp → Cloudflare Proxy → Azure Container App → Azure OpenAI
            (DDoS, WAF)              ↓
                              Supabase (Database)
                                     ↓
                              Azure Key Vault (Secrets)
                                     ↓
                         Azure Application Insights (Monitoring)
```

**Why Cloudflare Proxy (not Tunnel)?**
- ✅ Azure Container Apps are **already public** - no tunnel needed!
- ✅ Cloudflare acts as reverse proxy for DDoS protection
- ✅ Custom domain + SSL management
- ✅ Rate limiting & WAF protection
- ✅ CDN & caching capabilities

---

## 🔄 Migration from Local + Tunnel → Azure + Proxy

### Before (Local Development)
```
Local Server (port 3011)
    ↓
Cloudflare Tunnel (cloudflared daemon)
    ↓
Public URL: whatsapp-bridge.tslfiles.org
```
**Problem:** Requires local machine running 24/7, tunnel daemon management

### After (Production on Azure)
```
Azure Container App (already has public URL)
    ↓
Cloudflare Proxy (DNS CNAME + orange cloud)
    ↓
Custom URL: whatsapp-bridge.tslfiles.org
```
**Benefits:** Serverless, auto-scaling, no tunnel daemon, better security

### What Changes?
| Component | Before | After |
|-----------|--------|-------|
| **Server** | Local machine | Azure Container Apps |
| **Public Access** | Cloudflare Tunnel | Native Azure HTTPS |
| **Cloudflare Role** | Tunnel (cloudflared) | Proxy (DNS only) |
| **Daemon** | cloudflared running | None needed |
| **Scaling** | Manual | Auto (1-10 replicas) |
| **Cost** | Electricity + ISP | ~$50-70/month |

---

## 🚀 Option 1: Azure Container Apps (Recommended)

**Why:** Serverless, auto-scaling, managed infrastructure, pay only for usage

### Step 1: Prepare Docker Image

**Create `Dockerfile`:**
```dockerfile
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o whatsapp-bridge .

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/
COPY --from=builder /app/whatsapp-bridge .

EXPOSE 3011

CMD ["./whatsapp-bridge"]
```

**Build and push to Azure Container Registry:**
```bash
# Login to Azure
az login

# Create resource group
az group create \
  --name whatsapp-bridge-rg \
  --location eastus

# Create Azure Container Registry
az acr create \
  --resource-group whatsapp-bridge-rg \
  --name whatsappbridgeacr \
  --sku Basic

# Login to ACR
az acr login --name whatsappbridgeacr

# Build and push
docker build -t whatsappbridgeacr.azurecr.io/whatsapp-bridge:latest .
docker push whatsappbridgeacr.azurecr.io/whatsapp-bridge:latest
```

### Step 2: Set Up Azure Key Vault

```bash
# Create Key Vault
az keyvault create \
  --name whatsapp-bridge-kv \
  --resource-group whatsapp-bridge-rg \
  --location eastus

# Add secrets
az keyvault secret set \
  --vault-name whatsapp-bridge-kv \
  --name azure-openai-api-key \
  --value "YOUR_AZURE_OPENAI_KEY"

az keyvault secret set \
  --vault-name whatsapp-bridge-kv \
  --name azure-openai-endpoint \
  --value "https://your-endpoint.openai.azure.com"

az keyvault secret set \
  --vault-name whatsapp-bridge-kv \
  --name azure-openai-deployment \
  --value "gpt-4o-realtime-preview"

az keyvault secret set \
  --vault-name whatsapp-bridge-kv \
  --name supabase-url \
  --value "https://gglkagcmyfdyojtgrzyv.supabase.co"

az keyvault secret set \
  --vault-name whatsapp-bridge-kv \
  --name supabase-anon-key \
  --value "YOUR_SUPABASE_KEY"

az keyvault secret set \
  --vault-name whatsapp-bridge-kv \
  --name whatsapp-phone-number-id \
  --value "YOUR_PHONE_NUMBER_ID"

az keyvault secret set \
  --vault-name whatsapp-bridge-kv \
  --name whatsapp-access-token \
  --value "YOUR_WHATSAPP_TOKEN"
```

### Step 3: Create Container App Environment

```bash
# Create Container Apps environment
az containerapp env create \
  --name whatsapp-bridge-env \
  --resource-group whatsapp-bridge-rg \
  --location eastus

# Create managed identity for Key Vault access
az identity create \
  --name whatsapp-bridge-identity \
  --resource-group whatsapp-bridge-rg

# Get identity details
IDENTITY_ID=$(az identity show \
  --name whatsapp-bridge-identity \
  --resource-group whatsapp-bridge-rg \
  --query id -o tsv)

PRINCIPAL_ID=$(az identity show \
  --name whatsapp-bridge-identity \
  --resource-group whatsapp-bridge-rg \
  --query principalId -o tsv)

# Grant Key Vault access to managed identity
az keyvault set-policy \
  --name whatsapp-bridge-kv \
  --object-id $PRINCIPAL_ID \
  --secret-permissions get list
```

### Step 4: Deploy Container App

**Create `containerapp.yaml`:**
```yaml
properties:
  managedEnvironmentId: /subscriptions/{subscription-id}/resourceGroups/whatsapp-bridge-rg/providers/Microsoft.App/managedEnvironments/whatsapp-bridge-env
  configuration:
    ingress:
      external: true
      targetPort: 3011
      transport: auto
      allowInsecure: false
    secrets:
      - name: azure-openai-api-key
        keyVaultUrl: https://whatsapp-bridge-kv.vault.azure.net/secrets/azure-openai-api-key
        identity: system
      - name: azure-openai-endpoint
        keyVaultUrl: https://whatsapp-bridge-kv.vault.azure.net/secrets/azure-openai-endpoint
        identity: system
      - name: azure-openai-deployment
        keyVaultUrl: https://whatsapp-bridge-kv.vault.azure.net/secrets/azure-openai-deployment
        identity: system
      - name: supabase-url
        keyVaultUrl: https://whatsapp-bridge-kv.vault.azure.net/secrets/supabase-url
        identity: system
      - name: supabase-anon-key
        keyVaultUrl: https://whatsapp-bridge-kv.vault.azure.net/secrets/supabase-anon-key
        identity: system
      - name: whatsapp-phone-number-id
        keyVaultUrl: https://whatsapp-bridge-kv.vault.azure.net/secrets/whatsapp-phone-number-id
        identity: system
      - name: whatsapp-access-token
        keyVaultUrl: https://whatsapp-bridge-kv.vault.azure.net/secrets/whatsapp-access-token
        identity: system
    registries:
      - server: whatsappbridgeacr.azurecr.io
        identity: system
  template:
    containers:
      - name: whatsapp-bridge
        image: whatsappbridgeacr.azurecr.io/whatsapp-bridge:latest
        resources:
          cpu: 1.0
          memory: 2Gi
        env:
          - name: AZURE_OPENAI_API_KEY
            secretRef: azure-openai-api-key
          - name: AZURE_OPENAI_ENDPOINT
            secretRef: azure-openai-endpoint
          - name: AZURE_OPENAI_DEPLOYMENT
            secretRef: azure-openai-deployment
          - name: SUPABASE_URL
            secretRef: supabase-url
          - name: SUPABASE_ANON_KEY
            secretRef: supabase-anon-key
          - name: WHATSAPP_PHONE_NUMBER_ID
            secretRef: whatsapp-phone-number-id
          - name: WHATSAPP_API_VERSION
            value: v21.0
          - name: WHATSAPP_ACCESS_TOKEN
            secretRef: whatsapp-access-token
    scale:
      minReplicas: 1
      maxReplicas: 10
      rules:
        - name: http-scaling
          http:
            metadata:
              concurrentRequests: "100"
```

**Deploy:**
```bash
az containerapp create \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg \
  --environment whatsapp-bridge-env \
  --image whatsappbridgeacr.azurecr.io/whatsapp-bridge:latest \
  --target-port 3011 \
  --ingress external \
  --min-replicas 1 \
  --max-replicas 10 \
  --cpu 1.0 \
  --memory 2Gi \
  --registry-server whatsappbridgeacr.azurecr.io \
  --registry-identity system \
  --user-assigned $IDENTITY_ID \
  --secrets \
    azure-openai-api-key=keyvaultref:https://whatsapp-bridge-kv.vault.azure.net/secrets/azure-openai-api-key,identityref:$IDENTITY_ID \
    azure-openai-endpoint=keyvaultref:https://whatsapp-bridge-kv.vault.azure.net/secrets/azure-openai-endpoint,identityref:$IDENTITY_ID \
    azure-openai-deployment=keyvaultref:https://whatsapp-bridge-kv.vault.azure.net/secrets/azure-openai-deployment,identityref:$IDENTITY_ID \
    supabase-url=keyvaultref:https://whatsapp-bridge-kv.vault.azure.net/secrets/supabase-url,identityref:$IDENTITY_ID \
    supabase-anon-key=keyvaultref:https://whatsapp-bridge-kv.vault.azure.net/secrets/supabase-anon-key,identityref:$IDENTITY_ID \
    whatsapp-phone-number-id=keyvaultref:https://whatsapp-bridge-kv.vault.azure.net/secrets/whatsapp-phone-number-id,identityref:$IDENTITY_ID \
    whatsapp-access-token=keyvaultref:https://whatsapp-bridge-kv.vault.azure.net/secrets/whatsapp-access-token,identityref:$IDENTITY_ID \
  --env-vars \
    AZURE_OPENAI_API_KEY=secretref:azure-openai-api-key \
    AZURE_OPENAI_ENDPOINT=secretref:azure-openai-endpoint \
    AZURE_OPENAI_DEPLOYMENT=secretref:azure-openai-deployment \
    SUPABASE_URL=secretref:supabase-url \
    SUPABASE_ANON_KEY=secretref:supabase-anon-key \
    WHATSAPP_PHONE_NUMBER_ID=secretref:whatsapp-phone-number-id \
    WHATSAPP_API_VERSION=v21.0 \
    WHATSAPP_ACCESS_TOKEN=secretref:whatsapp-access-token

# Get the app URL
az containerapp show \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg \
  --query properties.configuration.ingress.fqdn -o tsv
```

### Step 5: Configure Cloudflare Proxy (Recommended)

**No tunnel needed!** Azure Container Apps are already public. Use Cloudflare as a reverse proxy for DDoS protection.

**Get your Azure Container App URL:**
```bash
CONTAINER_APP_URL=$(az containerapp show \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg \
  --query properties.configuration.ingress.fqdn -o tsv)

echo $CONTAINER_APP_URL
# Output: whatsapp-bridge.xyz123.eastus.azurecontainerapps.io
```

**Add custom domain to Azure:**
```bash
# Add your custom domain to Container App
az containerapp hostname add \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg \
  --hostname whatsapp-bridge.tslfiles.org

# Get validation TXT record
az containerapp hostname show \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg \
  --hostname whatsapp-bridge.tslfiles.org
```

**Configure Cloudflare DNS:**

1. **Login to Cloudflare Dashboard** → Select your domain (`tslfiles.org`)

2. **DNS → Records → Add record:**
   ```
   Type: CNAME
   Name: whatsapp-bridge
   Target: whatsapp-bridge.xyz123.eastus.azurecontainerapps.io
   Proxy status: Proxied (orange cloud) ✅
   TTL: Auto
   ```

3. **Optional: Enable additional protection**
   - **Security → WAF** → Create rule to block suspicious requests
   - **Security → Rate Limiting** → Limit to 100 req/min per IP
   - **SSL/TLS** → Set to "Full (strict)"

4. **Verify DNS propagation:**
   ```bash
   dig whatsapp-bridge.tslfiles.org
   # Should show Cloudflare IPs (proxied)
   ```

**Update WhatsApp Webhook:**
```
Webhook URL: https://whatsapp-bridge.tslfiles.org/webhook
Verify Token: <your-verify-token>
```

**Optional: Stop old Cloudflare Tunnel (no longer needed)**
```bash
# The tunnel was only for local development
sudo systemctl stop cloudflared
sudo systemctl disable cloudflared

# Keep config files for reference, but they're no longer used
```

---

## 🚀 Option 2: Azure App Service (Alternative)

**Simpler, but less cost-effective than Container Apps**

### Deploy via Azure CLI

```bash
# Create App Service Plan
az appservice plan create \
  --name whatsapp-bridge-plan \
  --resource-group whatsapp-bridge-rg \
  --sku B1 \
  --is-linux

# Create Web App
az webapp create \
  --resource-group whatsapp-bridge-rg \
  --plan whatsapp-bridge-plan \
  --name whatsapp-bridge-app \
  --deployment-container-image-name whatsappbridgeacr.azurecr.io/whatsapp-bridge:latest

# Configure container registry
az webapp config container set \
  --name whatsapp-bridge-app \
  --resource-group whatsapp-bridge-rg \
  --docker-custom-image-name whatsappbridgeacr.azurecr.io/whatsapp-bridge:latest \
  --docker-registry-server-url https://whatsappbridgeacr.azurecr.io

# Enable managed identity
az webapp identity assign \
  --name whatsapp-bridge-app \
  --resource-group whatsapp-bridge-rg

# Configure environment variables from Key Vault
az webapp config appsettings set \
  --name whatsapp-bridge-app \
  --resource-group whatsapp-bridge-rg \
  --settings \
    AZURE_OPENAI_API_KEY="@Microsoft.KeyVault(VaultName=whatsapp-bridge-kv;SecretName=azure-openai-api-key)" \
    AZURE_OPENAI_ENDPOINT="@Microsoft.KeyVault(VaultName=whatsapp-bridge-kv;SecretName=azure-openai-endpoint)" \
    AZURE_OPENAI_DEPLOYMENT="@Microsoft.KeyVault(VaultName=whatsapp-bridge-kv;SecretName=azure-openai-deployment)" \
    SUPABASE_URL="@Microsoft.KeyVault(VaultName=whatsapp-bridge-kv;SecretName=supabase-url)" \
    SUPABASE_ANON_KEY="@Microsoft.KeyVault(VaultName=whatsapp-bridge-kv;SecretName=supabase-anon-key)" \
    WHATSAPP_PHONE_NUMBER_ID="@Microsoft.KeyVault(VaultName=whatsapp-bridge-kv;SecretName=whatsapp-phone-number-id)" \
    WHATSAPP_ACCESS_TOKEN="@Microsoft.KeyVault(VaultName=whatsapp-bridge-kv;SecretName=whatsapp-access-token)" \
    WHATSAPP_API_VERSION="v21.0"
```

---

## 📊 Option 3: Azure Virtual Machine (Traditional)

**For full control, similar to VPS approach**

```bash
# Create VM
az vm create \
  --resource-group whatsapp-bridge-rg \
  --name whatsapp-bridge-vm \
  --image Ubuntu2204 \
  --size Standard_B2s \
  --admin-username azureuser \
  --generate-ssh-keys \
  --public-ip-sku Standard

# SSH into VM
ssh azureuser@{vm-public-ip}

# Install Go
wget https://go.dev/dl/go1.23.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Clone and build
git clone {your-repo}
cd pion-whatsapp-bridge
go build .

# Set up systemd service (see PRODUCTION_CHECKLIST.md)
```

---

## 🔍 Monitoring with Azure Application Insights

### Enable Application Insights

```bash
# Create Application Insights
az monitor app-insights component create \
  --app whatsapp-bridge-insights \
  --location eastus \
  --resource-group whatsapp-bridge-rg \
  --application-type web

# Get instrumentation key
INSTRUMENTATION_KEY=$(az monitor app-insights component show \
  --app whatsapp-bridge-insights \
  --resource-group whatsapp-bridge-rg \
  --query instrumentationKey -o tsv)
```

### Add to Go Application

```bash
go get github.com/microsoft/ApplicationInsights-Go/appinsights
```

**Update `main.go`:**
```go
import "github.com/microsoft/ApplicationInsights-Go/appinsights"

var telemetryClient appinsights.TelemetryClient

func init() {
    telemetryConfig := appinsights.NewTelemetryConfiguration(os.Getenv("APPINSIGHTS_INSTRUMENTATION_KEY"))
    telemetryClient = appinsights.NewTelemetryClientFromConfig(telemetryConfig)
}

// Track events
telemetryClient.TrackEvent("CallAccepted", map[string]string{
    "callId": callID,
    "caller": phoneNumber,
})

// Track metrics
telemetryClient.TrackMetric("ActiveCalls", float64(len(activeCalls)))

// Track dependencies
dependency := appinsights.NewRemoteDependencyTelemetry(
    "WhatsApp API",
    "HTTP",
    "POST /v21.0/calls",
    true,
)
telemetryClient.Track(dependency)
```

### View Metrics in Azure Portal

1. Go to Azure Portal → Application Insights → whatsapp-bridge-insights
2. View:
   - **Live Metrics**: Real-time requests, failures, performance
   - **Failures**: Error tracking and stack traces
   - **Performance**: Response times, dependencies
   - **Usage**: Active users, sessions

---

## 🔐 Azure-Specific Security

### 1. Azure Key Vault Integration (Done Above)

**Rotate secrets:**
```bash
# Update secret
az keyvault secret set \
  --vault-name whatsapp-bridge-kv \
  --name azure-openai-api-key \
  --value "NEW_KEY"

# Restart app to pick up new secret
az containerapp revision restart \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg
```

### 2. Azure Private Link (Optional, for high security)

**Connect to Supabase via Private Link:**
```bash
# If Supabase supports Private Link
az network private-endpoint create \
  --name supabase-private-endpoint \
  --resource-group whatsapp-bridge-rg \
  --vnet-name whatsapp-bridge-vnet \
  --subnet default \
  --private-connection-resource-id {supabase-resource-id} \
  --group-id postgresqlServer \
  --connection-name supabase-connection
```

### 3. Azure Front Door (Optional, for DDoS protection)

```bash
az afd profile create \
  --profile-name whatsapp-bridge-afd \
  --resource-group whatsapp-bridge-rg \
  --sku Premium_AzureFrontDoor
```

---

## 💰 Cost Optimization

### Azure Container Apps Pricing (Estimated)

**Assumptions:** 1000 calls/day, avg 2 min each, 1 CPU, 2GB RAM

| Component | Monthly Cost |
|-----------|--------------|
| Container Apps (consumption) | ~$30-50 |
| Container Registry (Basic) | ~$5 |
| Key Vault | ~$3 |
| Application Insights | ~$10 |
| Bandwidth (outbound) | ~$10 |
| **Total** | **~$58-78/month** |

**Free tier eligible:**
- Container Apps: First 180,000 vCPU-seconds free/month
- Application Insights: First 5GB data free/month

### Cost Saving Tips

1. **Use consumption plan** (Container Apps) vs dedicated (App Service)
2. **Scale to zero** when not in use (Container Apps supports this)
3. **Use Basic tier ACR** instead of Standard/Premium
4. **Monitor and optimize** with Azure Cost Management

---

## 🚀 CI/CD with Azure DevOps

**Create `azure-pipelines.yml`:**
```yaml
trigger:
  - main

pool:
  vmImage: 'ubuntu-latest'

variables:
  imageName: 'whatsapp-bridge'
  acrName: 'whatsappbridgeacr'
  containerAppName: 'whatsapp-bridge'
  resourceGroup: 'whatsapp-bridge-rg'

stages:
  - stage: Build
    jobs:
      - job: BuildAndPush
        steps:
          - task: Docker@2
            displayName: Build and push image
            inputs:
              command: buildAndPush
              repository: $(imageName)
              dockerfile: Dockerfile
              containerRegistry: $(acrName)
              tags: |
                $(Build.BuildId)
                latest

  - stage: Test
    jobs:
      - job: RunTests
        steps:
          - task: Go@0
            inputs:
              command: 'test'
              arguments: '-v ./...'

  - stage: Deploy
    dependsOn: [Build, Test]
    jobs:
      - deployment: DeployToAzure
        environment: 'production'
        strategy:
          runOnce:
            deploy:
              steps:
                - task: AzureCLI@2
                  inputs:
                    azureSubscription: 'Azure-Subscription'
                    scriptType: 'bash'
                    scriptLocation: 'inlineScript'
                    inlineScript: |
                      az containerapp update \
                        --name $(containerAppName) \
                        --resource-group $(resourceGroup) \
                        --image $(acrName).azurecr.io/$(imageName):$(Build.BuildId)
```

---

## 📈 Auto-Scaling Configuration

**Container Apps auto-scales based on:**

```bash
# HTTP concurrency scaling
az containerapp update \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg \
  --min-replicas 1 \
  --max-replicas 10 \
  --scale-rule-name http-rule \
  --scale-rule-type http \
  --scale-rule-http-concurrency 50

# CPU scaling
az containerapp update \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg \
  --scale-rule-name cpu-rule \
  --scale-rule-type cpu \
  --scale-rule-metadata threshold=70
```

---

## 🔧 Troubleshooting Azure Deployment

### View Container Logs
```bash
# Stream logs
az containerapp logs show \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg \
  --follow

# View recent logs
az containerapp logs tail \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg
```

### Debug Key Vault Access
```bash
# Check managed identity has access
az keyvault show \
  --name whatsapp-bridge-kv \
  --query properties.accessPolicies
```

### Test Container Locally
```bash
# Run container with env vars
docker run -p 3011:3011 \
  -e AZURE_OPENAI_API_KEY="xxx" \
  -e AZURE_OPENAI_ENDPOINT="xxx" \
  whatsappbridgeacr.azurecr.io/whatsapp-bridge:latest
```

---

## ✅ Azure Deployment Checklist

### Pre-Deployment
- [ ] Create Azure account
- [ ] Install Azure CLI (`az`)
- [ ] Login: `az login`
- [ ] Create resource group
- [ ] Create Container Registry
- [ ] Build and push Docker image

### Secrets Management
- [ ] Create Key Vault
- [ ] Add all secrets to Key Vault
- [ ] Create managed identity
- [ ] Grant Key Vault access to identity

### Deployment
- [ ] Create Container Apps environment
- [ ] Deploy Container App
- [ ] Configure auto-scaling
- [ ] Set up Application Insights

### Post-Deployment
- [ ] Add custom domain to Azure Container App
- [ ] Configure Cloudflare DNS (CNAME with proxy enabled)
- [ ] Update WhatsApp webhook URL
- [ ] Test inbound call
- [ ] Test outbound call
- [ ] Test reminder creation
- [ ] Monitor logs and metrics
- [ ] Stop old Cloudflare Tunnel (no longer needed)

### Production Hardening
- [ ] Set up alerts (service down, high errors)
- [ ] Enable backups (Supabase)
- [ ] Configure auto-scaling rules
- [ ] Set up CI/CD pipeline
- [ ] Document runbook

---

## 🎯 Quick Start: Deploy to Azure in 20 Minutes

```bash
# 1. Login and create resources
az login
az group create --name whatsapp-bridge-rg --location eastus

# 2. Create and configure ACR
az acr create --resource-group whatsapp-bridge-rg --name whatsappbridgeacr --sku Basic
az acr login --name whatsappbridgeacr

# 3. Build and push Docker image
docker build -t whatsappbridgeacr.azurecr.io/whatsapp-bridge:latest .
docker push whatsappbridgeacr.azurecr.io/whatsapp-bridge:latest

# 4. Create Key Vault and add secrets (see Step 2 above)

# 5. Deploy Container App (see Step 4 above)

# 6. Configure Cloudflare Proxy
# Get Azure URL
AZURE_URL=$(az containerapp show --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg \
  --query properties.configuration.ingress.fqdn -o tsv)

# Add custom domain to Azure
az containerapp hostname add \
  --name whatsapp-bridge \
  --resource-group whatsapp-bridge-rg \
  --hostname whatsapp-bridge.tslfiles.org

# In Cloudflare Dashboard:
# - Add CNAME record: whatsapp-bridge → $AZURE_URL
# - Enable proxy (orange cloud)
# - Set SSL to "Full (strict)"

# 7. Update WhatsApp webhook to https://whatsapp-bridge.tslfiles.org/webhook

# 8. Optional: Stop old Cloudflare Tunnel (no longer needed)
sudo systemctl stop cloudflared
sudo systemctl disable cloudflared

# 9. Test!
curl https://whatsapp-bridge.tslfiles.org/health
```

You're now running on Azure with:
- ✅ Enterprise-grade security (Key Vault)
- ✅ Auto-scaling (1-10 replicas)
- ✅ DDoS protection (Cloudflare Proxy)
- ✅ Built-in monitoring (Application Insights)
- ✅ Custom domain with SSL

**Total cost:** ~$50-70/month 🚀
