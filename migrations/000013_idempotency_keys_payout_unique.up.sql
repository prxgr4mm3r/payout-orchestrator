ALTER TABLE idempotency_keys
    ADD CONSTRAINT idempotency_keys_payout_id_key UNIQUE (payout_id);
