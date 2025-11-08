# Ziggy Reminders Setup Guide

This guide explains how to set up the reminders system that allows Ziggy to call users back at specified times.

## Timezone Configuration

**All times are in IST (Indian Standard Time, UTC+5:30)**

**Simple timezone handling - AI doesn't do conversions!**
- User says: "2 PM tomorrow"
- Ziggy formats as: `2025-11-09 14:00` (simple IST format)
- Backend converts to UTC: `2025-11-09T08:30:00Z` (for database)
- Cron checks against UTC but reminder is at correct IST time

The complexity is handled in Go, not by the AI. This ensures accuracy!

## Architecture

1. **User sets reminder** → Ziggy stores it in Supabase with `reminder_time`
2. **Supabase cron job** → Runs every minute, calls `/check-reminders` endpoint
3. **Bridge checks reminders** → Gets all due reminders from Supabase
4. **Bridge initiates calls** → Calls each user with a due reminder
5. **Ziggy announces reminder** → When user answers, Ziggy tells them what the reminder was about

## Setup Steps

### 1. Run the Reminders Migration

Go to your Supabase dashboard and run the migration:

1. Navigate to: https://supabase.com/dashboard/project/gglkagcmyfdyojtgrzyv
2. Click **SQL Editor**
3. Copy contents of `supabase_reminders_migration.sql`
4. Click **Run**

This creates the `ziggy_reminders` table with:
- `phone_number` - Who to call
- `reminder_text` - What to remind them about
- `reminder_time` - When to call
- `status` - pending, called, completed, cancelled

### 2. Set Up Supabase Cron Job

#### Option A: Using pg_cron (Recommended)

Add this to your Supabase SQL Editor:

```sql
-- Enable pg_cron extension
CREATE EXTENSION IF NOT EXISTS pg_cron;

-- Create a function to call the check-reminders endpoint
CREATE OR REPLACE FUNCTION check_reminders_http()
RETURNS void AS $$
DECLARE
  response json;
BEGIN
  -- Make HTTP POST request to your bridge
  SELECT content::json INTO response
  FROM http_post(
    'https://whatsapp-bridge.tslfiles.org/check-reminders',
    '{}',
    'application/json'
  );

  RAISE NOTICE 'Reminders check response: %', response;
END;
$$ LANGUAGE plpgsql;

-- Schedule the job to run every minute
SELECT cron.schedule(
  'check-ziggy-reminders',
  '* * * * *',  -- Every minute
  $$SELECT check_reminders_http()$$
);
```

#### Option B: Using Supabase Edge Functions

Create an edge function that runs on a schedule:

```bash
# In your terminal
npx supabase functions new check-reminders-cron

# Edit the function file
cat > supabase/functions/check-reminders-cron/index.ts <<'EOF'
import { serve } from "https://deno.land/std@0.168.0/http/server.ts"

serve(async (_req) => {
  try {
    const response = await fetch('https://whatsapp-bridge.tslfiles.org/check-reminders', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
    })

    const data = await response.json()
    return new Response(
      JSON.stringify(data),
      { headers: { "Content-Type": "application/json" } },
    )
  } catch (error) {
    return new Response(
      JSON.stringify({ error: error.message }),
      { status: 500, headers: { "Content-Type": "application/json" } },
    )
  }
})
EOF

# Deploy the function
npx supabase functions deploy check-reminders-cron

# Set up cron schedule (every minute)
# Go to Supabase Dashboard → Edge Functions → check-reminders-cron → Settings
# Set cron: "* * * * *"
```

#### Option C: Using External Cron Service

Use a service like:
- **Cron-job.org** (free)
- **EasyCron** (free tier)
- **GitHub Actions** (free for public repos)

Example GitHub Actions workflow:

```yaml
# .github/workflows/check-reminders.yml
name: Check Reminders
on:
  schedule:
    - cron: '* * * * *'  # Every minute
  workflow_dispatch:  # Allow manual trigger

jobs:
  check-reminders:
    runs-on: ubuntu-latest
    steps:
      - name: Call check-reminders endpoint
        run: |
          curl -X POST https://whatsapp-bridge.tslfiles.org/check-reminders
```

