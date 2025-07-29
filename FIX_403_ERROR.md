# Fix WhatsApp 403 Forbidden Error

The 403 error is occurring because the webhook signature verification is failing. Here's how to fix it:

## The Issue

WhatsApp is sending webhook requests with a signature header (`X-Hub-Signature-256`), but the signature verification is failing because:

1. The `WHATSAPP_WEBHOOK_SECRET` environment variable is not set
2. OR it's set to the wrong value (it should be your App Secret, not the Verify Token)

## Solution

### Step 1: Get Your App Secret

1. Go to [Meta for Developers](https://developers.facebook.com)
2. Select your app
3. Navigate to: **Settings > Basic**
4. Find **App Secret** (click "Show" to reveal it)
5. Copy the entire App Secret value

### Step 2: Set the Environment Variable in Railway

1. Go to your Railway project dashboard
2. Click on your deployment
3. Go to **Variables** tab
4. Add or update:
   ```
   WHATSAPP_WEBHOOK_SECRET=your_app_secret_here
   ```

⚠️ **Important**: 
- This is NOT the Verify Token (which is `maitrise` in your case)
- This is the App Secret from Meta's dashboard
- It should be a long string of letters and numbers

### Step 3: Redeploy

Railway will automatically redeploy when you update the environment variable.

## Alternative: Disable Signature Verification (Not Recommended)

If you need to test quickly, you can temporarily disable signature verification by removing the `X-Hub-Signature-256` check, but this is NOT recommended for production as it's a security risk.

## Verify It's Working

After setting the correct App Secret:

1. Make a test call to your WhatsApp number
2. Check Railway logs - you should see:
   - No more 403 errors
   - Successful webhook processing
   - Call events being received

## Current Environment Variables

Make sure you have all these set correctly in Railway:

```
WHATSAPP_TOKEN=EAACyTlbIehsBANxVHrhTTPavC42DdW1yY9dDwz7aFqcPXesTj7ZB0QfZA4gZBpKaJJ22pV6qdjo26L4eT0C8mQO8AX3a7irhuZAwnW4c5HdY23zA2UOkmm54U3IgakOiGgHkGQBSaZC6wHiI5kQ4UQ9aiv2EGd3VWMLjVPircGZBMBkJiDkb3B
PHONE_NUMBER_ID=106382542433515
VERIFY_TOKEN=maitrise
WHATSAPP_WEBHOOK_SECRET=[YOUR_APP_SECRET_HERE]
```

The App Secret is different from all of these - it's found in your Meta app's Basic Settings.