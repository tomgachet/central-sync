# central-sync

`central-sync` synchronizes data from an ODK Central instance to a PostgreSQL server through the ODK Central API and its OData endpoints.

It is designed for scheduled or manual sync jobs where ODK Central remains the source of truth while PostgreSQL provides structured, queryable data for business workflows, downstream information systems, reporting, and application integrations.

French documentation: [README.fr.md](README.fr.md)

## Features

- Synchronizes ODK Central datasets, also called entity lists.
- Synchronizes form submissions, including repeat groups.
- Creates and evolves PostgreSQL tables when Central schemas change.
- Stores geometry values as GeoJSON.
- Tracks sync runs and row-level details in `central_metadata`.
- Supports incremental form sync with `append_only` and `upsert` modes.
- Can restrict form sync to approved submissions.
- Can approve submissions in Central after a successful database sync.
- Runs as a single Go binary with local configuration files.

## Requirements

- An ODK Central account with access to the projects, datasets and forms to synchronize.
- A PostgreSQL database for each configured ODK Central project.
- A PostgreSQL role with access to the sync schemas.
- The following files next to the binary:
  - `.env`
  - `central_config.yaml`

For development, Go is required. The project currently targets Go `1.26.x` in CI.

## Installation

Download the `.tar.gz` archive for your platform from the release page, then extract the `central-sync` binary into a directory containing `.env` and `central_config.yaml`.

```sh
tar -xzf central-sync-linux-amd64.tar.gz
```

The binary name stays `central-sync` across versions, so production cron jobs and scripts do not need to change when you upgrade.

If you download the raw Linux binary directly from GitHub, make it executable and rename it:

```sh
chmod +x central-sync-linux-amd64
mv central-sync-linux-amd64 central-sync
```

From source:

```sh
go test ./...
go build -o central-sync
```

## Environment File

Create `.env` next to the binary:

```env
ODK_CENTRAL_URL=https://central.example.org
ODK_CENTRAL_USER_EMAIL=user@example.org
ODK_CENTRAL_USER_PASSWORD=your_password

PG_HOST=localhost
PG_PORT=5432
PG_USER=central_user
PG_PASSWORD=pg_central_user_password
PG_SSLMODE=disable
```

Required variables:

| Variable | Description |
| --- | --- |
| `ODK_CENTRAL_URL` | Base URL of the ODK Central instance. |
| `ODK_CENTRAL_USER_EMAIL` | Central login email. |
| `ODK_CENTRAL_USER_PASSWORD` | Central password. |
| `PG_HOST` | PostgreSQL host. |
| `PG_PORT` | PostgreSQL port. |
| `PG_USER` | PostgreSQL role used by `central-sync`. |
| `PG_SSLMODE` | PostgreSQL SSL mode, for example `disable` or `require`. |

`PG_PASSWORD` is optional at parsing time, but most PostgreSQL setups require it.

Do not commit `.env`. It contains credentials and Central tokens may also be cached locally by the program.

## Project Configuration

Create `central_config.yaml` next to the binary:

```yaml
attachment_storage:
  backend: "local"
  local_directory: "/var/lib/central-sync/attachments"

projects:
  - project_id: 1
    project_name: "Example project"
    database_name: "central_project_1"
    datasets:
      - name: "species"
        table_name: "species"
        sync: true
      - name: "sites"
        table_name: "sites"
        sync: false
    forms:
      - xml_form_id: "site_visit"
        table_name: "site_visit"
        sync: true
        sync_mode: "upsert"
        approved_only: true
        approve_after_sync: false
        sync_attachments: true

  - project_id: 2
    project_name: "Another project"
    database_name: "central_project_2"
    datasets: []
    forms: []
```

### Attachment Storage Fields

| Field | Required | Description |
| --- | --- | --- |
| `backend` | When attachment sync is enabled | Attachment storage backend. Currently only `local` is supported. |
| `local_directory` | When attachment sync uses `backend: local` | Root directory where submission attachments will be stored. Relative paths are resolved from the process working directory. |

### Project Fields

| Field | Required | Description |
| --- | --- | --- |
| `project_id` | Yes | Numeric ODK Central project ID. Must be greater than `0`. |
| `project_name` | No | Informational name only. |
| `database_name` | Yes | Target PostgreSQL database for this project. |
| `datasets` | No | Dataset mappings to synchronize. |
| `forms` | No | Form mappings to synchronize. |

### Dataset Fields

| Field | Required | Description |
| --- | --- | --- |
| `name` | Yes | ODK Central dataset/entity list name. |
| `table_name` | Yes | Target table name in `central_datasets`. |
| `sync` | Yes | Only entries with `sync: true` are synchronized. |

