# Express WhatsApp Call Permission System

## Overview

The Express Permission System allows you to **proactively request call permissions** from users via WhatsApp messages, without waiting for them to call you first.

## Two Ways to Get Permission

### 1. **Automatic (Inbound Call)**
   - User calls your WhatsApp Business number
   - Permission automatically granted ✅
   - **No expiry** - permanent until revoked

### 2. **Express (Permission Request)**
   - You send a permission request message
   - User clicks "Yes, you can call me" ✅
   - Permission granted for **72 hours**
   - After 72 hours, permission expires

## How It Works

```
┌─────────────────────────────────────────────────┐
│  EXPRESS PERMISSION FLOW                        │
└─────────────────────────────────────────────────┘

1. POST /request-call-permission {"to": "14085551234"}
   │
   ├─> Rate limit check (1/24h, 2/7days)
   │
   ├─> Send WhatsApp message with buttons:
   │   "📞 Would you like to receive voice calls from us?"
   │   [✅ Yes, you can call me]  [❌ No, thanks]
   │
   └─> Database: Record permission request

2. User clicks "✅ Yes, you can call me"
   │
   ├─> Webhook receives button response
   │
   ├─> Database: Update permission
   │   - permission_granted = true
   │   - permission_expires_at = now + 72 hours
   │   - permission_source = "express_request"
   │
   └─> Reply: "✅ Thank you! You've granted permission..."

3. You initiate call within 72 hours
   │
   ├─> POST /initiate-call {"to": "14085551234"}
   │
   ├─> Check permission (validates expiry)
   │
   └─> Call proceeds ✅
```

## API Endpoints

### Request Permission

```bash
POST /request-call-permission
Content-Type: application/json

{
  "to": "14085551234"  # Phone number without +
}
```

**Success Response (200):**
```json
{
  "status": "sent",
  "message": "Call permission request sent successfully",
  "to": "14085551234"
}
```

**Error Responses:**
- `400 Bad Request` - Missing or invalid phone number
- `429 Too Many Requests` - Rate limited (see limits below)
- `500 Internal Server Error` - Failed to send message

### Rate Limits

**WhatsApp enforced limits:**
- 1 request per 24 hours per number
- 2 requests per 7 days per number

**What happens when rate limited:**
```json
HTTP 429 Too Many Requests
"Rate limited. You can only send 1 request per 24 hours, 2 per 7 days."
```

## Database Schema Updates

Add these columns to `whatsapp_call_permissions` table:

```sql
-- New columns for express permission system
ALTER TABLE whatsapp_call_permissions
ADD COLUMN permission_requested_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN permission_approved_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN permission_expires_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN permission_request_count INTEGER DEFAULT 0,
ADD COLUMN last_permission_request_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN permission_source TEXT; -- 'inbound_call', 'express_request', 'manual'
```

**Updated Schema:**
```
whatsapp_call_permissions
├── id (UUID)
├── phone_number (TEXT, unique)
├── permission_granted (BOOLEAN)
├── first_inbound_call_at (TIMESTAMP)
├── last_inbound_call_at (TIMESTAMP)
├── total_inbound_calls (INTEGER)
├── permission_requested_at (TIMESTAMP)        ← NEW
├── permission_approved_at (TIMESTAMP)         ← NEW
├── permission_expires_at (TIMESTAMP)          ← NEW (72-hour window)
├── permission_request_count (INTEGER)         ← NEW (rate limiting)
├── last_permission_request_at (TIMESTAMP)     ← NEW (rate limiting)
├── permission_source (TEXT)                   ← NEW ('inbound_call' or 'express_request')
├── created_at (TIMESTAMP)
└── updated_at (TIMESTAMP)
```

## Permission Sources

| Source | Granted By | Expiry | Use Case |
|--------|-----------|--------|----------|
| `inbound_call` | User calls you first | ❌ No expiry | Long-term relationship |
| `express_request` | User approves your request | ✅ 72 hours | Time-sensitive calls |
| `manual` | Admin/API | Custom | Special cases |

## Code Functions

### 1. **SendCallPermissionRequest(phoneNumber)**
**Location:** `supabase_client.go:945`

Sends an interactive WhatsApp message with permission buttons.

```go
err := SendCallPermissionRequest("14085551234")
if err != nil {
    if err.Error() == "rate limited" {
        // Handle rate limit
    }
}
```

**What it does:**
1. Checks rate limits
2. Records request in database
3. Sends WhatsApp message with buttons
4. Returns error if rate limited or failed

### 2. **ApproveCallPermission(phoneNumber, source)**
**Location:** `supabase_client.go:893`

Approves permission and sets 72-hour expiry.

```go
err := ApproveCallPermission("14085551234", "express_request")
// Sets permission_expires_at = now + 72 hours
```

