CREATE ROLE your_central_user WITH 
	NOSUPERUSER
	NOCREATEDB
	NOCREATEROLE
	NOINHERIT
	LOGIN
	NOREPLICATION
	NOBYPASSRLS
	CONNECTION LIMIT -1
    PASSWORD 'your_central_user_password';

GRANT CONNECT ON DATABASE your_database TO your_central_user;

GRANT ALL ON SCHEMA central_datasets TO your_central_user;
GRANT ALL ON SCHEMA central_submissions TO your_central_user;
GRANT ALL ON SCHEMA central_metadata TO your_central_user;

ALTER TABLE central_metadata.sync_runs OWNER TO your_central_user;
ALTER TABLE central_metadata.sync_runs_detail OWNER TO your_central_user;

ALTER VIEW central_metadata.last_successful_submissions_sync OWNER TO your_central_user;
ALTER VIEW central_metadata.last_successful_datasets_sync OWNER TO your_central_user;
ALTER VIEW central_metadata.last_failed_submissions OWNER TO your_central_user;