### Form Fields

| Field | Required | Description |
| --- | --- | --- |
| `xml_form_id` | Yes | ODK Central XML form ID. |
| `table_name` | Yes | Root target table name in `central_submissions`. Repeat tables are derived from this name. |
| `sync` | Yes | Only entries with `sync: true` are synchronized. |
| `sync_mode` | No | `append_only` by default. Can be `append_only` or `upsert`. |
| `approved_only` | No | When `true`, only approved submissions are fetched. |
| `approve_after_sync` | No | When `true`, successfully synchronized submissions are approved in Central after the PostgreSQL commit. |
| `sync_attachments` | No | When `true`, submission attachments are synchronized using the configured `attachment_storage`. Defaults to `false`. |

Table names must be unique inside the same project across dataset and form mappings.

### Submission Attachment Synchronization

When `sync_attachments: true`, attachments are synchronized after the submission rows have been committed to PostgreSQL and before `approve_after_sync` is applied. Files reported as missing by Central are counted but not downloaded.

The local backend writes files atomically below `local_directory` using this layout:

```text
project-{project_id}/form-{xml_form_id}/submission-{submission_uuid}/{filename}
```

Path components are escaped before use. Directories use mode `0750` and files use mode `0640`; the operating-system user running `central-sync` must be able to create and write below `local_directory`. Include this directory in backups together with the project database.

`central-sync` calculates a SHA-256 checksum while downloading. An existing file with identical content is left unchanged. Different content replaces the file atomically and is reported as `replaced` in the logs. The checksum is stored in attachment metadata.

Stored file metadata is upserted in `central_metadata.submission_attachments`. A download, storage, or metadata error marks the submission as failed, prevents automatic approval, and makes it eligible for the existing failed-submission retry flow.

Each form row in `central_metadata.sync_runs` records `attachments_expected`, `attachments_present`, `attachments_stored`, and `attachments_failed` totals for operational monitoring.

## PostgreSQL Setup

Each configured project points to one PostgreSQL database. That database must already exist and contain the required schemas and metadata tables.

Run the initialization scripts with a privileged PostgreSQL role, adapting database and role names first:

```sh
psql -d your_database -f sql_init/01_init_structure.sql
psql -d your_database -f sql_init/02_init_role_and_privileges.sql
```

After retrieving a new `central-sync` version, inspect `sql_migrations/` for files newer than the previously installed version. Run the required migrations in version order on every project database before starting the new binary for the first time.

For an upgrade from 0.2.x to 0.3.0, run:

```sh
psql -d your_database -f sql_migrations/v0.3.0_add_sync_id.sql
```

Historical metadata rows keep a NULL `sync_id` because their original process launch cannot be reconstructed reliably.

The migration also validates the foreign key from `sync_runs_detail.run_id` to `sync_runs.run_id`. It stops without committing if historical orphan detail rows exist; resolve those rows before retrying the migration.

For an upgrade from 0.3.x to 0.4.0, replace `your_central_user` in the migration with the PostgreSQL role used by `central-sync`, then run:

```sh
psql -d your_database -f sql_migrations/v0.4.0_add_submission_attachments.sql
```

The structure script creates:

- `central_datasets`
- `central_submissions`
- `central_metadata`
- `central_metadata.sync_runs`
- `central_metadata.sync_runs_detail`
- `central_metadata.submission_attachments`
- metadata views used for incremental sync and retry tracking

The privileges script is a template. Replace `your_central_user`, `your_central_user_password` and `your_database` before running it.

A typical role setup is:

```sql
CREATE ROLE central_user WITH
    NOSUPERUSER
    NOCREATEDB
    NOCREATEROLE
    NOINHERIT
    LOGIN
    NOREPLICATION
    NOBYPASSRLS
    CONNECTION LIMIT -1
    PASSWORD 'pg_central_user_password';

GRANT CONNECT ON DATABASE your_database TO central_user;
GRANT ALL ON SCHEMA central_datasets TO central_user;
GRANT ALL ON SCHEMA central_submissions TO central_user;
GRANT ALL ON SCHEMA central_metadata TO central_user;
```

Use a dedicated PostgreSQL role rather than a superuser for regular sync runs.

## Running

Run the binary from the directory containing `.env` and `central_config.yaml`:

```sh
cd /path/to/central-sync-directory
./central-sync
```

Print the version:

```sh
./central-sync version
```

The program runs in this order:

