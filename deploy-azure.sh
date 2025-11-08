#!/bin/bash

# Azure Deployment Script for WhatsApp Voice Bridge
# This script automates the deployment to Azure Container Apps

set -e

echo "🚀 Azure Container Apps Deployment for WhatsApp Voice Bridge"
echo "=============================================================="

# Configuration
RESOURCE_GROUP="whatsapp-bridge-rg"
LOCATION="eastus"
ACR_NAME="whatsappbridgeacr"
KEY_VAULT_NAME="whatsapp-bridge-kv"
CONTAINER_ENV="whatsapp-bridge-env"
CONTAINER_APP="whatsapp-bridge"
IDENTITY_NAME="whatsapp-bridge-identity"
IMAGE_NAME="whatsapp-bridge"

echo ""
echo "📋 Configuration:"
echo "  Resource Group: $RESOURCE_GROUP"
echo "  Location: $LOCATION"
echo "  ACR Name: $ACR_NAME"
echo "  Container App: $CONTAINER_APP"
echo ""

# Check if Azure CLI is installed
if ! command -v az &> /dev/null; then
    echo "❌ Azure CLI is not installed. Please install it first:"
    echo "   https://docs.microsoft.com/en-us/cli/azure/install-azure-cli"
    exit 1
fi

# Check if logged in
echo "🔐 Checking Azure login status..."
if ! az account show &> /dev/null; then
    echo "❌ Not logged in to Azure. Please run: az login"
    exit 1
fi

SUBSCRIPTION_ID=$(az account show --query id -o tsv)
echo "✅ Logged in to subscription: $SUBSCRIPTION_ID"

# Step 1: Create Resource Group
echo ""
echo "📦 Step 1: Creating Resource Group..."
if az group show --name $RESOURCE_GROUP &> /dev/null; then
    echo "✅ Resource group already exists"
else
    az group create --name $RESOURCE_GROUP --location $LOCATION
    echo "✅ Resource group created"
fi

# Step 2: Create Azure Container Registry
echo ""
echo "🐳 Step 2: Creating Azure Container Registry..."
if az acr show --name $ACR_NAME --resource-group $RESOURCE_GROUP &> /dev/null; then
    echo "✅ ACR already exists"
else
    az acr create \
        --resource-group $RESOURCE_GROUP \
        --name $ACR_NAME \
        --sku Basic
    echo "✅ ACR created"
fi

# Login to ACR
echo "🔐 Logging into ACR..."
az acr login --name $ACR_NAME
echo "✅ Logged into ACR"

# Step 3: Build and Push Docker Image
echo ""
echo "🔨 Step 3: Building and pushing Docker image..."
FULL_IMAGE_NAME="$ACR_NAME.azurecr.io/$IMAGE_NAME:latest"
docker build -t $FULL_IMAGE_NAME .
docker push $FULL_IMAGE_NAME
echo "✅ Docker image built and pushed: $FULL_IMAGE_NAME"

# Step 4: Create Key Vault
echo ""
echo "🔑 Step 4: Creating Azure Key Vault..."
if az keyvault show --name $KEY_VAULT_NAME --resource-group $RESOURCE_GROUP &> /dev/null; then
    echo "✅ Key Vault already exists"
else
    az keyvault create \
        --name $KEY_VAULT_NAME \
        --resource-group $RESOURCE_GROUP \
        --location $LOCATION
    echo "✅ Key Vault created"
fi

# Step 5: Add Secrets to Key Vault
echo ""
echo "🔐 Step 5: Adding secrets to Key Vault..."
echo ""
echo "⚠️  IMPORTANT: You need to provide the following secrets."
echo "    Press Enter to skip if already configured, or paste the value."
echo ""

# Function to add secret if not exists
add_secret_if_needed() {
    local secret_name=$1
    local prompt_text=$2

    if az keyvault secret show --vault-name $KEY_VAULT_NAME --name $secret_name &> /dev/null; then
        echo "✅ Secret '$secret_name' already exists (skipping)"
    else
        echo -n "$prompt_text: "
        read -r secret_value
        if [ -n "$secret_value" ]; then
            az keyvault secret set \
                --vault-name $KEY_VAULT_NAME \
                --name $secret_name \
                --value "$secret_value" > /dev/null
            echo "✅ Secret '$secret_name' added"
        else
            echo "⚠️  Skipped '$secret_name' - add manually later"
        fi
    fi
}

add_secret_if_needed "azure-openai-api-key" "Azure OpenAI API Key"
add_secret_if_needed "azure-openai-endpoint" "Azure OpenAI Endpoint (e.g., https://your-endpoint.openai.azure.com)"
add_secret_if_needed "azure-openai-deployment" "Azure OpenAI Deployment (e.g., gpt-4o-realtime-preview)"
add_secret_if_needed "supabase-url" "Supabase URL"
add_secret_if_needed "supabase-anon-key" "Supabase Anon Key"
add_secret_if_needed "whatsapp-phone-number-id" "WhatsApp Phone Number ID"
add_secret_if_needed "whatsapp-access-token" "WhatsApp Access Token"

# Step 6: Create Managed Identity
echo ""
echo "🆔 Step 6: Creating Managed Identity..."
if az identity show --name $IDENTITY_NAME --resource-group $RESOURCE_GROUP &> /dev/null; then
    echo "✅ Managed identity already exists"
else
    az identity create \
        --name $IDENTITY_NAME \
        --resource-group $RESOURCE_GROUP
    echo "✅ Managed identity created"
fi

IDENTITY_ID=$(az identity show \
    --name $IDENTITY_NAME \
    --resource-group $RESOURCE_GROUP \
    --query id -o tsv)

