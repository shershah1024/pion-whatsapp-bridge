-- Migration: Add SECURITY DEFINER to reminder functions
-- Date: 2025-11-09
-- Purpose: Fix permission denied error when creating cron jobs from triggers

-- Update the schedule_reminder_call function to run with elevated permissions
CREATE OR REPLACE FUNCTION schedule_reminder_call(
  p_reminder_id UUID,
  p_phone_number TEXT,
  p_reminder_time TIMESTAMP WITH TIME ZONE,
  p_reminder_text TEXT,
  p_recurrence_pattern TEXT DEFAULT 'once'
)
RETURNS TEXT
SECURITY DEFINER  -- Run with function owner's permissions, not caller's
SET search_path = public, pg_temp  -- Security best practice
AS $$
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

  -- Use correct Azure Container App URL
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

-- Update the trigger function to also run with elevated permissions
CREATE OR REPLACE FUNCTION trigger_schedule_reminder()
RETURNS TRIGGER
SECURITY DEFINER  -- Run with function owner's permissions
SET search_path = public, pg_temp  -- Security best practice
AS $$
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

-- Note: SECURITY DEFINER allows these functions to access cron schema
-- even when called by anon users through triggers
