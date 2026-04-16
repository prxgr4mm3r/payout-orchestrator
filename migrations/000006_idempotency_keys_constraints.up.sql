ALTER TABLE idempotency_keys
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ADD CONSTRAINT idempotency_keys_client_id_fkey FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE,
    ADD CONSTRAINT idempotency_keys_payout_id_fkey FOREIGN KEY (payout_id) REFERENCES payouts(id) ON DELETE CASCADE,
    ADD CONSTRAINT idempotency_keys_key_check CHECK (key <> ''),
    ADD CONSTRAINT idempotency_keys_request_hash_check CHECK (request_hash <> '');