**What it does:**
1. Sets `permission_granted = true`
2. Records approval timestamp
3. Sets expiry to 72 hours from now
4. Updates permission source

### 3. **CheckCallPermission(phoneNumber)**
**Location:** `supabase_client.go:676`

Checks if permission exists and validates expiry.

```go
permission, err := CheckCallPermission("14085551234")
if permission == nil {
    // No permission or expired
} else {
    // Permission valid
    log.Printf("Granted via: %s", permission.PermissionSource)
    if permission.PermissionExpiresAt != "" {
        log.Printf("Expires at: %s", permission.PermissionExpiresAt)
    }
}
```

**What it does:**
1. Queries database for granted permissions
2. Checks if `permission_expires_at` has passed
3. Auto-revokes expired permissions
4. Returns nil if no permission or expired

## User Experience

### Permission Request Message

**User receives:**
```
📞 Would you like to receive voice calls from us?
This will allow us to contact you by phone when needed.

[✅ Yes, you can call me]  [❌ No, thanks]
```

### When User Clicks "Yes"

**User receives:**
```
✅ Thank you! You've granted permission for us to call you.
We can now contact you by phone when needed.
This permission is valid for 72 hours.
```

### When User Clicks "No"

**User receives:**
```
👍 No problem! We won't call you.
You can change your mind anytime by typing 'allow calls'.
```

## Button Handlers

The interactive button responses are handled in `main.go:617`:

```go
case "approve_call_permission":
    // User clicked "✅ Yes, you can call me"
    ApproveCallPermission(sender, "express_request")
    handler.ReplyText("✅ Thank you! You've granted permission...")

case "deny_call_permission":
    // User clicked "❌ No, thanks"
    handler.ReplyText("👍 No problem! We won't call you...")
```

## Testing

### Test Permission Request

```bash
curl -X POST http://localhost:3011/request-call-permission \
  -H "Content-Type: application/json" \
  -d '{"to": "14085551234"}'
```

**Expected:**
1. User receives WhatsApp message with buttons
2. Database records permission request
3. API returns success response

### Test Permission Approval

1. Click "✅ Yes, you can call me" in WhatsApp
2. Check database:
   ```sql
   SELECT phone_number, permission_granted, permission_expires_at, permission_source
   FROM whatsapp_call_permissions
   WHERE phone_number = '14085551234';
   ```
3. Verify:
   - `permission_granted = true`
   - `permission_expires_at = now + 72 hours`
   - `permission_source = 'express_request'`

### Test Call After Approval

```bash
curl -X POST http://localhost:3011/initiate-call \
  -H "Content-Type: application/json" \
  -d '{"to": "14085551234"}'
```

**Expected:**
- Call proceeds successfully ✅
- Logs show: "✅ Call permission verified for 14085551234 (granted on...)"

### Test Expiry

1. Grant permission via express request
2. Manually update database to expired time:
   ```sql
   UPDATE whatsapp_call_permissions
   SET permission_expires_at = NOW() - INTERVAL '1 hour'
   WHERE phone_number = '14085551234';
   ```
3. Try to call:
   ```bash
   curl -X POST http://localhost:3011/initiate-call \
     -H "Content-Type: application/json" \
     -d '{"to": "14085551234"}'
   ```
4. **Expected:** 403 Forbidden - Permission expired

### Test Rate Limiting

1. Send first request:
   ```bash
   curl -X POST http://localhost:3011/request-call-permission \
     -H "Content-Type: application/json" \
     -d '{"to": "14085551234"}'
   # Success
   ```

2. Send second request immediately:
   ```bash
   curl -X POST http://localhost:3011/request-call-permission \
     -H "Content-Type: application/json" \
     -d '{"to": "14085551234"}'
   # 429 Too Many Requests
   ```

## Comparison: Automatic vs Express

| Feature | Automatic (Inbound) | Express (Request) |
|---------|---------------------|-------------------|
| **Trigger** | User calls you | You request permission |
| **User Action** | Make a call | Click button |
| **Expiry** | Never | 72 hours |
| **Rate Limit** | None | 1/day, 2/week |
| **Use Case** | Ongoing relationship | One-time urgent call |
| **Database Source** | `inbound_call` | `express_request` |

## Best Practices

### When to Use Express Permissions

✅ **Good Use Cases:**
- Appointment reminders (medical, salon, etc.)
- Delivery notifications (order arriving soon)
- Time-sensitive alerts (password reset, fraud)
- Customer support callbacks (they requested a call)
- Event reminders (webinar starting soon)

❌ **Avoid Using For:**
- Marketing calls (use inbound permission)
- General outreach (wait for user to call first)
- Repeated calls (will hit rate limit)
- Non-urgent communications (use text instead)

