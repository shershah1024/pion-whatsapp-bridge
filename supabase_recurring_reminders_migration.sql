-- Migration to add recurring reminder support
-- Run this in Supabase SQL Editor after running supabase_reminders_migration.sql

-- Add recurrence_pattern column
ALTER TABLE ziggy_reminders
ADD COLUMN IF NOT EXISTS recurrence_pattern TEXT CHECK (
    recurrence_pattern IN (NULL, 'once', 'daily', 'weekly', 'monthly', 'yearly')
);

-- Add comment for documentation
COMMENT ON COLUMN ziggy_reminders.recurrence_pattern IS
'Recurrence pattern: NULL or "once" for one-time reminders, "daily", "weekly", "monthly", or "yearly" for recurring reminders';

-- Update existing reminders to be one-time (NULL = one-time)
UPDATE ziggy_reminders
SET recurrence_pattern = NULL
WHERE recurrence_pattern IS NULL;

-- Create index for querying recurring reminders
CREATE INDEX IF NOT EXISTS idx_ziggy_reminders_recurrence ON ziggy_reminders(recurrence_pattern)
WHERE recurrence_pattern IS NOT NULL;

-- View to see recurring vs one-time reminders
CREATE OR REPLACE VIEW reminder_types AS
SELECT
    CASE
        WHEN recurrence_pattern IS NULL OR recurrence_pattern = 'once' THEN 'one-time'
        ELSE 'recurring'
    END as reminder_type,
    recurrence_pattern,
    COUNT(*) as count
FROM ziggy_reminders
WHERE status = 'pending'
GROUP BY recurrence_pattern;

GRANT SELECT ON reminder_types TO anon;
