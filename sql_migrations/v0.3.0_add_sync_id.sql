-- Migration required when upgrading from central-sync 0.2.x to 0.3.0.
-- Add launch-level identifiers to databases initialized before sync_id existed.
--
-- Historical rows remain NULL because their original central-sync launch cannot
-- be reconstructed reliably. Every row created by current central-sync versions
-- receives a sync_id.

BEGIN;

ALTER TABLE central_metadata.sync_runs
    ADD COLUMN IF NOT EXISTS sync_id UUID;

CREATE INDEX IF NOT EXISTS sync_runs_sync_id_idx
    ON central_metadata.sync_runs (sync_id);

CREATE INDEX IF NOT EXISTS sync_runs_detail_run_id_idx
    ON central_metadata.sync_runs_detail (run_id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'sync_runs_detail_run_id_fkey'
          AND conrelid = 'central_metadata.sync_runs_detail'::regclass
    ) THEN
        ALTER TABLE central_metadata.sync_runs_detail
            ADD CONSTRAINT sync_runs_detail_run_id_fkey
            FOREIGN KEY (run_id)
            REFERENCES central_metadata.sync_runs (run_id)
            NOT VALID;
    END IF;
END
$$;

ALTER TABLE central_metadata.sync_runs_detail
    VALIDATE CONSTRAINT sync_runs_detail_run_id_fkey;

COMMIT;
