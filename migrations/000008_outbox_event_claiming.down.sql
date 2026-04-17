ALTER TABLE outbox_events
    DROP CONSTRAINT IF EXISTS outbox_events_status_check,
    ADD CONSTRAINT outbox_events_status_check CHECK (status IN ('pending', 'processed'));

ALTER TABLE outbox_events
    DROP COLUMN IF EXISTS claimed_at;
