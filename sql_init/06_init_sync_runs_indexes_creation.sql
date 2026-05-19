--Create the indexes that will cover the query


CREATE INDEX IF NOT EXISTS sync_runs_lookup_idx
ON central_metadata.sync_runs (
    project_id,
    form_xml_id,
    object_type,
    object_name,
    started_at DESC
);

CREATE INDEX IF NOT EXISTS sync_runs_success_idx
ON central_metadata.sync_runs (
    project_id,
    form_xml_id,
    object_type,
    object_name,
    finished_at DESC
)
WHERE sync_status = 'success';