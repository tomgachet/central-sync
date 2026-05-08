# central-sync
A Go program for synchronizing data from a ODK Central instance into a PostgreSQL database

## Features
- Configuration settings in a YAML file
- Synchronization of datasets (entity lists) and form submissions
- Automatic evolution of database tables linked to form updates in ODK Central
- Storage of geometries in GeoJSON format
- Simple binary to execute with no dependencies

## Installation

Download binary from the Release page

## Configuration

central-sync requires 2 configuration files to create in the same folder as the binary file  :

- `.env` with all the necessary credentials needed to access ODK Central instance and the PostgreSQL database :

```yaml
ODK_CENTRAL_URL=https://my_odk_central_instance.url
ODK_CENTRAL_USER_EMAIL=user@email.com
ODK_CENTRAL_USER_PASSWORD=user_password

PG_HOST=localhost
PG_PORT=5432
PG_USER=central_user
PG_PASSWORD=pg_central_user_password
PG_SSLMODE=disable
```
For security reasons, it is recommended to create a PostgreSQL user who will only have read/write privileges on schemas that will store data in the database.

<br>

- `central_config.yaml` with all the information regarding the elements to synchronize :


```yaml
projects:
  - project_id: 1
    project_name: "project_name_for_information_only"
    database_name: "target_database_name_for_project_1"
    datasets:
      - name: "central_entity_list_name_1"
        table_name: "target_table_name_1"
        sync: true
      - name: "central_entity_list_name_2"
        table_name: "target_table_name_1"
        sync: true
    forms:
      - xml_form_id: "central_form_id"
        table_name: "target_table_name"
        sync: true

  - project_id: 2
    project_name: "project_name_for_information_only"
    database_name: "target_database_name_for_project_2"
```

<br>

In Postgresql, some preparations need to be made with a superuser role:

- creation of the user who will connect to PosgreSQL from central-sync

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
```

- creation of the databases linked to the projects to be synchronized
```sql
CREATE DATABASE target_database_name_for_project_1
    WITH
    OWNER = superuser
    ENCODING = 'UTF8'
    LC_COLLATE = 'fr_FR.UTF-8'
    LC_CTYPE = 'fr_FR.UTF-8'
    LOCALE_PROVIDER = 'libc'
    TABLESPACE = pg_default
    CONNECTION LIMIT = -1
    IS_TEMPLATE = False;
```
- give the right to connect to databases :
```sql
GRANT CONNECT ON DATABASE target_database_name_for_project_1 TO central_user;
```

- create the schemas that will host the data from ODK Central
```sql
CREATE SCHEMA central_datasets AUTHORIZATION superuser;
CREATE SCHEMA central_submissions AUTHORIZATION superuser;
```
- give the appropriate privileges on these schemas to the central-sync user
```sql
GRANT ALL ON SCHEMA central_datasets TO central_user;
GRANT ALL ON SCHEMA central_submissions TO central_user;
```