1. Load `.env`.
2. Load and validate `central_config.yaml`.
3. Acquire a PostgreSQL advisory lock for each configured project database that has active dataset or form mappings.
4. Authenticate to ODK Central.
5. Synchronize configured datasets.
6. Synchronize configured forms.
7. Write logs to stdout and `central-sync.log`.

### Concurrent Runs

`central-sync` fails fast when another process is already synchronizing the same project database. The lock is a PostgreSQL advisory lock held on a dedicated database connection for the duration of the process, so it works across separate processes and is released automatically when the process exits or the connection is lost.

The lock scope is one PostgreSQL project database. Two configured projects that use different PostgreSQL databases can still synchronize in parallel, but two runs targeting the same database cannot.

If a second run starts while the lock is held, it logs a `sync lock error` and exits before reading or writing Central data. When available, the error includes the latest `central_metadata.sync_runs` row still marked as `running`, including its `run_id`, `project_id`, object type, object name, and start time. Finished, failed, and partial-success runs do not block future executions.

## Central Endpoints Used

`central-sync` uses `ODK_CENTRAL_URL` as the base URL. The configured Central user must have permissions for every project, dataset, form, and submission action listed below.

| Method | Endpoint | Purpose |
| --- | --- | --- |
| `POST` | `/v1/sessions` | Creates or refreshes the Central session token. |
| `GET` | `/v1/projects/{projectId}` | Checks that a configured project exists. |
| `GET` | `/v1/projects/{projectId}/forms` | Lists project forms and validates configured `xml_form_id` values. |
| `GET` | `/v1/projects/{projectId}/forms/{xmlFormId}.svc` | Reads the form OData service document and discovers root and repeat tables. |
| `GET` | `/v1/projects/{projectId}/forms/{xmlFormId}.svc/$metadata` | Reads OData metadata XML for form table schemas. |
| `GET` | `/v1/projects/{projectId}/forms/{xmlFormId}.svc/{odataTableUrl}` | Fetches submission rows from OData tables, including `Submissions` and repeat tables. Uses `$top=1000`, `$count=true`, optional `$filter`, and follows `@odata.nextLink`. |
| `PATCH` | `/v1/projects/{projectId}/forms/{xmlFormId}/submissions/{instanceId}` | Sets `reviewState` to `approved` when `approve_after_sync` is enabled. |
| `POST` | `/v1/projects/{projectId}/forms/{xmlFormId}/submissions/{instanceId}/comments` | Adds a sync comment after approval. |
| `GET` | `/v1/projects/{projectId}/datasets/{datasetName}` | Reads dataset metadata and properties. |
| `GET` | `/v1/projects/{projectId}/datasets/{datasetName}.svc/Entities` | Fetches dataset entities through OData. Uses `$top=1000`, `$count=true`, optional `$filter`, and follows `@odata.nextLink`. |
| `GET` | `/v1/projects/{projectId}/datasets/{datasetName}/entities.geojson` | Fetches dataset geometries as GeoJSON when geometry values are present. |

