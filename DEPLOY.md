# Azure Container Apps Deployment Guide

**For complete Azure deployment instructions, see [AZURE_DEPLOYMENT.md](AZURE_DEPLOYMENT.md)**

This guide provides a quick reference for deploying to Azure Container Apps.

## Quick Deploy to Azure

See the comprehensive guide in [AZURE_DEPLOYMENT.md](AZURE_DEPLOYMENT.md) which includes:

- Step-by-step Azure Container Apps setup
- Azure Key Vault configuration for secrets
- Cloudflare proxy setup for DDoS protection
- Auto-scaling configuration
- Application Insights monitoring
- CI/CD pipeline setup
- Cost optimization tips

## Prerequisites

1. Azure account
2. Azure CLI installed (`az`)
3. Docker installed (for building images)

## Quick Start (20 minutes)

```bash
# Login to Azure
az login

# Run the deployment script
./deploy-azure.sh
```

This script will:
1. Create Azure Container Registry
2. Build and push Docker image
3. Create Key Vault and store secrets
4. Deploy to Azure Container Apps
5. Configure auto-scaling and monitoring

## Advantages over Local Deployment

- ✅ **Serverless** - No server management required
- ✅ **Auto-scaling** - Scales from 1 to 10+ replicas based on load
- ✅ **Enterprise Security** - Secrets managed in Azure Key Vault
- ✅ **Built-in Monitoring** - Application Insights for logs and metrics
- ✅ **High Availability** - 99.95% SLA with automatic failover
- ✅ **Cost Effective** - Pay only for what you use (~$50-70/month)