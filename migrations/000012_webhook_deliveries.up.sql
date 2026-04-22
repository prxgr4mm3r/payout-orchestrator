CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    payout_id UUID NOT NULL,
    client_id UUID NOT NULL,
    target_url TEXT NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (payout_id) REFERENCES payouts(id) ON DELETE CASCADE,
    FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE,
    CONSTRAINT webhook_deliveries_status_check CHECK (status IN ('pending', 'processing', 'delivered', 'failed')),
    CONSTRAINT webhook_deliveries_attempt_count_check CHECK (attempt_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_payout_id_created_at
    ON webhook_deliveries (payout_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_client_id_created_at
    ON webhook_deliveries (client_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status_created_at
    ON webhook_deliveries (status, created_at ASC);
