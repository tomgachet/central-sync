-- Migration required when upgrading from central-sync 0.4.x to 0.5.0.
-- Replace your_central_user with the PostgreSQL role used by central-sync.

BEGIN;

CREATE SCHEMA IF NOT EXISTS central_users;

GRANT ALL ON SCHEMA central_users TO your_central_user;

ALTER TABLE central_metadata.sync_runs_detail
    ADD COLUMN IF NOT EXISTS app_user_id BIGINT;

CREATE INDEX IF NOT EXISTS sync_runs_detail_app_user_idx
    ON central_metadata.sync_runs_detail (project_id, app_user_id)
    WHERE app_user_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS central_users.app_users (
    project_id INT NOT NULL,
    app_user_id BIGINT NOT NULL,
    display_name TEXT NOT NULL,
    actor_type VARCHAR(30) NOT NULL,
    central_created_at TIMESTAMPTZ,
    central_updated_at TIMESTAMPTZ,
    central_deleted_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_by JSONB,
    properties JSONB NOT NULL DEFAULT '{}'::jsonb,
    revoked BOOLEAN NOT NULL,
    missing_from_central BOOLEAN NOT NULL DEFAULT FALSE,
    missing_since TIMESTAMPTZ,
    first_synced_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_synced_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_run_id BIGINT NOT NULL
        CONSTRAINT app_users_last_run_id_fkey
        REFERENCES central_metadata.sync_runs (run_id),
    PRIMARY KEY (project_id, app_user_id)
);

CREATE INDEX IF NOT EXISTS app_users_last_run_id_idx
    ON central_users.app_users (last_run_id);

ALTER TABLE central_users.app_users OWNER TO your_central_user;

COMMIT;
