-- Create ziggy_tasks table
CREATE TABLE IF NOT EXISTS ziggy_tasks (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    status TEXT DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed', 'cancelled')),
    priority TEXT DEFAULT 'medium' CHECK (priority IN ('low', 'medium', 'high', 'urgent')),
    due_date TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    created_by TEXT,
    phone_number TEXT
);

-- Enable Row Level Security
ALTER TABLE ziggy_tasks ENABLE ROW LEVEL SECURITY;

-- Create policy to allow anon users full access
CREATE POLICY "Allow anon users full access to ziggy_tasks"
ON ziggy_tasks
FOR ALL
TO anon
USING (true)
WITH CHECK (true);

-- Create updated_at trigger
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_ziggy_tasks_updated_at
    BEFORE UPDATE ON ziggy_tasks
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Create index on phone_number for faster lookups
CREATE INDEX IF NOT EXISTS idx_ziggy_tasks_phone_number ON ziggy_tasks(phone_number);
CREATE INDEX IF NOT EXISTS idx_ziggy_tasks_status ON ziggy_tasks(status);
CREATE INDEX IF NOT EXISTS idx_ziggy_tasks_created_at ON ziggy_tasks(created_at DESC);
