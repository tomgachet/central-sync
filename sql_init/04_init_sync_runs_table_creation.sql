--Create the table that will store the synchronization history.
--You need to connect to the database created previously


CREATE TABLE IF NOT EXISTS central_metadata.sync_runs (
    run_id BIGSERIAL PRIMARY KEY,

    project_id INT NOT NULL,
    form_xml_id TEXT,
    object_type TEXT NOT NULL,
    object_name TEXT NOT NULL,
    sql_table_name TEXT NOT NULL,
    sync_mode TEXT NOT NULL,

    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    sync_status TEXT NOT NULL,

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

--Change the owner to your central_user (replace the role name example to fit yours)
ALTER TABLE central_metadata.sync_runs  OWNER TO central_user;
