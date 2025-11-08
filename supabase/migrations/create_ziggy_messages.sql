-- Create ziggy_messages table for storing WhatsApp text conversations
CREATE TABLE IF NOT EXISTS public.ziggy_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone_number TEXT NOT NULL,
    message_content TEXT NOT NULL,
    direction TEXT NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    message_type TEXT DEFAULT 'text',
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    message_id TEXT,
    contact_name TEXT,
    raw_webhook_data JSONB
);

-- Create indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_ziggy_messages_phone_number ON public.ziggy_messages(phone_number);
CREATE INDEX IF NOT EXISTS idx_ziggy_messages_timestamp ON public.ziggy_messages(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_ziggy_messages_phone_timestamp ON public.ziggy_messages(phone_number, timestamp DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ziggy_messages_message_id ON public.ziggy_messages(message_id) WHERE message_id IS NOT NULL;

-- Enable RLS
ALTER TABLE public.ziggy_messages ENABLE ROW LEVEL SECURITY;

-- Allow anon users full access
CREATE POLICY "Allow anon users to select ziggy_messages"
    ON public.ziggy_messages
    FOR SELECT
    TO anon
    USING (true);

CREATE POLICY "Allow anon users to insert ziggy_messages"
    ON public.ziggy_messages
    FOR INSERT
    TO anon
    WITH CHECK (true);

CREATE POLICY "Allow anon users to update ziggy_messages"
    ON public.ziggy_messages
    FOR UPDATE
    TO anon
    USING (true)
    WITH CHECK (true);

CREATE POLICY "Allow anon users to delete ziggy_messages"
    ON public.ziggy_messages
    FOR DELETE
    TO anon
    USING (true);

COMMENT ON TABLE public.ziggy_messages IS 'Stores WhatsApp text message conversations for Ziggy voice assistant';
COMMENT ON COLUMN public.ziggy_messages.phone_number IS 'User phone number in international format';
COMMENT ON COLUMN public.ziggy_messages.message_content IS 'Text content of the message';
COMMENT ON COLUMN public.ziggy_messages.direction IS 'Message direction: inbound (from user) or outbound (to user)';
COMMENT ON COLUMN public.ziggy_messages.message_type IS 'Type of message: text, audio, image, video, interactive';
COMMENT ON COLUMN public.ziggy_messages.message_id IS 'WhatsApp message ID for deduplication';
COMMENT ON COLUMN public.ziggy_messages.contact_name IS 'WhatsApp contact name from profile';
