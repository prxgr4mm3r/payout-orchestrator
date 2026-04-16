ALTER TABLE outbox_events
    ALTER COLUMN entity_id DROP NOT NULL,
    DROP CONSTRAINT IF EXISTS outbox_events_status_check,
    DROP CONSTRAINT IF EXISTS outbox_events_event_type_check,
    DROP CONSTRAINT IF EXISTS outbox_events_entity_id_fkey,
    DROP COLUMN IF EXISTS entity_id;
