ALTER TABLE outbox_events
    ADD COLUMN claimed_at TIMESTAMPTZ;

ALTER TABLE outbox_events
    DROP CONSTRAINT IF EXISTS outbox_events_status_check,
    ADD CONSTRAINT outbox_events_status_check CHECK (status IN ('pending', 'processing', 'processed'));
