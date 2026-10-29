CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS sms_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient TEXT NOT NULL,
    content TEXT NOT NULL,
    title TEXT NOT NULL CHECK (title IN ('OTP', 'Info', 'Promotion', 'News')),
    submitted_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'sent', 'failed', 'delivered', 'undelivered')),
    provider_code TEXT,
    provider_message TEXT,
    provider_detail TEXT,
    provider_partition_key TEXT,
    provider_range_key TEXT,
    error_detail TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS sms_transactions_created_at_idx
    ON sms_transactions (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS sms_transactions_status_created_at_idx
    ON sms_transactions (status, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS sms_transactions_recipient_created_at_idx
    ON sms_transactions (recipient, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS sms_transactions_submitted_at_idx
    ON sms_transactions (submitted_at DESC) WHERE submitted_at IS NOT NULL;
