CREATE TABLE IF NOT EXISTS payouts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    client_id UUID NOT NULL,
    funding_source_id UUID NOT NULL,
    amount NUMERIC(10, 2) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE,
    FOREIGN KEY (funding_source_id) REFERENCES funding_sources(id) ON DELETE CASCADE,
    CONSTRAINT payouts_status_check CHECK (status IN ('pending', 'processing', 'succeeded', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_payouts_client_id_created_at
    ON payouts (client_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_payouts_status_created_at
    ON payouts (status, created_at ASC);
