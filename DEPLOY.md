# Railway Deployment Guide

## Prerequisites

1. GitHub account
2. Railway account (sign up at [railway.app](https://railway.app))

## Step 1: Push to GitHub

```bash
# Initialize git repository
git init

# Add all files
git add .

# Commit
git commit -m "Initial commit: Pion WhatsApp Bridge"

# Add your GitHub repository as origin
git remote add origin https://github.com/YOUR_USERNAME/pion-whatsapp-bridge.git

# Push to GitHub
git push -u origin main
```

## Step 2: Deploy on Railway

### Option A: Deploy Button (Add to your README)

Add this button to your GitHub README:

```markdown
[![Deploy on Railway](https://railway.app/button.svg)](https://railway.app/template/YOUR_TEMPLATE)
```

### Option B: Manual Deploy

1. Go to [railway.app](https://railway.app)
2. Click "New Project"
3. Choose "Deploy from GitHub repo"
4. Select your repository
5. Railway will automatically detect it's a Go project and deploy

## Step 3: Configure Environment Variables

In Railway dashboard:

1. Go to your project
2. Click on "Variables"
3. Add these required variables:
   ```
   WHATSAPP_TOKEN=your_whatsapp_access_token
   PHONE_NUMBER_ID=your_phone_number_id
   VERIFY_TOKEN=your_verify_token
   WHATSAPP_WEBHOOK_SECRET=your_webhook_secret
   ```

### How to get these values:

1. **WHATSAPP_TOKEN**:
   - Go to [Meta for Developers](https://developers.facebook.com)
   - Navigate to your app → WhatsApp → API Setup
   - Copy the temporary or permanent access token

2. **PHONE_NUMBER_ID**:
   - In the same API Setup page
   - Find your phone number
   - Copy the Phone number ID (not the display number)

3. **VERIFY_TOKEN**:
   - You choose this token
   - Use it when configuring the webhook in WhatsApp

4. **WHATSAPP_WEBHOOK_SECRET**:
   - Go to your app → Webhooks
   - Set up a webhook secret for signature verification

## Step 4: Get Your Public URL

1. In Railway dashboard, go to "Settings"
2. Under "Domains", click "Generate Domain"
3. You'll get a URL like: `pion-whatsapp-bridge-production.up.railway.app`

## Step 5: Configure WhatsApp Business API

Use your Railway URL for webhook configuration:

- **Webhook URL**: `https://your-app.up.railway.app/whatsapp-call`
- **Verify Token**: `whatsapp_bridge_token`

## Testing Your Deployment

```bash
# Test health endpoint
curl https://your-app.up.railway.app/health

# Test status
curl https://your-app.up.railway.app/status

# Test webhook verification
curl 'https://your-app.up.railway.app/whatsapp-call?hub.mode=subscribe&hub.verify_token=whatsapp_bridge_token&hub.challenge=test123'
```

## Monitoring

- Railway provides logs in the dashboard
- Check "Observability" tab for metrics
- Set up alerts for downtime

## Cost

- Railway's free tier includes:
  - $5 free credit monthly
  - 500 hours of runtime
  - Perfect for testing and small projects
- This lightweight Go service will run well within free tier

## Advantages over ngrok

- ✅ Permanent URL (no changing URLs)
- ✅ Automatic HTTPS with valid certificate
- ✅ Built-in monitoring and logs
- ✅ Auto-deploy on git push
- ✅ Environment variable management
- ✅ No local machine needed