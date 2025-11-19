-- Create ziggy_notes table for storing user notes
-- Users can ask Ziggy to create notes during conversations or voice calls
-- Examples: "Make a note that I need to buy groceries", "Note: meeting with John at 3pm"

CREATE TABLE IF NOT EXISTS public.ziggy_notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone_number TEXT NOT NULL,
    note_content TEXT NOT NULL
);

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_ziggy_notes_phone_number ON public.ziggy_notes(phone_number);

-- Create full-text search index for searching note content
CREATE INDEX IF NOT EXISTS idx_ziggy_notes_search ON public.ziggy_notes USING GIN(to_tsvector('english', note_content));

-- Enable RLS
ALTER TABLE public.ziggy_notes ENABLE ROW LEVEL SECURITY;

-- Allow anon users full access
CREATE POLICY "Allow anon users to select ziggy_notes"
    ON public.ziggy_notes
    FOR SELECT
    TO anon
    USING (true);

CREATE POLICY "Allow anon users to insert ziggy_notes"
    ON public.ziggy_notes
    FOR INSERT
    TO anon
    WITH CHECK (true);

CREATE POLICY "Allow anon users to update ziggy_notes"
    ON public.ziggy_notes
    FOR UPDATE
    TO anon
    USING (true)
    WITH CHECK (true);

CREATE POLICY "Allow anon users to delete ziggy_notes"
    ON public.ziggy_notes
    FOR DELETE
    TO anon
    USING (true);

-- Add table and column comments for documentation
COMMENT ON TABLE public.ziggy_notes IS 'Stores notes created by users via Ziggy assistant';
COMMENT ON COLUMN public.ziggy_notes.phone_number IS 'User phone number in international format';
COMMENT ON COLUMN public.ziggy_notes.note_content IS 'The actual note text content';
