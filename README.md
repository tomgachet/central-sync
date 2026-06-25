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

Download a binary from the release page, then place it in a directory containing `.env` and `central_config.yaml`.

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

  - project_id: 2
    project_name: "Another project"
    database_name: "central_project_2"
    datasets: []
    forms: []
```

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

Table names must be unique inside the same project across dataset and form mappings.

## PostgreSQL Setup

Each configured project points to one PostgreSQL database. That database must already exist and contain the required schemas and metadata tables.

Run the initialization scripts with a privileged PostgreSQL role, adapting database and role names first:

```sh
psql -d your_database -f sql_init/01_init_structure.sql
psql -d your_database -f sql_init/02_init_role_and_privileges.sql
```

The structure script creates:

- `central_datasets`
- `central_submissions`
- `central_metadata`
- `central_metadata.sync_runs`
- `central_metadata.sync_runs_detail`
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
./central-sync
```

Print the version:

```sh
./central-sync version
```

The program runs in this order:

1. Load `.env`.
2. Load and validate `central_config.yaml`.
3. Authenticate to ODK Central.
4. Synchronize configured datasets.
5. Synchronize configured forms.
6. Write logs to stdout and `central-sync.log`.

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

Each dataset or form sync creates records in `central_metadata.sync_runs`. Row-level details are written to `central_metadata.sync_runs_detail` with counters for fetched, inserted, updated, skipped, deleted and failed rows.

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

PostgreSQL indexes are still minimal in this first public version. Additional indexes for metadata lookups, incremental sync cursors, failed-submission retries, and synchronized business tables are planned for upcoming versions.

## License

`central-sync` is licensed under the Apache License 2.0. See [LICENSE](LICENSE).

## Contributing

Contributions are welcome. Bug reports, documentation improvements, tests, and code changes that make the sync safer or easier to operate are all useful.

Please open an issue or a pull request with a clear description of the problem, the proposed change, and any relevant context about your ODK Central or PostgreSQL setup.
