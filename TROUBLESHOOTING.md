# Troubleshooting WhatsApp Call Issues

## Issue: Only Receiving "terminate" Events

If you're only getting terminate events and not connect events with SDP offers:

### 1. Check Webhook Configuration
- Ensure your webhook is subscribed to the "calls" field in Meta Business Dashboard
- Verify the webhook URL is correct: `https://your-domain/whatsapp-call`
- Check that webhook verification passed

### 2. Check Call Settings
- Ensure voice calling is enabled for your WhatsApp Business number
- Check if there are any restrictions on your phone number
- Verify your business is approved for calling features

### 3. Timing Issues
- The call might be terminating before the connect event is sent
- Check if the caller is ending the call too quickly
- Try keeping the call ringing for at least 5-10 seconds

### 4. Multiple Devices
- If the WhatsApp Business number is active on multiple devices, calls might be answered elsewhere
- Try logging out from other devices

## Issue: "Failed to set remote description: EOF"

This error indicates the SDP parsing is failing. Possible causes:

### 1. Codec Registration
We've fixed this by:
- Registering Opus with exact parameters (PT:111)
- Registering telephone-event (PT:126)
- Registering required RTP extensions

### 2. SDP Format Issues
Check the logs for:
- SDP length (should be >500 bytes)
- First/last 50 characters
- Whether it starts with "v=0"

### 3. Line Ending Issues
The SDP might have:
- Mixed line endings (\r\n vs \n)
- Escaped line endings (\\r\\n)
- Truncated content

## Debugging Steps

1. **Check Status Endpoint**
   ```bash
   curl https://your-domain/status
   ```
   Verify:
   - All environment variables are set
   - Webhook is ready
   - Codecs are registered

2. **Monitor Logs**
   Look for:
   - "📞 Call event:" logs
   - "🔍 SDP Offer (cleaned):" logs
   - Error details with "📋"

3. **Test Webhook Manually**
   Use the test script to simulate a connect event:
   ```bash
   ./test-webhook.sh https://your-domain/whatsapp-call
   ```

4. **Verify Call Flow**
   The expected sequence is:
   - Receive "connect" event with SDP offer
   - Parse and set remote description
   - Create answer with our SDP
   - Send pre_accept
   - Wait for ICE connection
   - Send accept
   - Audio flows

## Common Solutions

1. **Restart the service** after making changes
2. **Re-verify the webhook** in Meta dashboard
3. **Check Railway logs** for any startup errors
4. **Ensure ENABLE_ECHO=true** for testing audio

## Still Having Issues?

1. Check the full webhook payload in logs
2. Look for any unhandled event types
3. Verify the SDP structure matches WhatsApp's documentation
4. Check if calls work with a different WhatsApp account