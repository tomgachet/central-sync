CREATE SCHEMA IF NOT EXISTS central_datasets;
CREATE SCHEMA IF NOT EXISTS central_submissions;
CREATE SCHEMA IF NOT EXISTS central_metadata;

CREATE TABLE IF NOT EXISTS central_metadata.sync_runs (
    run_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id INT NOT NULL,
    form_xml_id VARCHAR(100),
    object_type VARCHAR(20) NOT NULL,        -- dataset | form
    object_name VARCHAR(150) NOT NULL,       -- ex: tg_taxon, Liste_1
    sql_table_name VARCHAR(150),
    sync_mode VARCHAR(20) NOT NULL,          -- upsert | append_only
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    sync_status VARCHAR(20) NOT NULL,        -- running | success | partial_success | failed
    rows_fetched INT NOT NULL DEFAULT 0,
    rows_inserted INT NOT NULL DEFAULT 0,
    rows_updated INT NOT NULL DEFAULT 0,
    rows_deleted INT NOT NULL DEFAULT 0,
    rows_skipped INT NOT NULL DEFAULT 0,
    rows_failed INT NOT NULL DEFAULT 0,
    error_message TEXT
);

CREATE TABLE IF NOT EXISTS central_metadata.sync_runs_detail (
    run_detail_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id BIGINT NOT NULL,
    project_id INT NOT NULL,
    form_xml_id VARCHAR(100),
    object_type VARCHAR(20) NOT NULL,          -- dataset | form_submission
    object_name VARCHAR(150) NOT NULL,         -- ex: Submissions.repeat_session
    sql_table_name VARCHAR(150) NOT NULL,      -- ex: tg_taxon__repeat_session
    submission_uuid UUID,
    entity_uuid UUID,
    central_submission_date TIMESTAMPTZ,
    central_created_at TIMESTAMPTZ,
    central_updated_at TIMESTAMPTZ,
    central_deleted_at TIMESTAMPTZ,
    sync_action VARCHAR(20) NOT NULL,          -- inserted | updated | skipped | failed
    sync_status VARCHAR(20) NOT NULL,          -- success | failed
    rows_fetched INT NOT NULL DEFAULT 1,
    rows_inserted INT NOT NULL DEFAULT 0,
    rows_updated INT NOT NULL DEFAULT 0,
    rows_deleted INT NOT NULL DEFAULT 0,
    rows_skipped INT NOT NULL DEFAULT 0,
    rows_failed INT NOT NULL DEFAULT 0,
    error_message TEXT,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);


CREATE OR REPLACE VIEW central_metadata.last_successful_submissions_sync AS
SELECT
    project_id,
    form_xml_id,
    object_name,
    MAX(central_submission_date) AS max_submission_date,
    MAX(central_updated_at) AS max_updated_at
FROM central_metadata.sync_runs_detail
WHERE object_type = 'form_submission'
  AND object_name = 'Submissions'
  AND sync_status = 'success'
GROUP BY project_id, form_xml_id, object_name;

CREATE OR REPLACE VIEW central_metadata.last_successful_datasets_sync AS
SELECT
    project_id,
    object_name,
    MAX(central_created_at) AS max_created_at,
    MAX(central_updated_at) AS max_updated_at,
    MAX(central_deleted_at) AS max_deleted_at
FROM central_metadata.sync_runs_detail
WHERE object_type = 'dataset'
  AND sync_status = 'success'
GROUP BY project_id, object_name;

-- central_metadata.last_failed_submissions source

CREATE OR REPLACE VIEW central_metadata.last_failed_submissions
AS SELECT run_detail_id,
    run_id,
    project_id,
    form_xml_id,
    submission_uuid,
    central_submission_date,
    central_updated_at,
    sync_status,
    sync_action,
    error_message,
    processed_at
   FROM ( SELECT DISTINCT ON (sync_runs_detail.project_id, sync_runs_detail.form_xml_id, sync_runs_detail.submission_uuid) sync_runs_detail.run_detail_id,
            sync_runs_detail.run_id,
            sync_runs_detail.project_id,
            sync_runs_detail.form_xml_id,
            sync_runs_detail.submission_uuid,
            sync_runs_detail.central_submission_date,
            sync_runs_detail.central_updated_at,
            sync_runs_detail.sync_status,
            sync_runs_detail.sync_action,
            sync_runs_detail.error_message,
            sync_runs_detail.processed_at
           FROM central_metadata.sync_runs_detail
          WHERE sync_runs_detail.object_type::text = 'form_submission'::text AND sync_runs_detail.submission_uuid IS NOT NULL
          ORDER BY sync_runs_detail.project_id, sync_runs_detail.form_xml_id, sync_runs_detail.submission_uuid, sync_runs_detail.processed_at DESC, sync_runs_detail.run_detail_id DESC) t
  WHERE sync_status::text = 'failed'::text;