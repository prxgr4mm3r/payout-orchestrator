ALTER TABLE outbox_events
    ADD COLUMN entity_id UUID,
    ADD CONSTRAINT outbox_events_entity_id_fkey FOREIGN KEY (entity_id) REFERENCES payouts(id) ON DELETE CASCADE,
    ADD CONSTRAINT outbox_events_event_type_check CHECK (event_type <> ''),
    ADD CONSTRAINT outbox_events_status_check CHECK (status IN ('pending', 'processed'));

UPDATE outbox_events
SET entity_id = NULLIF(payload ->> 'payout_id', '')::uuid
WHERE entity_id IS NULL
  AND payload ? 'payout_id';

ALTER TABLE outbox_events
    ALTER COLUMN entity_id SET NOT NULL;