### 3. Test the System

#### Create a test reminder (1 minute from now in IST):

```bash
# For IST timezone (UTC+5:30), calculate time 1 minute from now
# Example IST time: 2025-11-09T14:30:00+05:30

# Create test reminder via Supabase
curl -X POST 'https://gglkagcmyfdyojtgrzyv.supabase.co/rest/v1/ziggy_reminders' \
  -H "apikey: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6ImdnbGthZ2NteWZkeW9qdGdyenl2Iiwicm9sZSI6ImFub24iLCJpYXQiOjE3NDY0NzMxODMsImV4cCI6MjA2MjA0OTE4M30.qLpjhwNsK8sF2OJ8O-N6Jkwy8wDTB8FDGQGStO_LhB0" \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6ImdnbGthZ2NteWZkeW9qdGdyenl2Iiwicm9sZSI6ImFub24iLCJpYXQiOjE3NDY0NzMxODMsImV4cCI6MjA2MjA0OTE4M30.qLpjhwNsK8sF2OJ8O-N6Jkwy8wDTB8FDGQGStO_LhB0" \
  -H "Content-Type: application/json" \
  -H "Prefer: return=representation" \
  -d '{
    "phone_number": "919885842349",
    "reminder_text": "Test reminder - call the dentist",
    "reminder_time": "2025-11-09T14:31:00+05:30",
    "status": "pending"
  }'

# Note: The time will be automatically converted to UTC in the database
# IST: 2025-11-09T14:31:00+05:30 → UTC: 2025-11-09T09:01:00Z
```

#### Manually trigger check:

```bash
curl -X POST https://whatsapp-bridge.tslfiles.org/check-reminders
```

Expected response:
```json
{
  "status": "success",
  "message": "Processed 1 reminders",
  "called": 1,
  "failed": 0
}
```

## How It Works

### When User Sets a Reminder

**User says:** "Ziggy, remind me to call the dentist tomorrow at 2 PM"

**Ziggy processes:**
1. Parses intent: User wants a reminder
2. Calls `add_reminder` function
3. Stores in Supabase:
   ```json
   {
     "phone_number": "919885842349",
     "reminder_text": "Call the dentist",
     "reminder_time": "2025-11-09T14:00:00Z",
     "status": "pending"
   }
   ```
4. Confirms: "Reminder set for tomorrow at 2 PM. I'll call you back then."

### When Reminder is Due

**Cron job runs** (every minute):
1. Calls `/check-reminders` endpoint
2. Bridge queries Supabase for `status=pending AND reminder_time <= NOW()`
3. For each due reminder:
   - Initiates outbound call to user
   - Updates status to `called`
4. Returns summary

**User answers call:**
1. Ziggy greets: "Hello! I'm calling to remind you about..."
2. Announces reminder text
3. Asks if task is completed
4. Can mark reminder as completed

## Table Structure

```sql
ziggy_reminders:
  - id (UUID, primary key)
  - phone_number (TEXT, indexed)
  - reminder_text (TEXT)
  - reminder_time (TIMESTAMPTZ, indexed)
  - status (TEXT: pending, called, completed, cancelled)
  - created_at (TIMESTAMPTZ)
  - updated_at (TIMESTAMPTZ)
  - call_id (TEXT, nullable)
  - attempts (INTEGER, default 0)
```

## Monitoring

### Check Due Reminders

```bash
curl https://whatsapp-bridge.tslfiles.org/check-reminders
```

### Query Reminders in Supabase

```sql
-- Pending reminders
SELECT * FROM ziggy_reminders
WHERE status = 'pending'
ORDER BY reminder_time ASC;

-- Due reminders
SELECT * FROM ziggy_reminders
WHERE status = 'pending'
AND reminder_time <= NOW()
ORDER BY reminder_time ASC;

-- Recent calls
SELECT * FROM ziggy_reminders
WHERE status = 'called'
ORDER BY updated_at DESC
LIMIT 10;
```

