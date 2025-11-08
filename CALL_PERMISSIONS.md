# WhatsApp Call Permissions System

## Overview

WhatsApp requires explicit permission from users before businesses can initiate calls to them. This system automatically tracks and manages these permissions.

## How It Works

### Automatic Permission Granting

When a user **calls your business first**, they automatically grant implicit permission for you to call them back. The system tracks this:

1. **Inbound Call Received** → `acceptIncomingCall()` is triggered
2. **Permission Auto-Granted** → `GrantCallPermission(phoneNumber)` is called
3. **Database Record Created** → Entry added to `whatsapp_call_permissions` table

### Outbound Call Permission Check

Before initiating any outbound call, the system checks if permission exists:

1. **Outbound Call Requested** → `/initiate-call` endpoint receives request
2. **Permission Check** → `CheckCallPermission(phoneNumber)` verifies permission
3. **Call Proceeds or Blocked**:
   - ✅ **Permission exists** → Call initiated via WhatsApp API
   - 🚫 **No permission** → Returns `403 Forbidden` with message: "No call permission from recipient. They must call you first to grant permission."

## Database Schema

### Table: `whatsapp_call_permissions`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `phone_number` | TEXT | Phone number (unique) |
| `first_inbound_call_at` | TIMESTAMP | When user first called us |
| `last_inbound_call_at` | TIMESTAMP | Most recent inbound call |
| `permission_granted` | BOOLEAN | Whether they have permission (default: true) |
| `total_inbound_calls` | INTEGER | Count of inbound calls |
| `created_at` | TIMESTAMP | Record creation time |
| `updated_at` | TIMESTAMP | Last update time |

**Indexes:**
- `idx_whatsapp_call_permissions_phone` - Fast lookup by phone number
- `idx_whatsapp_call_permissions_granted` - Query active permissions

## API Functions

### `GrantCallPermission(phoneNumber string) error`
**Location:** `supabase_client.go:577`

Called automatically when a user makes an inbound call. This function:
- Creates a new permission record if it doesn't exist
- Updates existing record with new call timestamp and increments counter
- Re-grants permission if it was previously revoked

**Example Log Output:**
```
✅ Granted call permission for 14085551234 (first inbound call)
✅ Updated call permission for 14085551234 (total calls: 3)
```

### `CheckCallPermission(phoneNumber string) (*WhatsAppCallPermission, error)`
**Location:** `supabase_client.go:670`

Checks if a phone number has active call permission. Returns:
- `*WhatsAppCallPermission` - Permission record if exists and granted
- `nil` - No permission found or permission revoked
- `error` - Database error

**Example Usage:**
```go
permission, err := CheckCallPermission("14085551234")
if err != nil {
    log.Printf("Error checking permission: %v", err)
} else if permission == nil {
    log.Printf("No permission to call this number")
} else {
    log.Printf("Permission granted on %s", permission.FirstInboundCallAt)
}
```

### `RevokeCallPermission(phoneNumber string) error`
**Location:** `supabase_client.go:716`

Revokes call permission for a phone number. Use when:
- User requests to opt-out of calls
- User reports spam/unwanted calls
- Compliance requirements

**Example Log Output:**
```
🚫 Revoked call permission for 14085551234
```

## Call Flow Examples

### Scenario 1: User Calls First (Happy Path)

```
1. User calls your WhatsApp Business number
   └─> Webhook: POST /whatsapp-call with event="connect", direction="USER_INITIATED"

2. acceptIncomingCall() is triggered
   └─> GrantCallPermission(callerNumber)
   └─> Database: INSERT whatsapp_call_permissions
   └─> Log: "✅ Granted call permission for 14085551234 (first inbound call)"

3. Later: You want to call the user back
   └─> POST /initiate-call {"to": "14085551234"}
   └─> CheckCallPermission("14085551234")
   └─> Permission found ✅
   └─> Log: "✅ Call permission verified for 14085551234 (granted on 2025-11-09T...)"
   └─> WhatsApp API: Initiate outbound call
   └─> Call proceeds successfully
```

### Scenario 2: Calling Without Permission (Blocked)

```
1. You try to call a user who has never called you
   └─> POST /initiate-call {"to": "14085551234"}

2. CheckCallPermission("14085551234")
   └─> No record found in database
   └─> Returns nil

3. Request blocked
   └─> Log: "🚫 No call permission for 14085551234 - user has not called us first"
   └─> Response: 403 Forbidden
   └─> Message: "No call permission from recipient. They must call you first to grant permission."
```

### Scenario 3: Repeated Calls (Permission Tracking)

```
1. User calls you (first time)
   └─> GrantCallPermission() creates record
   └─> total_inbound_calls: 1
   └─> first_inbound_call_at: 2025-11-09T10:00:00Z

2. User calls you again (second time)
   └─> GrantCallPermission() updates record
   └─> total_inbound_calls: 2
   └─> last_inbound_call_at: 2025-11-09T14:30:00Z

3. You can see call frequency in database
   └─> Useful for analytics and compliance
```

## Integration Points

### main.go:725-729
```go
// Grant call permission automatically - user calling us grants implicit permission for callbacks
if err := GrantCallPermission(callerNumber); err != nil {
    log.Printf("⚠️ Failed to grant call permission for %s: %v", callerNumber, err)
    // Continue anyway - permission tracking is not critical for call handling
}
```

