DROP INDEX IF EXISTS ux_payouts_client_external_id;

ALTER TABLE payouts
    DROP COLUMN IF EXISTS recipient_account_id,
    DROP COLUMN IF EXISTS recipient_name,
    DROP COLUMN IF EXISTS external_id;