## Configuration

### Cron Frequency

Default: Every minute (`* * * * *`)

For testing, you can run less frequently:
- Every 5 minutes: `*/5 * * * *`
- Every 15 minutes: `*/15 * * * *`
- Every hour: `0 * * * *`

### Retry Logic

To add retry logic for failed calls, modify the cron job:

```sql
-- Function with retry logic
CREATE OR REPLACE FUNCTION check_reminders_with_retry()
RETURNS void AS $$
DECLARE
  reminder RECORD;
BEGIN
  FOR reminder IN
    SELECT * FROM ziggy_reminders
    WHERE status = 'pending'
    AND reminder_time <= NOW()
    AND attempts < 3  -- Max 3 attempts
    ORDER BY reminder_time ASC
  LOOP
    -- Update attempts
    UPDATE ziggy_reminders
    SET attempts = attempts + 1
    WHERE id = reminder.id;

    -- Call the HTTP endpoint
    PERFORM check_reminders_http();
  END LOOP;
END;
$$ LANGUAGE plpgsql;
```

## Troubleshooting

### Cron job not running

1. Check if pg_cron is enabled: `SELECT * FROM cron.job;`
2. Check cron logs: `SELECT * FROM cron.job_run_details ORDER BY start_time DESC LIMIT 10;`
3. Verify HTTP extension: `CREATE EXTENSION IF NOT EXISTS http;`

### Calls not being initiated

1. Check bridge logs for errors
2. Verify `/check-reminders` endpoint is accessible
3. Test manually: `curl -X POST https://whatsapp-bridge.tslfiles.org/check-reminders`
4. Check Supabase for due reminders: `SELECT * FROM ziggy_reminders WHERE status='pending' AND reminder_time <= NOW();`

### Reminders stuck in 'called' status

Update them to completed after successful announcement:

```sql
UPDATE ziggy_reminders
SET status = 'completed'
WHERE status = 'called'
AND updated_at < NOW() - INTERVAL '1 hour';
```

## Future Enhancements

1. **Snooze functionality** - Allow users to snooze reminders
2. **Recurring reminders** - Daily/weekly reminders
3. **Smart scheduling** - Respect quiet hours
4. **Multiple attempts** - Retry if user doesn't answer
5. **SMS fallback** - Send SMS if call fails
6. **Confirmation** - Ask user to confirm reminder was useful

## Security

- Reminders are user-specific (filtered by phone_number)
- No sensitive data stored in reminder_text
- Cron endpoint is public but safe (only triggers checks)
- Consider adding API key for production: `Authorization: Bearer YOUR_SECRET_KEY`

## IST Timezone Quick Reference

### Converting IST to UTC

IST is UTC+5:30, so:
- IST 2:00 PM (14:00) = UTC 8:30 AM (08:30)
- IST 10:00 AM = UTC 4:30 AM
- IST 6:00 PM (18:00) = UTC 12:30 PM

### Format Examples

**User-friendly (what Ziggy hears):**
- "2 PM tomorrow"
- "10:30 AM on November 9th"
- "6 PM today"

**Simple IST format (what Ziggy sends):**
- `2025-11-09 14:00`
- `2025-11-09 10:30`
- `2025-11-08 18:00`

**UTC in database (what backend converts and stores):**
- `2025-11-09T08:30:00Z`
- `2025-11-09T05:00:00Z`
- `2025-11-08T12:30:00Z`

The AI just needs to format as `YYYY-MM-DD HH:MM` in IST. The Go backend handles all timezone conversion!

### Testing Time Conversions

```bash
# Current time in IST
TZ='Asia/Kolkata' date

# Current time in UTC
date -u

# Convert specific IST time to UTC
date -u -j -f "%Y-%m-%dT%H:%M:%S" "2025-11-09T14:00:00" +"%Y-%m-%dT%H:%M:%SZ"
```