### main.go:1580-1591
```go
// Check if we have permission to call this number
permission, err := CheckCallPermission(req.To)
if err != nil {
    log.Printf("⚠️ Error checking call permission for %s: %v", req.To, err)
    // Continue anyway - if Supabase is down, we don't want to block calls
} else if permission == nil {
    log.Printf("🚫 No call permission for %s - user has not called us first", req.To)
    http.Error(w, "No call permission from recipient. They must call you first to grant permission.", http.StatusForbidden)
    return
} else {
    log.Printf("✅ Call permission verified for %s (granted on %s)", req.To, permission.FirstInboundCallAt)
}
```

## Error Handling

### Graceful Degradation

If Supabase is unavailable during an **inbound call**:
- ✅ Call still proceeds normally
- ⚠️ Warning logged: "Failed to grant call permission"
- The call handling is not blocked

If Supabase is unavailable during an **outbound call check**:
- ⚠️ Warning logged: "Error checking call permission"
- ✅ Call proceeds anyway (fail-open approach)
- Reason: Don't block legitimate calls due to infrastructure issues

### Strict Mode (Optional)

To enforce strict permission checking even if Supabase is down, modify main.go:1582-1585:

```go
permission, err := CheckCallPermission(req.To)
if err != nil {
    // STRICT MODE: Block calls if permission check fails
    log.Printf("❌ Cannot verify call permission for %s: %v", req.To, err)
    http.Error(w, "Permission verification failed", http.StatusServiceUnavailable)
    return
}
```

## WhatsApp API Error (Before This Fix)

**Error Code:** `138006`
**Error Message:** "No approved call permission from the recipient"
**Cause:** Attempting to call a user who has never called you first

**Now Fixed By:**
1. ✅ Auto-granting permission on inbound calls
2. ✅ Checking permission before outbound calls
3. ✅ Returning clear error message to caller

## Compliance & Privacy

### GDPR/Privacy Considerations

- **Implicit Consent:** User calling you first constitutes implicit consent for callbacks
- **Right to Revoke:** Use `RevokeCallPermission()` to honor opt-out requests
- **Data Retention:** Consider adding TTL or expiry to permissions
- **Audit Trail:** `first_inbound_call_at` and `total_inbound_calls` provide audit history

### Recommended Policies

1. **Permission Expiry:** Revoke permissions after 90 days of no inbound calls
2. **Opt-Out Handling:** Provide clear mechanism for users to opt-out via text message
3. **Frequency Limits:** Track outbound call frequency to avoid spam
4. **Business Hours:** Only initiate calls during appropriate hours (9am-9pm local time)

## Future Enhancements

### Potential Improvements

1. **Permission Expiry:**
   ```sql
   ALTER TABLE whatsapp_call_permissions ADD COLUMN expires_at TIMESTAMP;
   ```

2. **Call Frequency Tracking:**
   ```sql
   CREATE TABLE call_history (
       id UUID PRIMARY KEY,
       phone_number TEXT,
       direction TEXT, -- 'inbound' or 'outbound'
       call_id TEXT,
       timestamp TIMESTAMP
   );
   ```

3. **Opt-Out Keywords:**
   - "STOP CALLING" → Auto-revoke permission
   - "CALL ME" → Explicit permission grant

4. **Analytics Dashboard:**
   - Total users with permission
   - Permission grant rate
   - Average time between grant and first outbound call

## Testing

### Manual Testing

1. **Test Permission Granting:**
   ```bash
   # Make an inbound call to your WhatsApp Business number
   # Check logs for: "✅ Granted call permission for..."
   ```

2. **Test Permission Check:**
   ```bash
   # Try calling a number that never called you
   curl -X POST http://localhost:3011/initiate-call \
     -H "Content-Type: application/json" \
     -d '{"to": "14085551234"}'

   # Expected: 403 Forbidden (if they never called you)
   ```

3. **Test Database:**
   ```sql
   -- Check permissions
   SELECT * FROM whatsapp_call_permissions ORDER BY created_at DESC;

   -- Check specific number
   SELECT * FROM whatsapp_call_permissions WHERE phone_number = '14085551234';
   ```

### Automated Testing

```go
func TestCallPermission(t *testing.T) {
    // Grant permission
    err := GrantCallPermission("14085551234")
    assert.NoError(t, err)

    // Verify permission exists
    permission, err := CheckCallPermission("14085551234")
    assert.NoError(t, err)
    assert.NotNil(t, permission)
    assert.Equal(t, "14085551234", permission.PhoneNumber)

    // Revoke permission
    err = RevokeCallPermission("14085551234")
    assert.NoError(t, err)

    // Verify permission revoked
    permission, err = CheckCallPermission("14085551234")
    assert.NoError(t, err)
    assert.Nil(t, permission)
}
```

## Troubleshooting

### "No call permission from recipient"

**Problem:** Getting 403 Forbidden when trying to call someone
**Solution:**
1. Ask the user to call your WhatsApp Business number first
2. Once they call, permission is auto-granted
3. Try outbound call again

### Permission not being granted on inbound calls

**Problem:** User called but permission not in database
**Check:**
1. Logs for "✅ Granted call permission" message
2. Supabase credentials (SUPABASE_URL, SUPABASE_ANON_KEY)
3. Table exists and RLS policies are correct
4. Network connectivity to Supabase

### Permission check failing during outbound calls

**Problem:** Outbound calls blocked even though user called first
**Check:**
1. Phone number format matches (with/without + prefix)
2. Database query logs in Supabase dashboard
3. RLS policies allow SELECT on whatsapp_call_permissions
4. Index on phone_number column exists

## References

- **WhatsApp Cloud API Calling:** https://developers.facebook.com/docs/whatsapp/cloud-api/calling
- **Permission Requirements:** See WhatsApp API error code 138006
- **Supabase RLS:** https://supabase.com/docs/guides/auth/row-level-security
