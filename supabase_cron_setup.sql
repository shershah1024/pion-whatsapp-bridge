-- Enable required extensions (if not already enabled)
CREATE EXTENSION IF NOT EXISTS pg_cron;
CREATE EXTENSION IF NOT EXISTS http;

-- Function to schedule a reminder call via pg_cron
-- This schedules either a one-time or recurring cron job that will call the user at the specified time
CREATE OR REPLACE FUNCTION schedule_reminder_call(
  p_reminder_id UUID,
  p_phone_number TEXT,
  p_reminder_time TIMESTAMPTZ,
  p_reminder_text TEXT,
  p_recurrence_pattern TEXT DEFAULT NULL
)
RETURNS TEXT AS $$
DECLARE
  v_job_name TEXT;
  v_schedule TEXT;
  v_url TEXT;
  v_body JSONB;
  v_command TEXT;
BEGIN
  -- Create unique job name based on reminder ID
  v_job_name := 'reminder_' || p_reminder_id::TEXT;

  -- Convert timestamp to cron schedule based on recurrence pattern
  -- Format: minute hour day month dayofweek
  IF p_recurrence_pattern IS NULL OR p_recurrence_pattern = 'once' THEN
    -- One-time: specific date and time
    v_schedule := TO_CHAR(p_reminder_time, 'MI HH24 DD MM') || ' *';
  ELSIF p_recurrence_pattern = 'daily' THEN
    -- Daily: same time every day
    v_schedule := TO_CHAR(p_reminder_time, 'MI HH24') || ' * * *';
  ELSIF p_recurrence_pattern = 'weekly' THEN
    -- Weekly: same time and day of week
    v_schedule := TO_CHAR(p_reminder_time, 'MI HH24') || ' * * ' || TO_CHAR(p_reminder_time, 'D');
  ELSIF p_recurrence_pattern = 'monthly' THEN
    -- Monthly: same time and day of month
    v_schedule := TO_CHAR(p_reminder_time, 'MI HH24 DD') || ' * *';
  ELSIF p_recurrence_pattern = 'yearly' THEN
    -- Yearly: same time, day, and month
    v_schedule := TO_CHAR(p_reminder_time, 'MI HH24 DD MM') || ' *';
  ELSE
    RAISE EXCEPTION 'Invalid recurrence pattern: %', p_recurrence_pattern;
  END IF;

  -- Construct the API call body
  v_body := jsonb_build_object(
    'to', p_phone_number,
    'reminder_id', p_reminder_id::TEXT,
    'reminder_text', p_reminder_text
  );

  -- URL to call (your bridge endpoint)
  v_url := 'https://whatsapp-bridge.tslfiles.org/initiate-call';

  -- Build command based on whether this is recurring or one-time
  IF p_recurrence_pattern IS NULL OR p_recurrence_pattern = 'once' THEN
    -- One-time: unschedule after execution
    v_command := format(
      'SELECT net.http_post(url := %L, headers := %L::jsonb, body := %L::jsonb); SELECT cron.unschedule(%L);',
      v_url,
      '{"Content-Type": "application/json"}',
      v_body::TEXT,
      v_job_name
    );
  ELSE
    -- Recurring: don't unschedule, just make the call
    v_command := format(
      'SELECT net.http_post(url := %L, headers := %L::jsonb, body := %L::jsonb);',
      v_url,
      '{"Content-Type": "application/json"}',
      v_body::TEXT
    );
  END IF;

  -- Schedule the cron job
  PERFORM cron.schedule(
    v_job_name,
    v_schedule,
    v_command
  );

  -- Log the scheduled job
  RAISE NOTICE 'Scheduled reminder call: job=%, time=%, phone=%',
    v_job_name, p_reminder_time, p_phone_number;

  RETURN v_job_name;
END;
$$ LANGUAGE plpgsql;

-- Function to cancel a scheduled reminder call
CREATE OR REPLACE FUNCTION cancel_reminder_call(p_reminder_id UUID)
RETURNS BOOLEAN AS $$
DECLARE
  v_job_name TEXT;
  v_result BOOLEAN;
BEGIN
  v_job_name := 'reminder_' || p_reminder_id::TEXT;

  -- Unschedule the job
  PERFORM cron.unschedule(v_job_name);

  RAISE NOTICE 'Cancelled reminder call: job=%', v_job_name;

  RETURN TRUE;
EXCEPTION
  WHEN OTHERS THEN
    RAISE NOTICE 'Failed to cancel job %: %', v_job_name, SQLERRM;
    RETURN FALSE;
END;
$$ LANGUAGE plpgsql;

-- Trigger to automatically schedule cron when reminder is created
CREATE OR REPLACE FUNCTION trigger_schedule_reminder()
RETURNS TRIGGER AS $$
BEGIN
  -- Only schedule if status is pending and time is in the future (or recurring)
  IF NEW.status = 'pending' AND (
    NEW.reminder_time > NOW() OR
    (NEW.recurrence_pattern IS NOT NULL AND NEW.recurrence_pattern != 'once')
  ) THEN
    PERFORM schedule_reminder_call(
      NEW.id,
      NEW.phone_number,
      NEW.reminder_time,
      NEW.reminder_text,
      NEW.recurrence_pattern
    );
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger on ziggy_reminders table
DROP TRIGGER IF EXISTS on_reminder_created ON ziggy_reminders;
CREATE TRIGGER on_reminder_created
  AFTER INSERT ON ziggy_reminders
  FOR EACH ROW
  EXECUTE FUNCTION trigger_schedule_reminder();

-- Trigger to cancel cron when reminder is cancelled/completed
CREATE OR REPLACE FUNCTION trigger_cancel_reminder()
RETURNS TRIGGER AS $$
BEGIN
  -- If status changed from pending to something else, cancel the cron job
  IF OLD.status = 'pending' AND NEW.status != 'pending' THEN
    PERFORM cancel_reminder_call(NEW.id);
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for cancellation
DROP TRIGGER IF EXISTS on_reminder_updated ON ziggy_reminders;
CREATE TRIGGER on_reminder_updated
  AFTER UPDATE ON ziggy_reminders
  FOR EACH ROW
  EXECUTE FUNCTION trigger_cancel_reminder();

-- View to see all scheduled reminder jobs
CREATE OR REPLACE VIEW scheduled_reminder_jobs AS
SELECT
  j.jobid,
  j.jobname,
  j.schedule,
  j.command,
  j.active,
  SUBSTRING(j.jobname FROM 10)::UUID as reminder_id
FROM cron.job j
WHERE j.jobname LIKE 'reminder_%';

-- Grant access
GRANT SELECT ON scheduled_reminder_jobs TO anon;
