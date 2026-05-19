--Create the view to retrieve the last succesful sync per oject

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
    sync_in_submission_date,
    sync_in_updated_at,
    sync_out_submission_date,
    sync_out_updated_at,
    rows_fetched,
    rows_inserted,
    rows_updated,
    rows_skipped
FROM central_metadata.sync_runs
WHERE sync_status = 'success'
ORDER BY
    project_id,
    form_xml_id,
    object_type,
    object_name,
    finished_at DESC,
    run_id DESC;


--Change the owner to your central_user (replace the role name example to fit yours)
ALTER VIEW central_metadata.last_successful_sync_runs OWNER TO central_user;