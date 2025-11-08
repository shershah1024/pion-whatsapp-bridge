-- Create ziggy_reminders table
CREATE TABLE IF NOT EXISTS ziggy_reminders (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    phone_number TEXT NOT NULL,
    reminder_text TEXT NOT NULL,
    reminder_time TIMESTAMPTZ NOT NULL,
    status TEXT DEFAULT 'pending' CHECK (status IN ('pending', 'called', 'completed', 'cancelled')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    call_id TEXT,
    attempts INTEGER DEFAULT 0
);

-- Enable Row Level Security
ALTER TABLE ziggy_reminders ENABLE ROW LEVEL SECURITY;

-- Create policy to allow anon users full access
CREATE POLICY "Allow anon users full access to ziggy_reminders"
ON ziggy_reminders
FOR ALL
TO anon
USING (true)
WITH CHECK (true);

-- Create updated_at trigger
CREATE TRIGGER update_ziggy_reminders_updated_at
    BEFORE UPDATE ON ziggy_reminders
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_ziggy_reminders_phone_number ON ziggy_reminders(phone_number);
CREATE INDEX IF NOT EXISTS idx_ziggy_reminders_status ON ziggy_reminders(status);
CREATE INDEX IF NOT EXISTS idx_ziggy_reminders_time ON ziggy_reminders(reminder_time);
CREATE INDEX IF NOT EXISTS idx_ziggy_reminders_status_time ON ziggy_reminders(status, reminder_time);

-- Create a function to get due reminders
CREATE OR REPLACE FUNCTION get_due_reminders()
RETURNS TABLE (
    id UUID,
    phone_number TEXT,
    reminder_text TEXT,
    reminder_time TIMESTAMPTZ
) AS $$
BEGIN
    RETURN QUERY
    SELECT r.id, r.phone_number, r.reminder_text, r.reminder_time
    FROM ziggy_reminders r
    WHERE r.status = 'pending'
    AND r.reminder_time <= NOW()
    ORDER BY r.reminder_time ASC;
END;
$$ LANGUAGE plpgsql;
