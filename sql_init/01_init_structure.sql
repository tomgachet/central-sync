CREATE SCHEMA IF NOT EXISTS central_datasets;
CREATE SCHEMA IF NOT EXISTS central_submissions;
CREATE SCHEMA IF NOT EXISTS central_metadata;

CREATE TABLE IF NOT EXISTS central_metadata.sync_runs (
    run_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id INT NOT NULL,
    form_xml_id VARCHAR(100),
    object_type VARCHAR(20) NOT NULL,        
    object_name VARCHAR(150) NOT NULL, 
    sql_table_name VARCHAR(150) NOT NULL,
    sync_mode VARCHAR(20) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    sync_status VARCHAR(20) NOT NULL,
    sync_in_submission_date TIMESTAMPTZ,
    sync_in_updated_at TIMESTAMPTZ,
    sync_out_submission_date TIMESTAMPTZ,
    sync_out_updated_at TIMESTAMPTZ,
    rows_fetched INT NOT NULL DEFAULT 0,
    rows_inserted INT NOT NULL DEFAULT 0,
    rows_updated INT NOT NULL DEFAULT 0,
    rows_skipped INT NOT NULL DEFAULT 0,
    error_message TEXT
);

CREATE OR REPLACE VIEW central_metadata.last_successful_sync_runs AS
SELECT DISTINCT ON (project_id, form_xml_id, object_type, object_name)
    run_id,
    project_id,
    form_xml_id,
    object_type,
    object_name,
    sql_table_name,
    sync_mode,
    started_at,
    finished_at,
    sync_status,
    sync_in_submission_date,
    sync_in_updated_at,
    sync_out_submission_date,
    sync_out_updated_at,
    rows_fetched,
    rows_inserted,
    rows_updated,
    rows_skipped,
    error_message
FROM central_metadata.sync_runs
WHERE sync_status = 'success'
ORDER BY
    project_id,
    form_xml_id,
    object_type,
    object_name,
    finished_at DESC,
    run_id DESC;


CREATE TABLE IF NOT EXISTS central_metadata.failed_submissions (
    failed_submission_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id INT NOT NULL,
    form_xml_id VARCHAR(100) NOT NULL,
    submission_uuid UUID NOT NULL,
    submission_date TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    first_failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    retry_count INT NOT NULL DEFAULT 0,
    sync_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    failed_object_name VARCHAR(150) NOT NULL,
    last_error_message TEXT NOT NULL,
    root_table_name VARCHAR(150) NOT NULL,
    last_run_id BIGINT,
    resolved_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS central_metadata.failed_dataset_entities (
    failed_entity_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id INT NOT NULL,
    dataset_name VARCHAR(150) NOT NULL,
    sql_table_name VARCHAR(150) NOT NULL,
    entity_uuid UUID NOT NULL,
    entity_version BIGINT,
    first_failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    retry_count INT NOT NULL DEFAULT 0,
    sync_status VARCHAR(20) NOT NULL DEFAULT 'pending',   -- pending | retrying | resolved | ignored
    last_error_message TEXT NOT NULL,
    last_run_id BIGINT,
    resolved_at TIMESTAMPTZ
);