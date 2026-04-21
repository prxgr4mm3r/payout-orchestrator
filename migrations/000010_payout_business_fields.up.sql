ALTER TABLE payouts
    ADD COLUMN external_id VARCHAR(255),
    ADD COLUMN recipient_name VARCHAR(255),
    ADD COLUMN recipient_account_id VARCHAR(255);

CREATE UNIQUE INDEX IF NOT EXISTS ux_payouts_client_external_id
    ON payouts (client_id, external_id)
    WHERE external_id IS NOT NULL AND external_id <> '';