PRINCIPAL_ID=$(az identity show \
    --name $IDENTITY_NAME \
    --resource-group $RESOURCE_GROUP \
    --query principalId -o tsv)

echo "Identity ID: $IDENTITY_ID"
echo "Principal ID: $PRINCIPAL_ID"

# Grant Key Vault access
echo "🔐 Granting Key Vault access to managed identity..."
az keyvault set-policy \
    --name $KEY_VAULT_NAME \
    --object-id $PRINCIPAL_ID \
    --secret-permissions get list
echo "✅ Key Vault access granted"

# Step 7: Create Container Apps Environment
echo ""
echo "🌍 Step 7: Creating Container Apps environment..."
if az containerapp env show --name $CONTAINER_ENV --resource-group $RESOURCE_GROUP &> /dev/null; then
    echo "✅ Container Apps environment already exists"
else
    az containerapp env create \
        --name $CONTAINER_ENV \
        --resource-group $RESOURCE_GROUP \
        --location $LOCATION
    echo "✅ Container Apps environment created"
fi

# Step 8: Deploy Container App
echo ""
echo "🚀 Step 8: Deploying Container App..."
if az containerapp show --name $CONTAINER_APP --resource-group $RESOURCE_GROUP &> /dev/null; then
    echo "Updating existing Container App..."
    az containerapp update \
        --name $CONTAINER_APP \
        --resource-group $RESOURCE_GROUP \
        --image $FULL_IMAGE_NAME
    echo "✅ Container App updated"
else
    echo "Creating new Container App..."
    az containerapp create \
        --name $CONTAINER_APP \
        --resource-group $RESOURCE_GROUP \
        --environment $CONTAINER_ENV \
        --image $FULL_IMAGE_NAME \
        --target-port 3011 \
        --ingress external \
        --min-replicas 1 \
        --max-replicas 10 \
        --cpu 1.0 \
        --memory 2Gi \
        --registry-server $ACR_NAME.azurecr.io \
        --registry-identity system \
        --user-assigned $IDENTITY_ID \
        --secrets \
            azure-openai-api-key=keyvaultref:https://$KEY_VAULT_NAME.vault.azure.net/secrets/azure-openai-api-key,identityref:$IDENTITY_ID \
            azure-openai-endpoint=keyvaultref:https://$KEY_VAULT_NAME.vault.azure.net/secrets/azure-openai-endpoint,identityref:$IDENTITY_ID \
            azure-openai-deployment=keyvaultref:https://$KEY_VAULT_NAME.vault.azure.net/secrets/azure-openai-deployment,identityref:$IDENTITY_ID \
            supabase-url=keyvaultref:https://$KEY_VAULT_NAME.vault.azure.net/secrets/supabase-url,identityref:$IDENTITY_ID \
            supabase-anon-key=keyvaultref:https://$KEY_VAULT_NAME.vault.azure.net/secrets/supabase-anon-key,identityref:$IDENTITY_ID \
            whatsapp-phone-number-id=keyvaultref:https://$KEY_VAULT_NAME.vault.azure.net/secrets/whatsapp-phone-number-id,identityref:$IDENTITY_ID \
            whatsapp-access-token=keyvaultref:https://$KEY_VAULT_NAME.vault.azure.net/secrets/whatsapp-access-token,identityref:$IDENTITY_ID \
        --env-vars \
            AZURE_OPENAI_API_KEY=secretref:azure-openai-api-key \
            AZURE_OPENAI_ENDPOINT=secretref:azure-openai-endpoint \
            AZURE_OPENAI_DEPLOYMENT=secretref:azure-openai-deployment \
            SUPABASE_URL=secretref:supabase-url \
            SUPABASE_ANON_KEY=secretref:supabase-anon-key \
            WHATSAPP_PHONE_NUMBER_ID=secretref:whatsapp-phone-number-id \
            WHATSAPP_API_VERSION=v21.0 \
            WHATSAPP_ACCESS_TOKEN=secretref:whatsapp-access-token
    echo "✅ Container App created"
fi

# Get the app URL
echo ""
echo "🌐 Getting Container App URL..."
CONTAINER_APP_URL=$(az containerapp show \
    --name $CONTAINER_APP \
    --resource-group $RESOURCE_GROUP \
    --query properties.configuration.ingress.fqdn -o tsv)

echo ""
echo "=============================================================="
echo "✅ Deployment Complete!"
echo "=============================================================="
echo ""
echo "📍 Container App URL: https://$CONTAINER_APP_URL"
echo ""
echo "🔗 Next Steps:"
echo "1. Test the deployment:"
echo "   curl https://$CONTAINER_APP_URL/health"
echo ""
echo "2. Configure Cloudflare Proxy (for custom domain):"
echo "   - Add custom domain to Azure:"
echo "     az containerapp hostname add \\"
echo "       --name $CONTAINER_APP \\"
echo "       --resource-group $RESOURCE_GROUP \\"
echo "       --hostname whatsapp-bridge.tslfiles.org"
echo ""
echo "   - In Cloudflare DNS, add CNAME record:"
echo "     Name: whatsapp-bridge"
echo "     Target: $CONTAINER_APP_URL"
echo "     Proxy: Enabled (orange cloud)"
echo ""
echo "3. Update WhatsApp webhook URL to:"
echo "   https://$CONTAINER_APP_URL/whatsapp-call"
echo "   (or your custom domain after Cloudflare setup)"
echo ""
echo "4. View logs:"
echo "   az containerapp logs show --name $CONTAINER_APP --resource-group $RESOURCE_GROUP --follow"
echo ""
echo "=============================================================="
