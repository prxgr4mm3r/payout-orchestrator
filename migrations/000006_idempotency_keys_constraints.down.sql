ALTER TABLE idempotency_keys
    DROP CONSTRAINT IF EXISTS idempotency_keys_request_hash_check,
    DROP CONSTRAINT IF EXISTS idempotency_keys_key_check,
    DROP CONSTRAINT IF EXISTS idempotency_keys_payout_id_fkey,
    DROP CONSTRAINT IF EXISTS idempotency_keys_client_id_fkey,
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC';
