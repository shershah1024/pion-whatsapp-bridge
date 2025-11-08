# Recurring Reminders Guide

This guide explains how to use the recurring reminders feature in Ziggy.

## Features

- **One-time reminders**: Set a reminder for a specific date/time (default)
- **Recurring reminders**: Set reminders that repeat daily, weekly, monthly, or yearly
- **Automatic timezone detection**: Works based on user's phone number
- **Easy cancellation**: Cancel any reminder via voice command

## Setup Steps

### 1. Run Database Migrations

Run these SQL files in your Supabase SQL Editor **in this order**:

1. **`supabase_reminders_migration.sql`** (if not already done)
   - Creates `ziggy_reminders` table

2. **`supabase_recurring_reminders_migration.sql`** (NEW)
   - Adds `recurrence_pattern` column
   - Creates views for monitoring

3. **`supabase_cron_setup.sql`** (updated version)
   - Enables pg_cron and http extensions
   - Creates functions to schedule one-time and recurring cron jobs
   - Sets up automatic triggers

## How It Works

### One-Time Reminders (Default)

**User:** "Remind me to call the dentist tomorrow at 2 PM"

**Ziggy:**
- Creates reminder with `recurrence_pattern = 'once'`
- Schedules one-time cron job
- **After calling:** Cron job automatically unschedules itself ✅

**Cron pattern:** `30 14 09 11 *` (Nov 9 at 2:30 PM, one-time)

### Recurring Reminders

**User:** "Remind me to take my medication daily at 9 AM"

**Ziggy:**
- Creates reminder with `recurrence_pattern = 'daily'`
- Schedules recurring cron job
- **After calling:** Cron job stays active and repeats ✅

**Cron pattern:** `0 9 * * *` (Every day at 9 AM)

## Recurrence Patterns

| Pattern | Example | Cron Schedule | Description |
|---------|---------|---------------|-------------|
| `once` (default) | "Remind me tomorrow at 2 PM" | `0 14 09 11 *` | One-time only |
| `daily` | "Remind me daily at 9 AM" | `0 9 * * *` | Every day at same time |
| `weekly` | "Remind me weekly on Monday at 10 AM" | `0 10 * * 1` | Every week, same day/time |
| `monthly` | "Remind me monthly on the 1st at 2 PM" | `0 14 1 * *` | Every month, same day/time |
| `yearly` | "Remind me yearly on Nov 9 at 3 PM" | `0 15 09 11 *` | Every year, same date/time |

## Usage Examples

### Setting Reminders

```
User: "Set a daily reminder to take my vitamins at 8 AM"
Ziggy: "Daily reminder set for 8:00 AM. I'll call you daily."

User: "Remind me weekly on Fridays at 5 PM to review my week"
Ziggy: "Weekly reminder set for Friday at 5:00 PM. I'll call you weekly."

User: "Remind me on the 1st of every month at noon to pay rent"
Ziggy: "Monthly reminder set for the 1st at 12:00 PM. I'll call you monthly."
```

### Listing Reminders

```
User: "What reminders do I have?"
Ziggy: "You have 3 pending reminders:
  1. Take vitamins (daily at 8 AM)
  2. Review week (weekly, Friday 5 PM)
  3. Pay rent (monthly, 1st at noon)"
```

### Cancelling Reminders

```
User: "Cancel my daily vitamin reminder"
Ziggy: [Lists reminders] "Which one would you like to cancel?"
User: "The first one"
Ziggy: "Reminder cancelled successfully."

# OR

User: "Show me all my reminders"
Ziggy: [Shows list with IDs]
User: "Cancel the reminder about vitamins"
Ziggy: "Reminder cancelled successfully."
```

## New API Tools

Ziggy now has 3 reminder tools:

### 1. `add_reminder`
```javascript
{
  reminder_text: "Take medication",
  reminder_time: "2025-11-09 14:00",
  recurrence: "daily"  // Optional: once, daily, weekly, monthly, yearly
}
```

### 2. `list_reminders`
```javascript
{
  status: "pending"  // Optional: pending, called, completed, cancelled
}
```

### 3. `cancel_reminder`
```javascript
{
  reminder_id: "uuid-of-reminder"  // From list_reminders
}
```

## Database Schema

```sql
ziggy_reminders:
  - id (UUID)
  - phone_number (TEXT)
  - reminder_text (TEXT)
  - reminder_time (TIMESTAMPTZ)
  - recurrence_pattern (TEXT) -- NEW: once, daily, weekly, monthly, yearly
  - status (TEXT)              -- pending, called, completed, cancelled
  - created_at (TIMESTAMPTZ)
  - updated_at (TIMESTAMPTZ)
  - call_id (TEXT)
  - attempts (INTEGER)
```

## Cancellation Flow

When a reminder is cancelled (status → 'cancelled'):

