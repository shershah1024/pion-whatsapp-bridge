-- Migration: Fix reminder URL and reinstall triggers
-- Date: 2025-11-09
-- Purpose: Update Azure Container App URL and ensure triggers are installed

-- Update the schedule_reminder_call function with correct Azure URL
CREATE OR REPLACE FUNCTION schedule_reminder_call(
  p_reminder_id UUID,
  p_phone_number TEXT,
  p_reminder_time TIMESTAMP WITH TIME ZONE,
  p_reminder_text TEXT,
  p_recurrence_pattern TEXT DEFAULT 'once'
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
  IF p_recurrence_pattern IS NULL OR p_recurrence_pattern = 'once' THEN
    v_schedule := TO_CHAR(p_reminder_time, 'MI HH24 DD MM') || ' *';
  ELSIF p_recurrence_pattern = 'daily' THEN
    v_schedule := TO_CHAR(p_reminder_time, 'MI HH24') || ' * * *';
  ELSIF p_recurrence_pattern = 'weekly' THEN
    v_schedule := TO_CHAR(p_reminder_time, 'MI HH24') || ' * * ' || TO_CHAR(p_reminder_time, 'D');
  ELSIF p_recurrence_pattern = 'monthly' THEN
    v_schedule := TO_CHAR(p_reminder_time, 'MI HH24 DD') || ' * *';
  ELSIF p_recurrence_pattern = 'yearly' THEN
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

  -- UPDATED: Use correct Azure Container App URL
  v_url := 'https://whatsapp-bridge.agreeablehill-44d96eb3.eastus.azurecontainerapps.io/initiate-call';

  -- Build command based on whether this is recurring or one-time
  IF p_recurrence_pattern IS NULL OR p_recurrence_pattern = 'once' THEN
    v_command := format(
      'SELECT net.http_post(url := %L, headers := %L::jsonb, body := %L::jsonb); SELECT cron.unschedule(%L);',
      v_url,
      '{"Content-Type": "application/json"}',
      v_body::TEXT,
      v_job_name
    );
  ELSE
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

  RAISE NOTICE 'Scheduled reminder call: job=%, time=%, phone=%',
    v_job_name, p_reminder_time, p_phone_number;

  RETURN v_job_name;
END;
$$ LANGUAGE plpgsql;

-- Reinstall trigger function for automatic scheduling
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

-- Reinstall trigger on ziggy_reminders table
DROP TRIGGER IF EXISTS on_reminder_created ON ziggy_reminders;
CREATE TRIGGER on_reminder_created
  AFTER INSERT ON ziggy_reminders
  FOR EACH ROW
  EXECUTE FUNCTION trigger_schedule_reminder();

-- Manually schedule the 2 past-due reminders
-- Reminder 1: "Call someone" at 12:55:00
SELECT schedule_reminder_call(
  '74492b86-a299-493a-a749-8afee7bead91'::UUID,
  '919885842349',
  '2025-11-09 12:55:00+00'::TIMESTAMP WITH TIME ZONE,
  'Call someone',
  'once'
);

-- Reminder 2: "Do the new task" at 13:01:00
SELECT schedule_reminder_call(
  '99ba7843-45a1-47dc-853b-ce71446b2bd2'::UUID,
  '919885842349',
  '2025-11-09 13:01:00+00'::TIMESTAMP WITH TIME ZONE,
  'Do the new task',
  'once'
);

-- Note: These will execute immediately since the times are in the past