### Permission Renewal Strategy

**Option 1: Request Renewal**
```
If permission expires and you need to call again:
1. Send new permission request
2. User approves → New 72-hour window
```

**Option 2: Encourage Inbound Call**
```
Send message: "We need to discuss your order.
Please call us at [number] or click below to allow us to call you."
[Call Me] button
```

### Handling Denials

When user clicks "❌ No, thanks":
1. Respect their choice - don't call
2. Don't send another request for at least 7 days
3. Consider alternative communication (text, email)
4. Track denials to avoid spam reputation

## Analytics Queries

### Permission Request Success Rate

```sql
SELECT
  COUNT(*) as total_requests,
  COUNT(CASE WHEN permission_granted = true THEN 1 END) as approved,
  ROUND(100.0 * COUNT(CASE WHEN permission_granted = true THEN 1 END) / COUNT(*), 2) as approval_rate
FROM whatsapp_call_permissions
WHERE permission_source = 'express_request';
```

### Expired Permissions

```sql
SELECT phone_number, permission_approved_at, permission_expires_at
FROM whatsapp_call_permissions
WHERE permission_expires_at < NOW()
  AND permission_granted = true;
```

### Rate Limited Users

```sql
SELECT phone_number, permission_request_count, last_permission_request_at
FROM whatsapp_call_permissions
WHERE permission_request_count >= 2
  AND last_permission_request_at > NOW() - INTERVAL '7 days';
```

## Troubleshooting

### "Rate limited" Error

**Problem:** Can't send permission request
**Cause:** Sent 1 request in past 24 hours OR 2 requests in past 7 days
**Solution:**
- Wait 24 hours for next request
- Check database: `last_permission_request_at` and `permission_request_count`
- Consider asking user to call you instead

### Permission Expires Too Fast

**Problem:** 72 hours not enough time
**Cause:** Express permissions are temporary by design (WhatsApp requirement)
**Solutions:**
1. Ask user to call you first (grants permanent permission)
2. Send follow-up message with "Call Me" button
3. Request permission again when needed (respecting rate limits)

### User Approved But Call Fails

**Check:**
1. Permission not expired:
   ```sql
   SELECT permission_expires_at FROM whatsapp_call_permissions
   WHERE phone_number = '...' AND permission_expires_at > NOW();
   ```
2. Logs show permission check passed
3. WhatsApp API error is not 138006

### Permission Request Not Received

**Check:**
1. Phone number format (without +)
2. Active conversation window exists
3. WhatsApp credentials configured
4. Logs show message sent successfully
5. User has WhatsApp installed and active

## Integration Examples

### Appointment Reminder System

```go
// 24 hours before appointment
func SendAppointmentReminder(appointmentID string) {
    apt := GetAppointment(appointmentID)

    // Send text reminder
    client.SendText(apt.PhoneNumber,
        fmt.Sprintf("Reminder: Your appointment is tomorrow at %s", apt.Time))

    // Request call permission for confirmation call
    if err := SendCallPermissionRequest(apt.PhoneNumber); err == nil {
        log.Printf("Sent call permission request for appointment %s", appointmentID)
    }
}

// When user approves, schedule confirmation call
func OnPermissionApproved(phoneNumber string) {
    // Find appointment for this phone number
    apt := GetAppointmentByPhone(phoneNumber)

    // Schedule call for later
    ScheduleCall(phoneNumber, apt.Time.Add(-1 * time.Hour)) // Call 1 hour before
}
```

### Customer Support Callback

```go
// When customer submits "call me" request on website
func HandleCallbackRequest(phoneNumber, issue string) {
    // Send WhatsApp message
    client.SendText(phoneNumber,
        fmt.Sprintf("We received your callback request about: %s", issue))

    // Request permission
    if err := SendCallPermissionRequest(phoneNumber); err != nil {
        log.Printf("Failed to request permission: %v", err)
        // Fallback: Ask them to call us
        client.SendText(phoneNumber,
            "Please call us at 1-800-SUPPORT or we can call you if you approve below:")
        return
    }

    // When approved, initiate callback within 72 hours
    log.Printf("Permission request sent, will call within 72 hours")
}
```

## Summary

The Express Permission System gives you **proactive control** over call permissions:

✅ **Don't wait** for users to call you first
✅ **Request permission** when you need to make a call
✅ **Respect rate limits** (1/day, 2/week)
✅ **72-hour window** to make the call
✅ **Auto-expiry** prevents stale permissions
✅ **User-friendly** interactive button approval

**Next Steps:**
1. Update your database schema with new columns
2. Test permission request flow
3. Integrate into your application workflows
4. Monitor approval rates and optimize messaging