1. **Update database:** `status = 'cancelled'`
2. **Trigger fires:** `trigger_cancel_reminder()` detects status change
3. **Unschedule cron:** Calls `cancel_reminder_call(reminder_id)`
4. **Cron removed:** `cron.unschedule('reminder_' || id)` removes the job

This works for both one-time AND recurring reminders!

## Monitoring

### View All Scheduled Cron Jobs
```sql
SELECT * FROM scheduled_reminder_jobs;
```

### View Reminder Types
```sql
SELECT * FROM reminder_types;
```

### Check Cron Status
```sql
-- See all cron jobs
SELECT * FROM cron.job WHERE jobname LIKE 'reminder_%';

-- See cron execution history
SELECT * FROM cron.job_run_details
WHERE jobname LIKE 'reminder_%'
ORDER BY start_time DESC
LIMIT 10;
```

### Query Reminders
```sql
-- All pending reminders
SELECT id, reminder_text, reminder_time, recurrence_pattern, status
FROM ziggy_reminders
WHERE status = 'pending'
ORDER BY reminder_time ASC;

-- All recurring reminders
SELECT id, reminder_text, recurrence_pattern, reminder_time
FROM ziggy_reminders
WHERE recurrence_pattern IS NOT NULL
  AND recurrence_pattern != 'once'
  AND status = 'pending';

-- Reminders for specific user
SELECT * FROM ziggy_reminders
WHERE phone_number = '+919885842349'
  AND status = 'pending'
ORDER BY reminder_time ASC;
```

## Testing

### Test One-Time Reminder
```bash
# Create reminder 2 minutes from now
curl -X POST 'https://gglkagcmyfdyojtgrzyv.supabase.co/rest/v1/ziggy_reminders' \
  -H "apikey: YOUR_KEY" \
  -H "Authorization: Bearer YOUR_KEY" \
  -H "Content-Type: application/json" \
  -H "Prefer: return=representation" \
  -d '{
    "phone_number": "919885842349",
    "reminder_text": "Test one-time reminder",
    "reminder_time": "2025-11-08T18:32:00Z",
    "recurrence_pattern": "once",
    "status": "pending"
  }'
```

### Test Daily Recurring Reminder
```bash
# Create daily reminder
curl -X POST 'https://gglkagcmyfdyojtgrzyv.supabase.co/rest/v1/ziggy_reminders' \
  -H "apikey: YOUR_KEY" \
  -H "Authorization: Bearer YOUR_KEY" \
  -H "Content-Type: application/json" \
  -H "Prefer: return=representation" \
  -d '{
    "phone_number": "919885842349",
    "reminder_text": "Daily medication reminder",
    "reminder_time": "2025-11-09T03:30:00Z",
    "recurrence_pattern": "daily",
    "status": "pending"
  }'
```

### Test Cancellation
```sql
-- Update reminder status to trigger cancellation
UPDATE ziggy_reminders
SET status = 'cancelled'
WHERE id = 'your-reminder-uuid';

-- Check that cron job was removed
SELECT * FROM cron.job
WHERE jobname = 'reminder_your-reminder-uuid';
-- Should return 0 rows
```

## Troubleshooting

### Cron job not executing
1. Check cron is enabled: `SELECT * FROM cron.job WHERE jobname LIKE 'reminder_%';`
2. Check execution history: `SELECT * FROM cron.job_run_details ORDER BY start_time DESC LIMIT 10;`
3. Verify http extension: `CREATE EXTENSION IF NOT EXISTS http;`
4. Check timezone conversion is correct

### Recurring reminder not repeating
1. Verify `recurrence_pattern` is not 'once' or NULL
2. Check cron schedule: `SELECT schedule FROM cron.job WHERE jobname = 'reminder_xxx';`
3. Ensure status is 'pending'

### Cancellation not working
1. Check trigger is active: `SELECT * FROM pg_trigger WHERE tgname = 'on_reminder_updated';`
2. Verify status changed from 'pending' to something else
3. Check cron.job table to confirm job was removed

## Security Considerations

- Reminders are user-specific (filtered by phone_number)
- Cancellation requires knowing the reminder_id
- Database triggers ensure cron jobs are cleaned up
- RLS policies restrict access to user's own reminders

## Future Enhancements

1. **Custom recurrence patterns**: "Every 2 days", "First Monday of month"
2. **Quiet hours**: Don't call during certain times
3. **Snooze functionality**: "Remind me again in 10 minutes"
4. **Reminder confirmation**: Ask if task was completed
5. **Smart rescheduling**: If user doesn't answer, reschedule
6. **Multiple reminders**: Set multiple reminders for same task

## Cost Implications

- **One-time reminders**: Cron job exists only until execution, then self-deletes
- **Recurring reminders**: Cron job stays in database until cancelled
- **Storage**: Minimal (one row in cron.job per active reminder)
- **Execution**: One HTTP request per reminder execution
- **Cleanup**: Cancelled reminders automatically remove their cron jobs

Expected load:
- 100 recurring reminders = 100 rows in cron.job table
- Very low overhead, no accumulation of old jobs
