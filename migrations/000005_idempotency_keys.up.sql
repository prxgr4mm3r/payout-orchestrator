CREATE TABLE idempotency_keys (
    key TEXT NOT NULL,
    client_id UUID NOT NULL,
    request_hash TEXT NOT NULL,
    payout_id UUID NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (client_id, key)
);