For manual API testing, see the [`central-api-bruno`](https://github.com/tomgachet/central-api-bruno) repository, which provides a Bruno collection for exercising Central API endpoints.

## Sync Behavior

### Filters

`central-sync` builds OData `$filter` expressions from the last successful sync exposed by the SQL views in `central_metadata`, especially `last_successful_submissions_sync` and `last_successful_datasets_sync`. These filters limit each run to records that changed after the latest successful cursor for the same project and dataset or form.

For datasets, active entities are fetched when either `__system/createdAt` or `__system/updatedAt` is greater than the previous successful sync cursor. Deleted entities are fetched separately with `__system/deletedAt ne null`; after the first successful deleted-entity sync, this is narrowed to `__system/deletedAt gt <last_deleted_at>`.

For form submissions, the filter depends on `sync_mode`:

| Mode | Filter behavior |
| --- | --- |
| `append_only` | Fetches submissions where `submissionDate` is greater than the previous successful submission cursor. |
| `upsert` | Fetches submissions where `submissionDate` or `updatedAt` is greater than the previous successful cursor. |

When `approved_only: true`, the form filter also requires `reviewState eq 'approved'`. For repeat tables, the same submission system fields are addressed through `$root/Submissions/__system/...`, so repeat rows follow the same root submission filter.

Failed form submissions are also retried outside the normal incremental filter. `central-sync` reads `central_metadata.last_failed_submissions`, fetches those submission UUIDs again from Central, then merges them with the rows returned by the regular incremental OData filter so they are attempted again in the same run.

On a first run, no previous successful cursor exists, so the incremental date filter is omitted and Central returns the matching rows for the configured dataset or form.

### Datasets

Datasets are synchronized into the `central_datasets` schema. The program creates or updates target tables based on the Central entity schema and tracks processed rows in `central_metadata.sync_runs_detail`.

### Forms

Form submissions are synchronized into the `central_submissions` schema.

The root `Submissions` table uses the configured `table_name`. Repeat tables are derived from the root table name and the OData repeat path.

Supported form sync modes:

| Mode | Behavior |
| --- | --- |
| `append_only` | Fetches new submissions based on Central submission date and inserts rows that were not already synchronized. |
| `upsert` | Fetches submissions based on Central submission date or update date and updates existing rows when needed. |

If `sync_mode` is empty, `append_only` is used.

### Approved Submissions Only

Set `approved_only: true` on a form to fetch only submissions whose Central review state is `approved`.

This option only filters what is fetched from Central. It does not approve anything by itself.

### Approve After Sync

Set `approve_after_sync: true` on a form to approve each root submission in ODK Central after its rows have been successfully committed to PostgreSQL.

The sequence is intentionally ordered this way:

1. Insert or update the submission rows in PostgreSQL inside a transaction.
2. Commit the PostgreSQL transaction.
3. Approve the submission in ODK Central.
4. Add a sync comment in ODK Central.

This guarantees that a submission is not approved in Central before it has been stored locally.

This is not atomic across PostgreSQL and ODK Central. If the database commit succeeds but the Central approval fails, the data remains synchronized in PostgreSQL and the run records a failed detail with `sync_action = 'approve_after_sync_failed'`. If approval succeeds but the sync comment fails, the run records `sync_action = 'approve_comment_failed'`.

Treat these cases as recoverable partial failures and inspect `central_metadata.sync_runs` and `central_metadata.sync_runs_detail` to identify affected submissions.

## Observability

`central-sync` writes logs to stdout and to `central-sync.log`.

Each process launch generates one UUID named `sync_id`. The same value is stored on every run created during that launch, including work performed across different project databases. Startup, completion, dataset/form lifecycle and error log messages include it so the complete execution can be correlated without repeating it on every intermediate message.

Each dataset or form sync also receives its own database-generated `run_id` and creates a record in `central_metadata.sync_runs`. A `run_id` identifies one dataset or form run, while a `sync_id` identifies the complete `central-sync` launch. Row-level details are written to `central_metadata.sync_runs_detail` and linked to their launch through `run_id`.

For example, all metadata for one launch can be inspected with:

```sql
SELECT *
FROM central_metadata.sync_runs
WHERE sync_id = '00000000-0000-4000-8000-000000000000';
```

To include row-level details:

```sql
SELECT r.sync_id, d.*
FROM central_metadata.sync_runs AS r
JOIN central_metadata.sync_runs_detail AS d USING (run_id)
WHERE r.sync_id = '00000000-0000-4000-8000-000000000000';
```

Useful metadata objects:

| Object | Purpose |
| --- | --- |
| `central_metadata.sync_runs` | One row per high-level sync run. |
| `central_metadata.sync_runs_detail` | Detailed row-level or submission-level sync events. |
| `central_metadata.last_successful_submissions_sync` | Incremental cursor for form submissions. |
| `central_metadata.last_successful_datasets_sync` | Incremental cursor for datasets. |
| `central_metadata.last_failed_submissions` | Latest failed submission events used for retry tracking. |

## Development

Run tests locally:

```sh
go test ./...
```

The repository also includes a GitHub Actions workflow that runs the same command on `push` and `pull_request`.

Before opening a pull request, keep unrelated local files out of the commit. Local notes such as audit documents or next-step notes should only be committed if they are meant to become project documentation.

## Known Limitations

The following limitations are known and will be added or improved in future versions:

- PostgreSQL indexes are still minimal in this first public version. Additional indexes are planned for metadata lookups, incremental sync cursors, failed-submission retries, and synchronized business tables.
- Submission attachments, such as photos, audio files, or other media files, are not synchronized yet.
- Log file management is still minimal. `central-sync` writes to stdout and appends messages to `central-sync.log`, but it does not handle log rotation, retention, maximum size, or a configurable log path yet.

## License

`central-sync` is licensed under the Apache License 2.0. See [LICENSE](LICENSE).

## Contributing

Contributions are welcome. Bug reports, documentation improvements, tests, and code changes that make the sync safer or easier to operate are all useful.

Please open an issue or a pull request with a clear description of the problem, the proposed change, and any relevant context about your ODK Central or PostgreSQL setup.
