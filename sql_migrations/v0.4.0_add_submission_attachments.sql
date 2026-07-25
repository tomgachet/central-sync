-- Migration required when upgrading from central-sync 0.3.x to 0.4.0.
-- Replace your_central_user with the PostgreSQL role used by central-sync.

BEGIN;

CREATE TABLE IF NOT EXISTS central_metadata.submission_attachments (
    project_id INT NOT NULL,
    form_xml_id VARCHAR(100) NOT NULL,
    submission_uuid UUID NOT NULL,
    filename TEXT NOT NULL,
    storage_backend VARCHAR(20) NOT NULL,
    storage_path TEXT NOT NULL,
    content_type TEXT,
    size_bytes BIGINT NOT NULL
        CONSTRAINT submission_attachments_size_bytes_check
        CHECK (size_bytes >= 0),
    etag TEXT,
    last_run_id BIGINT NOT NULL
        CONSTRAINT submission_attachments_last_run_id_fkey
        REFERENCES central_metadata.sync_runs (run_id),
    synced_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, form_xml_id, submission_uuid, filename)
);

CREATE INDEX IF NOT EXISTS submission_attachments_last_run_id_idx
    ON central_metadata.submission_attachments (last_run_id);

ALTER TABLE central_metadata.submission_attachments OWNER TO your_central_user;

COMMIT;
