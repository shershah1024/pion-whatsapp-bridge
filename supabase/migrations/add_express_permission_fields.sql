-- Migration: Add Express Permission System fields to whatsapp_call_permissions table
-- This enables proactive permission requests with 72-hour expiry windows

-- Add new columns for express permission tracking
ALTER TABLE whatsapp_call_permissions
ADD COLUMN IF NOT EXISTS permission_requested_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN IF NOT EXISTS permission_approved_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN IF NOT EXISTS permission_expires_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN IF NOT EXISTS permission_request_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_permission_request_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN IF NOT EXISTS permission_source TEXT CHECK (permission_source IN ('inbound_call', 'express_request', 'manual'));

-- Add index for expiry queries (find expired permissions)
CREATE INDEX IF NOT EXISTS idx_whatsapp_call_permissions_expires
ON whatsapp_call_permissions(permission_expires_at)
WHERE permission_expires_at IS NOT NULL;

-- Add index for rate limit queries
CREATE INDEX IF NOT EXISTS idx_whatsapp_call_permissions_last_request
ON whatsapp_call_permissions(last_permission_request_at)
WHERE last_permission_request_at IS NOT NULL;

-- Add comment explaining the table
COMMENT ON TABLE whatsapp_call_permissions IS
'Tracks WhatsApp call permissions with two methods:
1. Automatic (inbound_call): User calls first, grants permanent permission
2. Express (express_request): Business requests permission, 72-hour window';

-- Add column comments
COMMENT ON COLUMN whatsapp_call_permissions.permission_source IS
'Source of permission: inbound_call (permanent), express_request (72h), or manual';

COMMENT ON COLUMN whatsapp_call_permissions.permission_expires_at IS
'Expiry time for express permissions (72 hours from approval). NULL for permanent permissions.';

COMMENT ON COLUMN whatsapp_call_permissions.permission_request_count IS
'Number of permission requests sent (for rate limiting: max 2 per 7 days)';

COMMENT ON COLUMN whatsapp_call_permissions.last_permission_request_at IS
'Timestamp of most recent permission request (for rate limiting: 1 per 24 hours)';
